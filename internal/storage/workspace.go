package storage

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

const (
	workspaceMaxIndexedFileSize = 512 * 1024
	workspaceSearchResultLimit  = 50
)

func (s *Store) workspaceMetaPath() string {
	return filepath.Join(s.baseDir, "workspace.json")
}

func (s *Store) workspaceFilesPath() string {
	return filepath.Join(s.baseDir, "index", "files.json")
}

func (s *Store) commandLogDir() string {
	return filepath.Join(s.baseDir, "ops", "commands")
}

func (s *Store) RefreshWorkspaceState() error {
	files, err := s.scanWorkspaceFiles()
	if err != nil {
		return err
	}
	summary := protocol.WorkspaceSummary{
		WorkspaceID:    workspaceIDFromPath(s.workspaceRoot),
		Root:           s.workspaceRoot,
		Name:           workspaceNameFromPath(s.workspaceRoot),
		FileCount:      len(files),
		TextFileCount:  countWorkspaceFilesByKind(files, protocol.WorkspaceFileKindText, protocol.WorkspaceFileKindCode, protocol.WorkspaceFileKindConfig),
		PaperFileCount: countPaperCandidates(files),
		SessionCount:   s.sessionCount(),
		IndexedAt:      time.Now().UTC(),
	}
	if err := s.saveJSON(s.workspaceMetaPath(), summary); err != nil {
		return err
	}
	return s.saveJSON(s.workspaceFilesPath(), files)
}

func (s *Store) LoadWorkspaceSummary() (*protocol.WorkspaceSummary, error) {
	var summary protocol.WorkspaceSummary
	if err := s.loadJSON(s.workspaceMetaPath(), &summary); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if refreshErr := s.RefreshWorkspaceState(); refreshErr != nil {
			return nil, refreshErr
		}
		if err := s.loadJSON(s.workspaceMetaPath(), &summary); err != nil {
			return nil, err
		}
	}
	return &summary, nil
}

func (s *Store) LoadWorkspaceFiles() ([]protocol.WorkspaceFile, error) {
	var files []protocol.WorkspaceFile
	if err := s.loadJSON(s.workspaceFilesPath(), &files); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if refreshErr := s.RefreshWorkspaceState(); refreshErr != nil {
			return nil, refreshErr
		}
		if err := s.loadJSON(s.workspaceFilesPath(), &files); err != nil {
			return nil, err
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func (s *Store) WorkspacePaperCandidates() ([]protocol.WorkspaceFile, error) {
	files, err := s.LoadWorkspaceFiles()
	if err != nil {
		return nil, err
	}
	out := make([]protocol.WorkspaceFile, 0)
	for _, file := range files {
		if file.PaperCandidate {
			out = append(out, file)
		}
	}
	return out, nil
}

func (s *Store) ResolveWorkspacePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(s.workspaceRoot, resolved)
	}
	resolved = filepath.Clean(resolved)
	rel, err := filepath.Rel(s.workspaceRoot, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}
	return resolved, nil
}

func (s *Store) ReadWorkspaceFile(path string) (string, error) {
	resolved, err := s.ResolveWorkspacePath(path)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *Store) WriteWorkspaceFile(path, content string) error {
	resolved, err := s.ResolveWorkspacePath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
		return err
	}
	return s.RefreshWorkspaceState()
}

func (s *Store) ReplaceWorkspaceFile(path, oldValue, newValue string) error {
	content, err := s.ReadWorkspaceFile(path)
	if err != nil {
		return err
	}
	if !strings.Contains(content, oldValue) {
		return fmt.Errorf("target text not found in %s", path)
	}
	return s.WriteWorkspaceFile(path, strings.Replace(content, oldValue, newValue, 1))
}

func (s *Store) SearchWorkspace(query string, limit int) ([]protocol.WorkspaceSearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search query is required")
	}
	if limit <= 0 || limit > workspaceSearchResultLimit {
		limit = workspaceSearchResultLimit
	}
	files, err := s.LoadWorkspaceFiles()
	if err != nil {
		return nil, err
	}
	lowerQuery := strings.ToLower(query)
	hits := make([]protocol.WorkspaceSearchHit, 0, limit)
	for _, file := range files {
		if !workspaceFileIsTextual(file) {
			continue
		}
		if file.SizeBytes > workspaceMaxIndexedFileSize {
			continue
		}
		content, err := s.ReadWorkspaceFile(file.Path)
		if err != nil {
			continue
		}
		lines := strings.Split(content, "\n")
		for idx, line := range lines {
			if !strings.Contains(strings.ToLower(line), lowerQuery) {
				continue
			}
			hits = append(hits, protocol.WorkspaceSearchHit{
				Path:    file.Path,
				Line:    idx + 1,
				Snippet: strings.TrimSpace(line),
			})
			if len(hits) >= limit {
				return hits, nil
			}
		}
	}
	return hits, nil
}

func (s *Store) RunWorkspaceCommand(command string) (protocol.WorkspaceCommandRecord, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return protocol.WorkspaceCommandRecord{}, fmt.Errorf("command is required")
	}
	startedAt := time.Now().UTC()
	cmd := exec.Command("zsh", "-lc", command)
	cmd.Dir = s.workspaceRoot
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	record := protocol.WorkspaceCommandRecord{
		Command:     command,
		Cwd:         s.workspaceRoot,
		ExitCode:    exitCode,
		Stdout:      stdout.String(),
		Stderr:      stderr.String(),
		StartedAt:   startedAt,
		CompletedAt: time.Now().UTC(),
	}
	if logErr := s.appendJSONL(filepath.Join(s.commandLogDir(), "commands.jsonl"), record); logErr != nil && runErr == nil {
		runErr = logErr
	}
	return record, runErr
}

func (s *Store) scanWorkspaceFiles() ([]protocol.WorkspaceFile, error) {
	files := make([]protocol.WorkspaceFile, 0, 128)
	err := filepath.WalkDir(s.workspaceRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == s.workspaceRoot {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if shouldSkipWorkspaceDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipWorkspaceFile(name) {
			return nil
		}
		rel, err := filepath.Rel(s.workspaceRoot, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		kind := classifyWorkspaceFile(path)
		files = append(files, protocol.WorkspaceFile{
			Path:           filepath.ToSlash(rel),
			AbsolutePath:   path,
			Kind:           kind,
			SizeBytes:      info.Size(),
			ModifiedAt:     info.ModTime().UTC(),
			PaperCandidate: isPaperCandidate(path),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func (s *Store) sessionCount() int {
	ids, err := s.SessionIDs()
	if err != nil {
		return 0
	}
	return len(ids)
}

func (s *Store) SessionIDs() ([]string, error) {
	entries, err := os.ReadDir(s.SessionsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			out = append(out, entry.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

func workspaceNameFromPath(path string) string {
	name := filepath.Base(path)
	if strings.TrimSpace(name) == "" || name == "." || name == string(filepath.Separator) {
		return "workspace"
	}
	return name
}

func workspaceIDFromPath(path string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(filepath.Clean(path)))
	return fmt.Sprintf("%s-%x", workspaceNameFromPath(path), h.Sum32())
}

func countWorkspaceFilesByKind(files []protocol.WorkspaceFile, kinds ...protocol.WorkspaceFileKind) int {
	if len(files) == 0 || len(kinds) == 0 {
		return 0
	}
	allowed := make(map[protocol.WorkspaceFileKind]struct{}, len(kinds))
	for _, kind := range kinds {
		allowed[kind] = struct{}{}
	}
	count := 0
	for _, file := range files {
		if _, ok := allowed[file.Kind]; ok {
			count++
		}
	}
	return count
}

func countPaperCandidates(files []protocol.WorkspaceFile) int {
	count := 0
	for _, file := range files {
		if file.PaperCandidate {
			count++
		}
	}
	return count
}

func shouldSkipWorkspaceDir(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ".git", ".papersilm", "sessions", "index", "cache", "ops", "output-styles", "skills", "node_modules", "vendor", "dist", "build", "target", ".next", ".venv", "venv", "__pycache__":
		return true
	default:
		return false
	}
}

func shouldSkipWorkspaceFile(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(name, ".ds_store"):
		return true
	case strings.HasSuffix(name, ".png"), strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"), strings.HasSuffix(name, ".gif"), strings.HasSuffix(name, ".webp"), strings.HasSuffix(name, ".zip"), strings.HasSuffix(name, ".tar"), strings.HasSuffix(name, ".gz"), strings.HasSuffix(name, ".sqlite"), strings.HasSuffix(name, ".db"), strings.HasSuffix(name, ".bin"), strings.HasSuffix(name, ".so"), strings.HasSuffix(name, ".dylib"), strings.HasSuffix(name, ".exe"), strings.HasSuffix(name, ".class"), strings.HasSuffix(name, ".jar"):
		return true
	default:
		return false
	}
}

func classifyWorkspaceFile(path string) protocol.WorkspaceFileKind {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".txt", ".rst":
		return protocol.WorkspaceFileKindText
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".java", ".rb", ".rs", ".c", ".cc", ".cpp", ".h", ".hpp", ".sh", ".zsh":
		return protocol.WorkspaceFileKindCode
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".env", ".lock":
		return protocol.WorkspaceFileKindConfig
	case ".pdf", ".tex":
		return protocol.WorkspaceFileKindPaper
	default:
		return protocol.WorkspaceFileKindOther
	}
}

func isPaperCandidate(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf", ".tex", ".md", ".txt":
	default:
		return false
	}
	base := strings.ToLower(filepath.Base(path))
	if ext == ".pdf" {
		return true
	}
	return strings.Contains(base, "paper") ||
		strings.Contains(base, "arxiv") ||
		strings.Contains(base, "review") ||
		strings.Contains(base, "notes") ||
		strings.Contains(base, "summary")
}

func workspaceFileIsTextual(file protocol.WorkspaceFile) bool {
	switch file.Kind {
	case protocol.WorkspaceFileKindText, protocol.WorkspaceFileKindCode, protocol.WorkspaceFileKindConfig, protocol.WorkspaceFileKindPaper:
		return !strings.HasSuffix(strings.ToLower(file.Path), ".pdf")
	default:
		return false
	}
}
