package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/internal/providers"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type sessionTaskMode string

const (
	sessionTaskWorkspace sessionTaskMode = "workspace"
	sessionTaskPaper     sessionTaskMode = "paper"
	sessionTaskMixed     sessionTaskMode = "mixed"
)

type workspaceIntent struct {
	kind        protocol.NodeKind
	targetPath  string
	searchQuery string
	command     string
}

type workspaceRewriteResponse struct {
	Summary string `json:"summary"`
	Content string `json:"content"`
}

var (
	arxivIDPattern = regexp.MustCompile(`\b(?:\d{4}\.\d{4,5}|[a-z\-]+(?:\.[A-Za-z\-]+)?/\d{7})(?:v\d+)?\b`)
	urlPattern     = regexp.MustCompile(`https?://[^\s]+`)
)

func classifySessionTask(goal string, existingSources []protocol.PaperRef) sessionTaskMode {
	if len(existingSources) > 0 {
		if goalMentionsWorkspace(goal) {
			return sessionTaskMixed
		}
		return sessionTaskPaper
	}
	if goalNeedsPaperContext(goal) {
		if goalMentionsWorkspace(goal) {
			return sessionTaskMixed
		}
		return sessionTaskPaper
	}
	return sessionTaskWorkspace
}

func goalNeedsPaperContext(goal string) bool {
	lower := strings.ToLower(strings.TrimSpace(goal))
	if lower == "" {
		return false
	}
	for _, token := range []string{
		"paper", "papers", "arxiv", "alphaxiv", "reviewer", "equation", "method", "experiment", "digest",
		"论文", "文献", "arxiv", "方法", "实验", "审稿", "公式", "摘要",
	} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return len(extractPromptPaperSources(goal)) > 0
}

func goalMentionsWorkspace(goal string) bool {
	lower := strings.ToLower(strings.TrimSpace(goal))
	for _, token := range []string{
		"workspace", "repo", "repository", "project", "readme", "file", "files", "folder",
		"工作区", "仓库", "项目", "目录", "文件", "readme",
	} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func extractPromptPaperSources(goal string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	urls := make([]string, 0)
	for _, match := range urlPattern.FindAllString(goal, -1) {
		match = strings.TrimSpace(strings.TrimRight(match, ".,);]"))
		if match == "" {
			continue
		}
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		urls = append(urls, match)
		out = append(out, match)
	}
	for _, match := range arxivIDPattern.FindAllString(goal, -1) {
		match = strings.TrimSpace(match)
		if match == "" {
			continue
		}
		if paperIDCoveredByPromptURL(match, urls) {
			continue
		}
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		out = append(out, match)
	}
	return out
}

func paperIDCoveredByPromptURL(paperID string, urls []string) bool {
	for _, raw := range urls {
		if strings.Contains(raw, paperID) {
			return true
		}
	}
	return false
}

func (a *Agent) ensurePaperContext(ctx context.Context, store *storage.Store, sink EventSink, sessionID, goal string) ([]protocol.PaperRef, error) {
	refs, err := store.LoadSources(sessionID)
	if err != nil {
		return nil, err
	}
	if len(refs) > 0 {
		return refs, nil
	}
	promptSources := extractPromptPaperSources(goal)
	if len(promptSources) > 0 {
		refs, err = a.tools.AttachSources(ctx, store, sessionID, promptSources)
		if err != nil {
			return nil, err
		}
		if err := a.emit(store, sink, sessionID, protocol.EventSourceAttached, "paper context attached from prompt", refs); err != nil {
			return nil, err
		}
		return refs, nil
	}
	candidates, err := a.tools.WorkspacePaperCandidates(store)
	if err != nil {
		return nil, err
	}
	selected := choosePaperCandidates(goal, candidates)
	if len(selected) == 0 {
		return nil, nil
	}
	refs, err = a.tools.AttachSources(ctx, store, sessionID, selected)
	if err != nil {
		return nil, err
	}
	if err := a.emit(store, sink, sessionID, protocol.EventSourceAttached, "paper context attached from workspace", refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func choosePaperCandidates(goal string, files []protocol.WorkspaceFile) []string {
	if len(files) == 0 {
		return nil
	}
	type scored struct {
		raw   string
		score int
	}
	scoredFiles := make([]scored, 0, len(files))
	lowerGoal := strings.ToLower(goal)
	compareRequested := strings.Contains(lowerGoal, "compare") || strings.Contains(lowerGoal, "比较") || strings.Contains(lowerGoal, "对比")
	for _, file := range files {
		if !strings.HasSuffix(strings.ToLower(file.Path), ".pdf") {
			continue
		}
		score := 5
		base := strings.ToLower(file.Path)
		for _, token := range goalKeywords(goal) {
			if strings.Contains(base, token) {
				score += 3
			}
		}
		scoredFiles = append(scoredFiles, scored{raw: file.AbsolutePath, score: score})
	}
	if len(scoredFiles) == 0 {
		return nil
	}
	sort.SliceStable(scoredFiles, func(i, j int) bool {
		if scoredFiles[i].score == scoredFiles[j].score {
			return scoredFiles[i].raw < scoredFiles[j].raw
		}
		return scoredFiles[i].score > scoredFiles[j].score
	})
	limit := 1
	if compareRequested && len(scoredFiles) > 1 {
		limit = min(4, len(scoredFiles))
	} else if len(scoredFiles) == 1 {
		limit = 1
	} else if scoredFiles[0].score < 6 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, item := range scoredFiles[:limit] {
		out = append(out, item.raw)
	}
	return out
}

func goalKeywords(goal string) []string {
	split := strings.FieldsFunc(strings.ToLower(goal), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	out := make([]string, 0, len(split))
	for _, token := range split {
		if len(token) < 3 {
			continue
		}
		out = append(out, token)
	}
	return out
}

func buildWorkspacePlan(goal string, approvalRequired bool, intent workspaceIntent) protocol.PlanResult {
	now := time.Now().UTC()
	nodeID := "workspace_task"
	node := protocol.PlanNode{
		ID:            nodeID,
		Kind:          intent.kind,
		Goal:          strings.TrimSpace(goal),
		WorkerProfile: protocol.WorkerProfileSupervisor,
		Required:      true,
		Status:        protocol.NodeStatusReady,
		ParallelGroup: "workspace",
	}
	step := protocol.PlanStep{
		ID:               nodeID,
		Tool:             string(intent.kind),
		Goal:             strings.TrimSpace(goal),
		ExpectedArtifact: "workspace_response",
	}
	return protocol.PlanResult{
		PlanID:           newPlanID(),
		Goal:             strings.TrimSpace(goal),
		SourceSummary:    nil,
		DAG:              protocol.PlanDAG{Nodes: []protocol.PlanNode{node}},
		Steps:            []protocol.PlanStep{step},
		WillCompare:      false,
		Risks:            []string{"workspace execution may edit local files or run commands depending on the task"},
		ApprovalRequired: approvalRequired,
		CreatedAt:        now,
	}
}

func inferWorkspaceIntent(goal string, files []protocol.WorkspaceFile) workspaceIntent {
	lower := strings.ToLower(strings.TrimSpace(goal))
	intent := workspaceIntent{kind: protocol.NodeKindWorkspaceInspect}
	if command := extractBacktickCommand(goal); command != "" {
		intent.kind = protocol.NodeKindWorkspaceCommand
		intent.command = command
		return intent
	}
	switch {
	case containsAny(lower, "search ", "grep ", "查找", "搜索", "find "):
		intent.kind = protocol.NodeKindWorkspaceSearch
		intent.searchQuery = extractSearchQuery(goal)
	case containsAny(lower, "edit ", "update ", "rewrite ", "fix ", "modify ", "create ", "add ", "修改", "更新", "改写", "修复", "创建", "新增"):
		intent.kind = protocol.NodeKindWorkspaceEdit
	case containsAny(lower, "list files", "show files", "tree", "目录", "列出", "文件"):
		intent.kind = protocol.NodeKindWorkspaceInspect
	default:
		intent.kind = protocol.NodeKindWorkspaceInspect
	}
	intent.targetPath = findWorkspaceTargetPath(goal, files, intent.kind != protocol.NodeKindWorkspaceEdit)
	return intent
}

func findWorkspaceTargetPath(goal string, files []protocol.WorkspaceFile, allowDefault bool) string {
	lower := strings.ToLower(goal)
	if explicit := extractWorkspacePathMention(goal); explicit != "" {
		if indexed := matchIndexedWorkspacePath(explicit, files); indexed != "" {
			return indexed
		}
		return explicit
	}
	best := ""
	bestScore := 0
	for _, file := range files {
		base := strings.ToLower(filepathBase(file.Path))
		filePath := strings.ToLower(file.Path)
		score := 0
		if strings.Contains(lower, filePath) {
			score += 10
		}
		if base != "" && strings.Contains(lower, base) {
			score += 8
		}
		if strings.Contains(lower, strings.TrimSuffix(base, filepathExt(base))) {
			score += 4
		}
		if score > bestScore {
			bestScore = score
			best = file.Path
		}
	}
	if bestScore == 0 && allowDefault {
		for _, file := range files {
			if strings.EqualFold(filepathBase(file.Path), "README.md") {
				return file.Path
			}
		}
	}
	return best
}

func matchIndexedWorkspacePath(explicit string, files []protocol.WorkspaceFile) string {
	key := workspacePathMatchKey(explicit)
	if key == "" {
		return ""
	}
	for _, file := range files {
		if workspacePathMatchKey(file.Path) == key {
			return file.Path
		}
	}
	if strings.Contains(key, "/") {
		return ""
	}
	match := ""
	for _, file := range files {
		if workspacePathMatchKey(filepathBase(file.Path)) != key {
			continue
		}
		if match != "" {
			return ""
		}
		match = file.Path
	}
	return match
}

func workspacePathMatchKey(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return ""
	}
	value = path.Clean(value)
	if value == "." {
		return ""
	}
	value = strings.TrimPrefix(value, "./")
	return strings.ToLower(value)
}

func extractWorkspacePathMention(goal string) string {
	for _, segment := range backtickSegments(goal) {
		if pathLooksLikeWorkspaceFile(segment) {
			return strings.TrimSpace(segment)
		}
	}
	for _, field := range strings.Fields(goal) {
		field = strings.Trim(field, " \t\r\n,.;:()[]{}<>\"'")
		if pathLooksLikeWorkspaceFile(field) {
			return field
		}
	}
	return ""
}

func pathLooksLikeWorkspaceFile(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\n\r") {
		return false
	}
	if strings.Contains(value, "://") || strings.ContainsAny(value, "*?") {
		return false
	}
	ext := strings.ToLower(filepathExt(value))
	switch ext {
	case ".md", ".txt", ".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".json", ".yaml", ".yml", ".toml", ".tex", ".csv":
		return true
	default:
		return false
	}
}

func extractSearchQuery(goal string) string {
	trimmed := strings.TrimSpace(goal)
	for _, quote := range []string{`"`, `'`, "`"} {
		parts := strings.Split(trimmed, quote)
		if len(parts) >= 3 {
			value := strings.TrimSpace(parts[1])
			if value != "" {
				return value
			}
		}
	}
	lower := strings.ToLower(trimmed)
	for _, marker := range []string{"search", "grep", "find", "查找", "搜索"} {
		if idx := strings.Index(lower, marker); idx >= 0 {
			value := strings.TrimSpace(trimmed[idx+len(marker):])
			value = strings.TrimLeft(value, ":： ")
			if value != "" {
				return value
			}
		}
	}
	return trimmed
}

func extractBacktickCommand(goal string) string {
	lower := strings.ToLower(goal)
	commandContext := containsAny(lower, "run ", "execute ", "shell", "command", "terminal", "运行", "执行", "命令")
	for _, segment := range backtickSegments(goal) {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if !commandContext && pathLooksLikeWorkspaceFile(segment) {
			continue
		}
		return segment
	}
	return ""
}

func backtickSegments(value string) []string {
	parts := strings.Split(value, "`")
	if len(parts) < 3 {
		return nil
	}
	segments := make([]string, 0, len(parts)/2)
	for i := 1; i < len(parts); i += 2 {
		if segment := strings.TrimSpace(parts[i]); segment != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}

func containsAny(input string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(input, token) {
			return true
		}
	}
	return false
}

func workspaceResponseFromOutputs(outputs []protocol.NodeOutputRef) string {
	parts := make([]string, 0, len(outputs))
	for _, output := range outputs {
		if output.Kind != "workspace_response" {
			continue
		}
		if response, ok := output.Data["response"].(string); ok && strings.TrimSpace(response) != "" {
			parts = append(parts, strings.TrimSpace(response))
		}
	}
	return strings.Join(parts, "\n\n")
}

func (a *Agent) executeWorkspaceInspect(ctx context.Context, store *storage.Store, goal string, intent workspaceIntent) (string, error) {
	if strings.TrimSpace(intent.targetPath) != "" {
		content, err := a.tools.ReadWorkspaceFile(store, intent.targetPath)
		if err != nil {
			return "", err
		}
		return a.workspaceSummarizeFile(ctx, store, goal, intent.targetPath, content)
	}
	files, err := a.tools.LoadWorkspaceFiles(store)
	if err != nil {
		return "", err
	}
	return a.workspaceSummarizeWorkspace(ctx, store, goal, files)
}

func (a *Agent) executeWorkspaceSearch(ctx context.Context, store *storage.Store, goal string, intent workspaceIntent) (string, error) {
	query := strings.TrimSpace(intent.searchQuery)
	if query == "" {
		query = strings.TrimSpace(goal)
	}
	hits, err := a.tools.SearchWorkspace(ctx, store, query, 20)
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return fmt.Sprintf("No matches for %q in the current workspace.", query), nil
	}
	lines := []string{fmt.Sprintf("Search results for %q:", query)}
	for _, hit := range hits {
		lines = append(lines, fmt.Sprintf("- %s:%d %s", hit.Path, hit.Line, hit.Snippet))
	}
	return strings.Join(lines, "\n"), nil
}

func (a *Agent) executeWorkspaceEdit(ctx context.Context, store *storage.Store, sessionID, nodeID, goal string, intent workspaceIntent) (string, error) {
	if strings.TrimSpace(intent.targetPath) == "" {
		return "", fmt.Errorf("workspace edit task requires a concrete target file")
	}
	if approval, err := store.LoadPendingApproval(sessionID); err == nil && approval != nil {
		if len(approval.Requests) > 0 {
			request, ok := findWorkspaceEditPreviewRequest(approval, nodeID, intent.targetPath)
			if !ok {
				return "", fmt.Errorf("approved workspace edit preview not found for %s; regenerate the permission request", nodeID)
			}
			if summary, applied, applyErr := a.applyWorkspaceEditPreview(store, intent, request); applyErr != nil {
				return summary, applyErr
			} else if applied {
				return summary, nil
			}
			return "", fmt.Errorf("approved workspace edit preview is unavailable for %s; regenerate the permission request", intent.targetPath)
		}
	} else if err != nil {
		return "", err
	}
	content, _, err := a.readWorkspaceFileForEdit(store, intent.targetPath)
	if err != nil {
		return "", err
	}
	rewritten, err := a.workspaceRewriteFile(ctx, intent.targetPath, goal, content)
	if err != nil {
		return "", err
	}
	if err := a.tools.WriteWorkspaceFile(store, intent.targetPath, rewritten.Content); err != nil {
		return "", err
	}
	summary := strings.TrimSpace(rewritten.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Updated %s", intent.targetPath)
	}
	return summary, nil
}

func (a *Agent) executeWorkspaceCommand(store *storage.Store, intent workspaceIntent) (string, error) {
	record, err := a.tools.RunWorkspaceCommand(store, intent.command)
	if err != nil {
		return formatCommandRecord(record), err
	}
	return formatCommandRecord(record), nil
}

func (a *Agent) workspaceSummarizeWorkspace(ctx context.Context, store *storage.Store, goal string, files []protocol.WorkspaceFile) (string, error) {
	summary, err := store.LoadWorkspaceSummary()
	if err != nil {
		return "", err
	}
	preview := make([]string, 0, min(8, len(files)))
	for _, file := range files {
		if workspacePreviewEligible(file) {
			preview = append(preview, file.Path)
		}
		if len(preview) == 8 {
			break
		}
	}
	if !a.workspaceLLMAvailable() {
		return fmt.Sprintf("Workspace %s at %s has %d indexed files (%d text/code, %d paper candidates). Top files: %s",
			summary.Name,
			summary.Root,
			summary.FileCount,
			summary.TextFileCount,
			summary.PaperFileCount,
			strings.Join(preview, ", "),
		), nil
	}
	var contextParts []string
	for _, candidate := range preview[:min(4, len(preview))] {
		content, err := a.tools.ReadWorkspaceFile(store, candidate)
		if err != nil {
			continue
		}
		contextParts = append(contextParts, fmt.Sprintf("File: %s\n%s", candidate, truncateWorkspaceContent(content, 4000)))
	}
	prompt := fmt.Sprintf("Task: %s\nWorkspace: %s\nIndexed files: %d\nPaper candidates: %d\n\nContext:\n%s",
		strings.TrimSpace(goal),
		summary.Root,
		summary.FileCount,
		summary.PaperFileCount,
		strings.Join(contextParts, "\n\n"),
	)
	return a.runWorkspacePrompt(ctx, "Summarize the workspace for the user. Be concise and actionable.", prompt)
}

func (a *Agent) workspaceSummarizeFile(ctx context.Context, store *storage.Store, goal, path, content string) (string, error) {
	if !a.workspaceLLMAvailable() {
		return fmt.Sprintf("Read %s\n\n%s", path, truncateWorkspaceContent(content, 1600)), nil
	}
	prompt := fmt.Sprintf("Task: %s\nFile: %s\n\nContent:\n%s", strings.TrimSpace(goal), path, truncateWorkspaceContent(content, 12000))
	return a.runWorkspacePrompt(ctx, "Summarize or explain the file content for the user using the provided task.", prompt)
}

func (a *Agent) workspaceRewriteFile(ctx context.Context, path, goal, content string) (workspaceRewriteResponse, error) {
	if !a.workspaceLLMAvailable() {
		return workspaceRewriteResponse{}, fmt.Errorf("workspace file editing requires a configured provider/model")
	}
	system := "You edit a workspace file. Return only strict JSON with fields summary and content. Preserve unrelated structure."
	user := fmt.Sprintf("Task: %s\nTarget file: %s\n\nCurrent content:\n%s", strings.TrimSpace(goal), path, truncateWorkspaceContent(content, 16000))
	text, err := a.runWorkspacePrompt(ctx, system, user)
	if err != nil {
		return workspaceRewriteResponse{}, err
	}
	var out workspaceRewriteResponse
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &out); err != nil {
		return workspaceRewriteResponse{}, fmt.Errorf("workspace rewrite response was not valid JSON: %w", err)
	}
	if strings.TrimSpace(out.Content) == "" {
		return workspaceRewriteResponse{}, fmt.Errorf("workspace rewrite returned empty content")
	}
	return out, nil
}

func (a *Agent) runWorkspacePrompt(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	model, err := providers.BuildChatModel(ctx, a.cfg.ActiveProviderConfig(), a.cfg.ProviderTimeout())
	if err != nil {
		return "", err
	}
	msg, err := model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(userPrompt),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(msg.Content), nil
}

func (a *Agent) workspaceLLMAvailable() bool {
	provider := a.cfg.ActiveProviderConfig()
	if strings.TrimSpace(provider.Model) == "" {
		return false
	}
	switch provider.Provider {
	case "ollama":
		return strings.TrimSpace(provider.BaseURL) != ""
	default:
		return strings.TrimSpace(provider.APIKey) != ""
	}
}

func truncateWorkspaceContent(content string, limit int) string {
	content = strings.TrimSpace(content)
	if limit <= 0 || len(content) <= limit {
		return content
	}
	return strings.TrimSpace(content[:limit]) + "\n...[truncated]"
}

func formatCommandRecord(record protocol.WorkspaceCommandRecord) string {
	lines := []string{fmt.Sprintf("$ %s", record.Command)}
	if strings.TrimSpace(record.Stdout) != "" {
		lines = append(lines, strings.TrimSpace(record.Stdout))
	}
	if strings.TrimSpace(record.Stderr) != "" {
		lines = append(lines, strings.TrimSpace(record.Stderr))
	}
	lines = append(lines, fmt.Sprintf("exit=%d", record.ExitCode))
	return strings.Join(lines, "\n")
}

func workspacePreviewEligible(file protocol.WorkspaceFile) bool {
	switch file.Kind {
	case protocol.WorkspaceFileKindText, protocol.WorkspaceFileKindCode, protocol.WorkspaceFileKindConfig:
		return true
	default:
		return strings.EqualFold(filepathBase(file.Path), "README.md")
	}
}

func filepathBase(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

func filepathExt(path string) string {
	base := filepathBase(path)
	if idx := strings.LastIndex(base, "."); idx >= 0 {
		return base[idx:]
	}
	return ""
}

func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return text
}
