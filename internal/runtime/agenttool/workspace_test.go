package agenttool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/pipeline"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/internal/tools"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestPersistAcceptSessionRulePropagatesStoreErrors(t *testing.T) {
	t.Parallel()

	baseFile := filepath.Join(t.TempDir(), "store-file")
	if err := os.WriteFile(baseFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := WorkspaceToolsConfig{Store: storage.New(baseFile), SessionID: "sess_test"}
	request := protocol.PermissionRequest{
		Tool:       string(protocol.NodeKindWorkspaceEdit),
		Operation:  "write",
		TargetPath: "README.md",
	}
	decision := protocol.PermissionDecision{Value: PermissionAcceptSession, Scope: PermissionScopePath}

	err := persistAcceptSessionRule(cfg, request, decision)
	if err == nil {
		t.Fatalf("expected permission rule persistence failure")
	}
	if !strings.Contains(err.Error(), "save session permission rule") {
		t.Fatalf("expected wrapped persistence error, got %v", err)
	}
}

func TestPersistAcceptSessionRuleIgnoresNonSessionDecision(t *testing.T) {
	t.Parallel()

	cfg := WorkspaceToolsConfig{Store: storage.New(filepath.Join(t.TempDir(), "store-file")), SessionID: "sess_test"}
	decision := protocol.PermissionDecision{Value: PermissionAcceptOnce}
	if err := persistAcceptSessionRule(cfg, protocol.PermissionRequest{}, decision); err != nil {
		t.Fatalf("accept-once decision should not persist a session rule: %v", err)
	}
}

func TestReplaceTextPermissionAppliesLocalizedEdit(t *testing.T) {
	t.Parallel()

	cfg := newWorkspaceToolTestConfig(t)
	if err := cfg.Store.WriteWorkspaceFile("README.md", "hello typo world\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile: %v", err)
	}
	request, conflict, err := BuildReplaceTextPermission(cfg, ReplaceTextInput{
		Path:    "README.md",
		OldText: "typo",
		NewText: "type",
		Summary: "fix typo",
	})
	if err != nil {
		t.Fatalf("BuildReplaceTextPermission: %v", err)
	}
	if conflict != nil {
		t.Fatalf("did not expect conflict: %+v", conflict)
	}
	if request.Preview.OldText != "typo" || request.Preview.NewText != "type" {
		t.Fatalf("expected replace preview to retain old/new text: %+v", request.Preview)
	}
	if !strings.Contains(request.Preview.Diff, "-hello typo world") || !strings.Contains(request.Preview.Diff, "+hello type world") {
		t.Fatalf("expected localized diff preview, got:\n%s", request.Preview.Diff)
	}

	result, err := applyReplaceText(cfg, request)
	if err != nil {
		t.Fatalf("applyReplaceText: %v", err)
	}
	if result.Status != "completed" || !result.Changed || result.TargetPath != "README.md" {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	content, err := cfg.Store.ReadWorkspaceFile("README.md")
	if err != nil {
		t.Fatalf("ReadWorkspaceFile: %v", err)
	}
	if content != "hello type world\n" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestReplaceTextPermissionReportsMissingOldTextAsConflict(t *testing.T) {
	t.Parallel()

	cfg := newWorkspaceToolTestConfig(t)
	if err := cfg.Store.WriteWorkspaceFile("README.md", "hello world\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile: %v", err)
	}
	request, conflict, err := BuildReplaceTextPermission(cfg, ReplaceTextInput{
		Path:    "README.md",
		OldText: "missing",
		NewText: "type",
	})
	if err != nil {
		t.Fatalf("BuildReplaceTextPermission: %v", err)
	}
	if request.RequestID != "" {
		t.Fatalf("conflict should not produce a permission request: %+v", request)
	}
	if conflict == nil || conflict.Status != "conflict" || !conflict.Conflict || conflict.Changed {
		t.Fatalf("expected conflict result, got %+v", conflict)
	}
}

func TestReplaceTextPermissionReportsDuplicateOldTextAsConflict(t *testing.T) {
	t.Parallel()

	cfg := newWorkspaceToolTestConfig(t)
	if err := cfg.Store.WriteWorkspaceFile("README.md", "typo once\ntypo twice\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile: %v", err)
	}
	request, conflict, err := BuildReplaceTextPermission(cfg, ReplaceTextInput{
		Path:    "README.md",
		OldText: "typo",
		NewText: "type",
	})
	if err != nil {
		t.Fatalf("BuildReplaceTextPermission: %v", err)
	}
	if request.RequestID != "" {
		t.Fatalf("conflict should not produce a permission request: %+v", request)
	}
	if conflict == nil || conflict.Status != "conflict" || !conflict.Conflict || !strings.Contains(conflict.Summary, "not unique") {
		t.Fatalf("expected duplicate match conflict, got %+v", conflict)
	}
}

func TestReplaceTextPermissionDetectsHashConflictOnApply(t *testing.T) {
	t.Parallel()

	cfg := newWorkspaceToolTestConfig(t)
	if err := cfg.Store.WriteWorkspaceFile("README.md", "hello typo world\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile: %v", err)
	}
	request, conflict, err := BuildReplaceTextPermission(cfg, ReplaceTextInput{
		Path:    "README.md",
		OldText: "typo",
		NewText: "type",
	})
	if err != nil {
		t.Fatalf("BuildReplaceTextPermission: %v", err)
	}
	if conflict != nil {
		t.Fatalf("did not expect conflict: %+v", conflict)
	}
	if err := cfg.Store.WriteWorkspaceFile("README.md", "hello changed typo world\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile(conflict): %v", err)
	}
	result, err := applyReplaceText(cfg, request)
	if err != nil {
		t.Fatalf("applyReplaceText should return a structured conflict, got err=%v", err)
	}
	if result.Status != "conflict" || !result.Conflict || result.Changed {
		t.Fatalf("expected conflict result, got %+v", result)
	}
	content, err := cfg.Store.ReadWorkspaceFile("README.md")
	if err != nil {
		t.Fatalf("ReadWorkspaceFile: %v", err)
	}
	if content != "hello changed typo world\n" {
		t.Fatalf("conflict should not change content, got %q", content)
	}
}

func TestReplaceTextPermissionDetectsDuplicateOldTextOnApply(t *testing.T) {
	t.Parallel()

	cfg := newWorkspaceToolTestConfig(t)
	if err := cfg.Store.WriteWorkspaceFile("README.md", "hello typo world\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile: %v", err)
	}
	request, conflict, err := BuildReplaceTextPermission(cfg, ReplaceTextInput{
		Path:    "README.md",
		OldText: "typo",
		NewText: "type",
	})
	if err != nil {
		t.Fatalf("BuildReplaceTextPermission: %v", err)
	}
	if conflict != nil {
		t.Fatalf("did not expect conflict: %+v", conflict)
	}
	request.Preview.OldContentHash = ""
	if err := cfg.Store.WriteWorkspaceFile("README.md", "typo once\ntypo twice\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile(duplicate): %v", err)
	}
	result, err := applyReplaceText(cfg, request)
	if err != nil {
		t.Fatalf("applyReplaceText should return a structured conflict, got err=%v", err)
	}
	if result.Status != "conflict" || !result.Conflict || !strings.Contains(result.Summary, "not unique") {
		t.Fatalf("expected duplicate conflict result, got %+v", result)
	}
}

func TestReplaceTextPermissionRejectsEscapedPath(t *testing.T) {
	t.Parallel()

	cfg := newWorkspaceToolTestConfig(t)
	if _, _, err := BuildReplaceTextPermission(cfg, ReplaceTextInput{
		Path:    "../outside.txt",
		OldText: "old",
		NewText: "new",
	}); err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("expected workspace escape error, got %v", err)
	}
}

func TestReplaceSessionRuleDoesNotAllowWholeFileWrite(t *testing.T) {
	t.Parallel()

	cfg := newWorkspaceToolTestConfig(t)
	if err := cfg.Store.CreateSession(protocol.SessionMeta{SessionID: cfg.SessionID}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := cfg.Store.WriteWorkspaceFile("README.md", "hello typo world\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile: %v", err)
	}
	replaceRequest, conflict, err := BuildReplaceTextPermission(cfg, ReplaceTextInput{
		Path:    "README.md",
		OldText: "typo",
		NewText: "type",
	})
	if err != nil {
		t.Fatalf("BuildReplaceTextPermission: %v", err)
	}
	if conflict != nil {
		t.Fatalf("did not expect conflict: %+v", conflict)
	}
	if err := AddPermissionRule(cfg.Store, cfg.SessionID, replaceRequest, protocol.PermissionDecision{
		Value: PermissionAcceptSession,
		Scope: PermissionScopePath,
	}); err != nil {
		t.Fatalf("AddPermissionRule: %v", err)
	}
	writeRequest, err := BuildWritePermission(cfg, WriteFileInput{
		Path:    "README.md",
		Content: "whole rewrite\n",
	})
	if err != nil {
		t.Fatalf("BuildWritePermission: %v", err)
	}
	if requestAllowedByRules(cfg.Store, cfg.SessionID, writeRequest) {
		t.Fatalf("replace session rule should not auto-allow whole-file write")
	}
	if !requestAllowedByRules(cfg.Store, cfg.SessionID, replaceRequest) {
		t.Fatalf("replace session rule should still auto-allow matching replace request")
	}
}

func TestWorkspaceRunCommandNonZeroExitReturnsStructuredToolResult(t *testing.T) {
	t.Parallel()

	cfg := newWorkspaceToolTestConfig(t)
	cfg.PermissionMode = protocol.PermissionModeAuto
	if err := cfg.Store.CreateSession(protocol.SessionMeta{SessionID: cfg.SessionID}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	invokable := findWorkspaceInvokableTool(t, cfg, "workspace_run_command")
	raw, err := invokable.InvokableRun(context.Background(), `{"command":"printf stdout; printf stderr >&2; exit 7","summary":"nonzero smoke"}`)
	if err != nil {
		t.Fatalf("InvokableRun should not fail the agent turn for non-zero exit: %v", err)
	}
	var result ToolResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal tool result %q: %v", raw, err)
	}
	if result.Status != "failed" {
		t.Fatalf("expected failed tool result, got %+v", result)
	}
	if result.ExitCode == nil || *result.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %+v", result.ExitCode)
	}
	if result.Stdout != "stdout" || result.Stderr != "stderr" || !strings.Contains(result.Summary, "command exited 7") {
		t.Fatalf("expected structured stdout/stderr summary, got %+v", result)
	}
	toolCalls, err := os.ReadFile(filepath.Join(cfg.Store.SessionDir(cfg.SessionID), "tool_calls.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile(tool_calls): %v", err)
	}
	var recorded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(toolCalls))), &recorded); err != nil {
		t.Fatalf("unmarshal tool_calls record %q: %v", string(toolCalls), err)
	}
	for key, want := range map[string]any{
		"tool":               "workspace_run_command",
		"tool_result_status": "failed",
		"command":            "printf stdout; printf stderr >&2; exit 7",
		"cwd":                cfg.Store.WorkspaceRoot(),
		"exit_code":          float64(7),
		"stdout":             "stdout",
		"stderr":             "stderr",
		"changed":            false,
		"conflict":           false,
	} {
		if got := recorded[key]; got != want {
			t.Fatalf("recorded[%s] = %#v, want %#v; record=%+v", key, got, want, recorded)
		}
	}
}

func TestWorkspaceReadOnlyToolsRecordFullToolCalls(t *testing.T) {
	t.Parallel()

	cfg := newWorkspaceToolTestConfig(t)
	cfg.PermissionMode = protocol.PermissionModeAuto
	if err := cfg.Store.CreateSession(protocol.SessionMeta{SessionID: cfg.SessionID}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := cfg.Store.WriteWorkspaceFile("README.md", "alpha marker\nsecond line\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile README: %v", err)
	}
	if err := cfg.Store.WriteWorkspaceFile("notes.md", "marker in notes\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile notes: %v", err)
	}

	readTool := findWorkspaceInvokableTool(t, cfg, "workspace_read_file")
	if _, err := readTool.InvokableRun(context.Background(), `{"path":"README.md"}`); err != nil {
		t.Fatalf("read InvokableRun: %v", err)
	}
	searchTool := findWorkspaceInvokableTool(t, cfg, "workspace_search")
	if _, err := searchTool.InvokableRun(context.Background(), `{"query":"marker","limit":10}`); err != nil {
		t.Fatalf("search InvokableRun: %v", err)
	}
	listTool := findWorkspaceInvokableTool(t, cfg, "workspace_list_files")
	if _, err := listTool.InvokableRun(context.Background(), `{"limit":10}`); err != nil {
		t.Fatalf("list InvokableRun: %v", err)
	}

	records := readToolCallRecords(t, cfg)
	byTool := map[string]map[string]any{}
	for _, record := range records {
		byTool[record["tool"].(string)] = record
	}
	readRecord := byTool["workspace_read_file"]
	if readRecord["target_path"] != "README.md" || !strings.Contains(readRecord["output"].(string), "alpha marker") {
		t.Fatalf("read record lost file content: %+v", readRecord)
	}
	searchRecord := byTool["workspace_search"]
	if searchRecord["query"] != "marker" || searchRecord["match_count"] != float64(2) {
		t.Fatalf("search record lost query/count: %+v", searchRecord)
	}
	hits, ok := searchRecord["hits"].([]any)
	if !ok || len(hits) != 2 {
		t.Fatalf("search record lost hits: %+v", searchRecord)
	}
	listRecord := byTool["workspace_list_files"]
	files, ok := listRecord["files"].([]any)
	if !ok || len(files) < 2 {
		t.Fatalf("list record lost files: %+v", listRecord)
	}
	if listRecord["match_count"] != float64(len(files)) {
		t.Fatalf("list record lost file count: %+v", listRecord)
	}
	filePaths := map[string]bool{}
	for _, item := range files {
		file, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("unexpected file record shape: %+v", item)
		}
		filePaths[file["path"].(string)] = true
	}
	if !filePaths["README.md"] || !filePaths["notes.md"] {
		t.Fatalf("list record missing workspace files: %+v", listRecord)
	}
}

func TestWorkspaceReplaceConflictRecordsToolCall(t *testing.T) {
	t.Parallel()

	cfg := newWorkspaceToolTestConfig(t)
	cfg.PermissionMode = protocol.PermissionModeAuto
	if err := cfg.Store.CreateSession(protocol.SessionMeta{SessionID: cfg.SessionID}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := cfg.Store.WriteWorkspaceFile("README.md", "hello world\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile: %v", err)
	}
	replaceTool := findWorkspaceInvokableTool(t, cfg, "workspace_replace_text")
	raw, err := replaceTool.InvokableRun(context.Background(), `{"path":"README.md","old_text":"missing","new_text":"type"}`)
	if err != nil {
		t.Fatalf("replace InvokableRun should return structured conflict: %v", err)
	}
	var result ToolResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal conflict result %q: %v", raw, err)
	}
	if result.Status != "conflict" || !strings.Contains(result.Summary, "target text not found") {
		t.Fatalf("expected conflict reason in tool result, got %+v", result)
	}
	records := readToolCallRecords(t, cfg)
	if len(records) != 1 {
		t.Fatalf("expected one tool call record, got %+v", records)
	}
	record := records[0]
	if record["tool"] != "workspace_replace_text" || record["tool_result_status"] != "conflict" || !strings.Contains(record["summary"].(string), "target text not found") {
		t.Fatalf("conflict record lost reason: %+v", record)
	}
}

func TestCommandExecutionFailureTreatsSignalAsStructuredFailure(t *testing.T) {
	t.Parallel()

	record := protocol.WorkspaceCommandRecord{Command: "kill self", ExitCode: -1}
	if !CommandExecutionFailure(record, errors.New("signal: killed")) {
		t.Fatalf("expected signaled command to be treated as structured command failure")
	}
}

func newWorkspaceToolTestConfig(t *testing.T) WorkspaceToolsConfig {
	t.Helper()
	store := storage.New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	cfg := config.Default()
	cfg.BaseDir = store.BaseDir()
	return WorkspaceToolsConfig{
		Store:          store,
		Registry:       tools.New(pipeline.New(cfg)),
		SessionID:      "sess_tool_test",
		PermissionMode: protocol.PermissionModeConfirm,
	}
}

func findWorkspaceInvokableTool(t *testing.T, cfg WorkspaceToolsConfig, name string) einotool.InvokableTool {
	t.Helper()
	workspaceTools, err := BuildWorkspaceTools(cfg)
	if err != nil {
		t.Fatalf("BuildWorkspaceTools: %v", err)
	}
	for _, candidate := range workspaceTools {
		info, err := candidate.Info(context.Background())
		if err != nil {
			t.Fatalf("Info: %v", err)
		}
		if info.Name != name {
			continue
		}
		invokable, ok := candidate.(einotool.InvokableTool)
		if !ok {
			t.Fatalf("tool %s is not invokable", name)
		}
		return invokable
	}
	t.Fatalf("tool %s not found", name)
	return nil
}

func readToolCallRecords(t *testing.T, cfg WorkspaceToolsConfig) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(cfg.Store.SessionDir(cfg.SessionID), "tool_calls.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile(tool_calls): %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("unmarshal tool_calls line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}
