package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

type processTUIScenario struct {
	Name      string
	Args      []string
	Width     int
	Height    int
	Script    func(*testing.T, *processTUIRun)
	Validate  func(*testing.T, *processTUIRun, string)
	Required  []string
	Forbidden []string
}

type processTUIRun struct {
	name      string
	binary    string
	args      []string
	workspace string
	home      string
	width     int
	height    int
	cmd       *exec.Cmd
	master    *os.File
	output    *lockedBuffer
	waitDone  chan error
	copyDone  chan error
}

func TestTUIProcessRuntimeE2E(t *testing.T) {
	if os.Getenv("PAPERSILM_PROCESS_TUI_SMOKE") != "1" {
		t.Skip("set PAPERSILM_PROCESS_TUI_SMOKE=1 to run process-backed TUI smoke")
	}
	if goruntime.GOOS == "windows" {
		t.Skip("process-backed TUI smoke requires a unix pseudo-terminal")
	}

	binary := buildProcessTUIBinary(t)
	for _, scenario := range processTUIScenarios() {
		scenario := scenario
		t.Run(scenario.Name, func(t *testing.T) {
			run := startProcessTUI(t, binary, scenario)
			defer run.stop()

			waitForProcessTUIFrame(t, run, []string{"papersilm", "› Ask about workspace or papers"}, scenario.Forbidden)
			if scenario.Script != nil {
				scenario.Script(t, run)
			}
			_, frame := waitForProcessTUIFrame(t, run, scenario.Required, scenario.Forbidden)
			assertProcessTUIFrame(t, run, frame, scenario.Required, scenario.Forbidden)
			if scenario.Validate != nil {
				scenario.Validate(t, run, frame)
			}
		})
	}
}

func processTUIScenarios() []processTUIScenario {
	const (
		width  = 80
		height = 28
	)
	baseForbidden := append([]string{}, globalVisualForbiddenStrings()...)
	baseForbidden = append(baseForbidden, "agent_event", "⏺", "Permission decision:")
	return []processTUIScenario{
		{
			Name:      "workspace_search",
			Args:      []string{"--permission-mode", "auto"},
			Width:     width,
			Height:    height,
			Forbidden: baseForbidden,
			Script: func(t *testing.T, run *processTUIRun) {
				t.Helper()
				processWriteInput(t, run, "search `process-smoke-marker`\r")
			},
			Required: []string{"search `process-smoke-marker`", "Search results", "README.md", "process-smoke-marker", "· Inspecting workspace"},
			Validate: func(t *testing.T, run *processTUIRun, _ string) {
				t.Helper()
				waitForProcessSessionFileContains(t, run, "events.jsonl", []string{"\"type\":\"plan\"", "\"type\":\"progress\"", "\"type\":\"result\""})
				waitForProcessSessionFileContains(t, run, "transcript.jsonl", []string{"search `process-smoke-marker`", "Search results", "process-smoke-marker"})
			},
		},
		{
			Name:      "confirm_command_approve",
			Args:      []string{"--permission-mode", "confirm"},
			Width:     width,
			Height:    height,
			Forbidden: baseForbidden,
			Script: func(t *testing.T, run *processTUIRun) {
				t.Helper()
				processWriteInput(t, run, "run command `printf %s process-shell`\r")
				waitForProcessTUIFrame(t, run, []string{"Run command", "printf %s process-shell", "Enter select"}, processTUIForbiddenStrings())
				processWriteInput(t, run, "\r")
			},
			Required: []string{"✓ Approved", "Command: printf %s process-shell", "command exited 0", "process-shell"},
			Validate: func(t *testing.T, run *processTUIRun, _ string) {
				t.Helper()
				waitForProcessSessionFileContains(t, run, "transcript.jsonl", []string{"Approval Required", "✓ Approved", "command exited 0", "process-shell"})
			},
		},
		{
			Name:      "confirm_edit_approve",
			Args:      []string{"--permission-mode", "confirm"},
			Width:     width,
			Height:    height,
			Forbidden: baseForbidden,
			Script: func(t *testing.T, run *processTUIRun) {
				t.Helper()
				processWriteInput(t, run, "update `README.md` replace `typo` with `type`\r")
				waitForProcessTUIFrame(t, run, []string{"Edit file", "README.md", "-hello typo", "+hello type", "Enter select"}, processTUIForbiddenStrings())
				assertProcessFileContains(t, run, "README.md", []string{"hello typo"}, []string{"hello type"})
				processWriteInput(t, run, "\r")
			},
			Required: []string{"✓ Approved", "README.md", "Replace text in README.md"},
			Validate: func(t *testing.T, run *processTUIRun, _ string) {
				t.Helper()
				assertProcessFileContains(t, run, "README.md", []string{"hello type"}, []string{"hello typo"})
				waitForProcessSessionFileContains(t, run, "transcript.jsonl", []string{"Approval Required", "✓ Approved", "Replace text in README.md"})
			},
		},
		{
			Name:      "confirm_command_reject_feedback",
			Args:      []string{"--permission-mode", "confirm"},
			Width:     width,
			Height:    height,
			Forbidden: baseForbidden,
			Script: func(t *testing.T, run *processTUIRun) {
				t.Helper()
				processWriteInput(t, run, "run command `sh -c 'printf rejected > rejected.txt'`\r")
				waitForProcessTUIFrame(t, run, []string{"Run command", "printf rejected > rejected.txt", "Enter select"}, processTUIForbiddenStrings())
				processWriteInput(t, run, "\x1b[B\x1b[B\tuse a safer command\r")
			},
			Required: []string{"Tool use rejected", "Rejected with feedback.", "Feedback: use a safer command"},
			Validate: func(t *testing.T, run *processTUIRun, _ string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(run.workspace, "rejected.txt")); err == nil {
					failProcessTUI(t, run, "rejected command produced side-effect file")
				} else if !errors.Is(err, os.ErrNotExist) {
					failProcessTUI(t, run, fmt.Sprintf("stat rejected command output: %v", err))
				}
				waitForProcessSessionFileContains(t, run, "transcript.jsonl", []string{"Tool use rejected", "Feedback: use a safer command"})
			},
		},
	}
}

func buildProcessTUIBinary(t *testing.T) string {
	t.Helper()
	repoRoot := processRepoRoot(t)
	binary := filepath.Join(t.TempDir(), "papersilm")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/papersilm")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build process TUI binary: %v\n%s", err, output)
	}
	return binary
}

func processRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		t.Fatalf("resolve caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func startProcessTUI(t *testing.T, binary string, scenario processTUIScenario) *processTUIRun {
	t.Helper()
	width := scenario.Width
	if width <= 0 {
		width = 80
	}
	height := scenario.Height
	if height <= 0 {
		height = 28
	}
	workspace := t.TempDir()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# process smoke\n\nprocess-smoke-marker\nhello typo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md): %v", err)
	}
	configDir := filepath.Join(home, ".papersilm")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("theme: dark\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(config.yaml): %v", err)
	}

	cmd := exec.Command(binary, scenario.Args...)
	cmd.Dir = workspace
	cmd.Env = processTUIEnv(home)
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(height), Cols: uint16(width)})
	if err != nil {
		if errors.Is(err, pty.ErrUnsupported) {
			t.Skipf("pty unsupported: %v", err)
		}
		t.Fatalf("start process TUI: %v", err)
	}

	run := &processTUIRun{
		name:      scenario.Name,
		binary:    binary,
		args:      append([]string{}, scenario.Args...),
		workspace: workspace,
		home:      home,
		width:     width,
		height:    height,
		cmd:       cmd,
		master:    master,
		output:    &lockedBuffer{},
		waitDone:  make(chan error, 1),
		copyDone:  make(chan error, 1),
	}
	go func() {
		run.copyDone <- copyProcessTUIOutput(run)
	}()
	go func() {
		run.waitDone <- cmd.Wait()
	}()
	return run
}

func copyProcessTUIOutput(run *processTUIRun) error {
	buf := make([]byte, 4096)
	sentBackground := false
	sentCursor := false
	for {
		n, err := run.master.Read(buf)
		if n > 0 {
			_, _ = run.output.Write(buf[:n])
			raw := run.output.String()
			if !sentBackground && strings.Contains(raw, "\x1b]11;?\x1b\\") {
				_, _ = run.master.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\"))
				sentBackground = true
			}
			if !sentCursor && strings.Contains(raw, "\x1b[6n") {
				_, _ = run.master.Write([]byte("\x1b[1;1R"))
				sentCursor = true
			}
		}
		if err != nil {
			return err
		}
	}
}

func processTUIEnv(home string) []string {
	env := make([]string, 0, len(os.Environ())+6)
	for _, item := range os.Environ() {
		if strings.HasPrefix(item, "HOME=") ||
			strings.HasPrefix(item, "TERM=") ||
			strings.HasPrefix(item, "NO_COLOR=") ||
			strings.HasPrefix(item, "PAPERSILM_PROCESS_TUI_SMOKE=") ||
			strings.HasPrefix(item, "PAPERSILM_TUI_PROCESS_ARTIFACT_DIR=") {
			continue
		}
		env = append(env, item)
	}
	env = append(env,
		"HOME="+home,
		"TERM=xterm-256color",
	)
	return env
}

func (r *processTUIRun) stop() {
	if r == nil {
		return
	}
	if r.master != nil {
		_, _ = r.master.Write([]byte{0x03})
	}
	select {
	case <-r.waitDone:
	case <-time.After(300 * time.Millisecond):
		if r.cmd != nil && r.cmd.Process != nil {
			_ = r.cmd.Process.Kill()
		}
		select {
		case <-r.waitDone:
		case <-time.After(time.Second):
		}
	}
	if r.master != nil {
		_ = r.master.Close()
	}
	select {
	case <-r.copyDone:
	case <-time.After(time.Second):
	}
}

func processWriteInput(t *testing.T, run *processTUIRun, input string) {
	t.Helper()
	if _, err := run.master.Write([]byte(input)); err != nil {
		failProcessTUI(t, run, fmt.Sprintf("write process TUI input %q: %v", input, err))
	}
}

func waitForProcessTUIFrame(t *testing.T, run *processTUIRun, required []string, forbidden []string) (string, string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var raw, frame string
	for time.Now().Before(deadline) {
		raw = run.output.String()
		frame = visiblePTYFrame(raw, run.width, run.height)
		if ptyFrameContainsAll(frame, required) && !ptyFrameContainsAny(frame, forbidden) && ptyFrameLinesFit(frame, run.width) {
			return raw, frame
		}
		time.Sleep(25 * time.Millisecond)
	}
	failProcessTUI(t, run, fmt.Sprintf("timed out waiting for process TUI frame with required strings: %v", required))
	return "", ""
}

func assertProcessTUIFrame(t *testing.T, run *processTUIRun, frame string, required []string, forbidden []string) {
	t.Helper()
	raw := run.output.String()
	if !strings.Contains(raw, "\x1b[?1049h") {
		failProcessTUI(t, run, "expected alt-screen ANSI in process TUI output")
	}
	for _, want := range required {
		if !strings.Contains(frame, want) {
			failProcessTUI(t, run, fmt.Sprintf("expected %q in process TUI frame", want))
		}
	}
	if !strings.Contains(frame, "›") {
		failProcessTUI(t, run, "expected prompt marker in process TUI frame")
	}
	for _, bad := range forbidden {
		if strings.Contains(frame, bad) {
			failProcessTUI(t, run, fmt.Sprintf("did not expect %q in process TUI frame", bad))
		}
	}
	if !ptyFrameLinesFit(frame, run.width) {
		failProcessTUI(t, run, fmt.Sprintf("expected process TUI frame to fit width %d", run.width))
	}
	if footer := extractPTYFooter(frame); len(footer) > 2 {
		failProcessTUI(t, run, fmt.Sprintf("expected footer <= 2 lines, got %d: %q", len(footer), footer))
	}
}

func assertProcessFileContains(t *testing.T, run *processTUIRun, rel string, required []string, forbidden []string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(run.workspace, rel))
	if err != nil {
		failProcessTUI(t, run, fmt.Sprintf("read workspace file %s: %v", rel, err))
	}
	content := string(raw)
	for _, want := range required {
		if !strings.Contains(content, want) {
			failProcessTUI(t, run, fmt.Sprintf("expected workspace file %s to contain %q, got %q", rel, want, content))
		}
	}
	for _, bad := range forbidden {
		if strings.Contains(content, bad) {
			failProcessTUI(t, run, fmt.Sprintf("did not expect workspace file %s to contain %q, got %q", rel, bad, content))
		}
	}
}

func waitForProcessSessionFileContains(t *testing.T, run *processTUIRun, rel string, needles []string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var content string
	for time.Now().Before(deadline) {
		sessionDir, err := latestProcessSessionDir(run)
		if err == nil {
			raw, readErr := os.ReadFile(filepath.Join(sessionDir, rel))
			if readErr == nil {
				content = string(raw)
				if containsAll(content, needles) {
					return
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	failProcessTUI(t, run, fmt.Sprintf("expected session file %s to contain %v, got %q", rel, needles, content))
}

func latestProcessSessionDir(run *processTUIRun) (string, error) {
	root := filepath.Join(run.workspace, ".papersilm", "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
		}
	}
	if len(dirs) == 0 {
		return "", os.ErrNotExist
	}
	sort.Strings(dirs)
	return dirs[len(dirs)-1], nil
}

func containsAll(content string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(content, needle) {
			return false
		}
	}
	return true
}

func processTUIForbiddenStrings() []string {
	forbidden := append([]string{}, globalVisualForbiddenStrings()...)
	forbidden = append(forbidden, "agent_event", "⏺", "Permission decision:")
	return forbidden
}

func failProcessTUI(t *testing.T, run *processTUIRun, reason string) {
	t.Helper()
	artifactDir := dumpProcessTUIArtifacts(t, run, reason)
	frame := visiblePTYFrame(run.output.String(), run.width, run.height)
	t.Fatalf("%s for %s\nProcess TUI failure artifacts: %s\n--- frame ---\n%s", reason, run.name, artifactDir, frame)
}

func dumpProcessTUIArtifacts(t *testing.T, run *processTUIRun, reason string) string {
	t.Helper()
	root := os.Getenv("PAPERSILM_TUI_PROCESS_ARTIFACT_DIR")
	var dir string
	var err error
	if root == "" {
		dir, err = os.MkdirTemp("", "papersilm-tui-process-*")
	} else {
		if err = os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("create process artifact root: %v", err)
		}
		dir, err = os.MkdirTemp(root, sanitizePTYArtifactName(run.name)+"-*")
	}
	if err != nil {
		t.Fatalf("create process artifact dir: %v", err)
	}
	raw := run.output.String()
	files := map[string]string{
		"frame.normalized.txt": visiblePTYFrame(raw, run.width, run.height),
		"raw.ansi":             raw,
		"raw.escaped.txt":      fmt.Sprintf("%q\n", raw),
		"screen.rendered.txt":  renderPTYScreen(raw, run.width, run.height),
		"scenario.txt": fmt.Sprintf(
			"scenario: %s\nreason: %s\nbinary: %s\nargs: %s\nworkspace: %s\nhome: %s\nwidth: %d\nheight: %d\n",
			run.name,
			reason,
			run.binary,
			strings.Join(run.args, " "),
			run.workspace,
			run.home,
			run.width,
			run.height,
		),
		"workspace-tree.txt": processWorkspaceTree(run.workspace),
		"session-files.txt":  processSessionFiles(run.workspace),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write process artifact %s: %v", name, err)
		}
	}
	return dir
}

func processWorkspaceTree(root string) string {
	var lines []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			lines = append(lines, fmt.Sprintf("ERROR %s: %v", path, err))
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		if rel == "." {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			lines = append(lines, rel)
			return nil
		}
		kind := "file"
		if entry.IsDir() {
			kind = "dir"
		}
		lines = append(lines, fmt.Sprintf("%s\t%s\t%d", kind, rel, info.Size()))
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func processSessionFiles(workspace string) string {
	root := filepath.Join(workspace, ".papersilm", "sessions")
	var parts []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".json") && !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			parts = append(parts, fmt.Sprintf("== %s ==\nread error: %v", rel, readErr))
			return nil
		}
		content := string(raw)
		if len(content) > 64*1024 {
			content = content[:64*1024] + "\n... truncated ..."
		}
		parts = append(parts, fmt.Sprintf("== %s ==\n%s", rel, content))
		return nil
	})
	sort.Strings(parts)
	return strings.Join(parts, "\n\n")
}
