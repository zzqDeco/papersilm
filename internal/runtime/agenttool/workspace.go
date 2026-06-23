package agenttool

import (
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"

	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/internal/tools"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

const (
	PermissionAcceptOnce    = "accept-once"
	PermissionAcceptSession = "accept-session"
	PermissionReject        = "reject"

	PermissionScopeNode          = "node"
	PermissionScopePath          = "path"
	PermissionScopeDirectory     = "directory"
	PermissionScopeCommandPrefix = "command-prefix"
	PermissionScopeSession       = "session"
)

type WorkspaceToolsConfig struct {
	Store          *storage.Store
	Registry       *tools.Registry
	SessionID      string
	PermissionMode protocol.PermissionMode
}

type ListFilesInput struct {
	Limit int `json:"limit,omitempty" jsonschema_description:"Maximum number of files to return."`
}

type ReadFileInput struct {
	Path string `json:"path" jsonschema_description:"Workspace-relative file path."`
}

type SearchInput struct {
	Query string `json:"query" jsonschema_description:"Text to search in workspace files."`
	Limit int    `json:"limit,omitempty" jsonschema_description:"Maximum number of search results."`
}

type WriteFileInput struct {
	Path    string `json:"path" jsonschema_description:"Workspace-relative file path to create or replace."`
	Content string `json:"content" jsonschema_description:"Complete new file content."`
	Summary string `json:"summary,omitempty" jsonschema_description:"Short summary of the edit."`
}

type ReplaceTextInput struct {
	Path    string `json:"path" jsonschema_description:"Workspace-relative file path to edit."`
	OldText string `json:"old_text" jsonschema_description:"Exact text to replace. Must appear once in the current file."`
	NewText string `json:"new_text" jsonschema_description:"Replacement text."`
	Summary string `json:"summary,omitempty" jsonschema_description:"Short summary of the edit."`
}

type CommandInput struct {
	Command string `json:"command" jsonschema_description:"Shell command to run inside the workspace root."`
	Summary string `json:"summary,omitempty" jsonschema_description:"Why the command is needed."`
}

type ToolResult struct {
	Status          string `json:"status"`
	Summary         string `json:"summary,omitempty"`
	Output          string `json:"output,omitempty"`
	TargetPath      string `json:"target_path,omitempty"`
	Command         string `json:"command,omitempty"`
	Cwd             string `json:"cwd,omitempty"`
	ExitCode        *int   `json:"exit_code,omitempty"`
	Stdout          string `json:"stdout,omitempty"`
	Stderr          string `json:"stderr,omitempty"`
	StdoutTruncated bool   `json:"stdout_truncated,omitempty"`
	StderrTruncated bool   `json:"stderr_truncated,omitempty"`
	Changed         bool   `json:"changed,omitempty"`
	Conflict        bool   `json:"conflict,omitempty"`
}

type writeState struct {
	Request protocol.PermissionRequest `json:"request"`
}

type replaceState struct {
	Request protocol.PermissionRequest `json:"request"`
}

type commandState struct {
	Request protocol.PermissionRequest `json:"request"`
}

func init() {
	gob.Register(protocol.PermissionRequest{})
	gob.Register(&writeState{})
	gob.Register(&replaceState{})
	gob.Register(&commandState{})
	gob.Register(protocol.PermissionDecision{})
}

func BuildWorkspaceTools(cfg WorkspaceToolsConfig) ([]tool.BaseTool, error) {
	out := make([]tool.BaseTool, 0, 6)
	listTool, err := toolutils.InferTool("workspace_list_files", "List indexed files in the current workspace.",
		func(ctx context.Context, input ListFilesInput) ([]protocol.WorkspaceFile, error) {
			files, err := cfg.Registry.LoadWorkspaceFiles(cfg.Store)
			if err != nil {
				return nil, err
			}
			limit := input.Limit
			if limit <= 0 || limit > 80 {
				limit = 80
			}
			if len(files) > limit {
				files = files[:limit]
			}
			return files, nil
		})
	if err != nil {
		return nil, err
	}
	out = append(out, listTool)

	readTool, err := toolutils.InferTool("workspace_read_file", "Read a text/code file from the current workspace.",
		func(ctx context.Context, input ReadFileInput) (*ToolResult, error) {
			content, err := cfg.Registry.ReadWorkspaceFile(cfg.Store, input.Path)
			if err != nil {
				return nil, err
			}
			return &ToolResult{Status: "completed", Summary: input.Path, Output: content}, nil
		})
	if err != nil {
		return nil, err
	}
	out = append(out, readTool)

	searchTool, err := toolutils.InferTool("workspace_search", "Search text/code files in the current workspace.",
		func(ctx context.Context, input SearchInput) ([]protocol.WorkspaceSearchHit, error) {
			limit := input.Limit
			if limit <= 0 || limit > 50 {
				limit = 20
			}
			return cfg.Registry.SearchWorkspace(ctx, cfg.Store, input.Query, limit)
		})
	if err != nil {
		return nil, err
	}
	out = append(out, searchTool)

	writeTool, err := toolutils.InferTool("workspace_write_file", "Create or replace a whole workspace file. Requires permission in confirm mode. Prefer workspace_replace_text for localized edits.",
		func(ctx context.Context, input WriteFileInput) (*ToolResult, error) {
			request, err := BuildWritePermission(cfg, input)
			if err != nil {
				return nil, err
			}
			if cfg.PermissionMode == protocol.PermissionModeConfirm && !requestAllowedByRules(cfg.Store, cfg.SessionID, request) {
				wasInterrupted, hasState, state := tool.GetInterruptState[*writeState](ctx)
				if wasInterrupted && hasState {
					isTarget, hasData, data := tool.GetResumeContext[protocol.PermissionDecision](ctx)
					if !isTarget {
						return nil, tool.StatefulInterrupt(ctx, state.Request, state)
					}
					if !hasData || data.Value == PermissionReject {
						result := &ToolResult{Status: "rejected", Summary: strings.TrimSpace(data.Feedback), TargetPath: state.Request.TargetPath}
						recordWorkspaceToolCall(cfg, "workspace_write_file", state.Request, result)
						return result, nil
					}
					if err := persistAcceptSessionRule(cfg, state.Request, data); err != nil {
						return nil, err
					}
					return applyWrite(cfg, state.Request)
				}
				return nil, tool.StatefulInterrupt(ctx, request, &writeState{Request: request})
			}
			return applyWrite(cfg, request)
		})
	if err != nil {
		return nil, err
	}
	out = append(out, writeTool)

	replaceTool, err := toolutils.InferTool("workspace_replace_text", "Replace exact text in a workspace file. Requires permission in confirm mode and revalidates the file before applying.",
		func(ctx context.Context, input ReplaceTextInput) (*ToolResult, error) {
			request, conflict, err := BuildReplaceTextPermission(cfg, input)
			if err != nil {
				return nil, err
			}
			if conflict != nil {
				return conflict, nil
			}
			if cfg.PermissionMode == protocol.PermissionModeConfirm && !requestAllowedByRules(cfg.Store, cfg.SessionID, request) {
				wasInterrupted, hasState, state := tool.GetInterruptState[*replaceState](ctx)
				if wasInterrupted && hasState {
					isTarget, hasData, data := tool.GetResumeContext[protocol.PermissionDecision](ctx)
					if !isTarget {
						return nil, tool.StatefulInterrupt(ctx, state.Request, state)
					}
					if !hasData || data.Value == PermissionReject {
						result := &ToolResult{Status: "rejected", Summary: strings.TrimSpace(data.Feedback), TargetPath: state.Request.TargetPath}
						recordWorkspaceToolCall(cfg, "workspace_replace_text", state.Request, result)
						return result, nil
					}
					if err := persistAcceptSessionRule(cfg, state.Request, data); err != nil {
						return nil, err
					}
					return applyReplaceText(cfg, state.Request)
				}
				return nil, tool.StatefulInterrupt(ctx, request, &replaceState{Request: request})
			}
			return applyReplaceText(cfg, request)
		})
	if err != nil {
		return nil, err
	}
	out = append(out, replaceTool)

	commandTool, err := toolutils.InferTool("workspace_run_command", "Run a shell command inside the current workspace. Requires permission in confirm mode.",
		func(ctx context.Context, input CommandInput) (*ToolResult, error) {
			request, err := BuildCommandPermission(cfg, input)
			if err != nil {
				return nil, err
			}
			if cfg.PermissionMode == protocol.PermissionModeConfirm && !requestAllowedByRules(cfg.Store, cfg.SessionID, request) {
				wasInterrupted, hasState, state := tool.GetInterruptState[*commandState](ctx)
				if wasInterrupted && hasState {
					isTarget, hasData, data := tool.GetResumeContext[protocol.PermissionDecision](ctx)
					if !isTarget {
						return nil, tool.StatefulInterrupt(ctx, state.Request, state)
					}
					if !hasData || data.Value == PermissionReject {
						result := &ToolResult{Status: "rejected", Summary: strings.TrimSpace(data.Feedback), Command: state.Request.Command}
						recordWorkspaceToolCall(cfg, "workspace_run_command", state.Request, result)
						return result, nil
					}
					if err := persistAcceptSessionRule(cfg, state.Request, data); err != nil {
						return nil, err
					}
					return applyCommand(cfg, state.Request)
				}
				return nil, tool.StatefulInterrupt(ctx, request, &commandState{Request: request})
			}
			return applyCommand(cfg, request)
		})
	if err != nil {
		return nil, err
	}
	out = append(out, commandTool)

	return out, nil
}

func BuildWritePermission(cfg WorkspaceToolsConfig, input WriteFileInput) (protocol.PermissionRequest, error) {
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return protocol.PermissionRequest{}, fmt.Errorf("workspace_write_file requires path")
	}
	oldContent, existed, err := readFileForEdit(cfg.Store, path)
	if err != nil {
		return protocol.PermissionRequest{}, err
	}
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		if existed {
			summary = "Update " + path
		} else {
			summary = "Create " + path
		}
	}
	oldHash := ""
	if existed {
		oldHash = contentHash(oldContent)
	}
	return protocol.PermissionRequest{
		RequestID:  requestID("write", path),
		SessionID:  cfg.SessionID,
		NodeID:     "workspace_write_file",
		Tool:       string(protocol.NodeKindWorkspaceEdit),
		Operation:  "write",
		Title:      "Edit file",
		Subtitle:   path,
		Question:   fmt.Sprintf("Do you want to make this edit to %s?", filepath.Base(path)),
		Summary:    summary,
		TargetPath: path,
		Preview: protocol.PermissionPreview{
			Kind:           "diff",
			Summary:        summary,
			Diff:           CompactUnifiedDiff(path, oldContent, input.Content),
			OldContentHash: oldHash,
			NewContent:     input.Content,
		},
		Options:   PermissionOptions(protocol.NodeKindWorkspaceEdit),
		CreatedAt: time.Now().UTC(),
	}, nil
}

func BuildReplaceTextPermission(cfg WorkspaceToolsConfig, input ReplaceTextInput) (protocol.PermissionRequest, *ToolResult, error) {
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return protocol.PermissionRequest{}, nil, fmt.Errorf("workspace_replace_text requires path")
	}
	oldText := input.OldText
	if oldText == "" {
		return protocol.PermissionRequest{}, nil, fmt.Errorf("workspace_replace_text requires old_text")
	}
	oldContent, existed, err := readFileForEdit(cfg.Store, path)
	if err != nil {
		return protocol.PermissionRequest{}, nil, err
	}
	if !existed {
		return protocol.PermissionRequest{}, conflictResult(path, "file does not exist: "+path), nil
	}
	if conflict := replaceTextMatchConflict(path, oldContent, oldText); conflict != nil {
		return protocol.PermissionRequest{}, conflict, nil
	}
	newContent := strings.Replace(oldContent, oldText, input.NewText, 1)
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		summary = "Replace text in " + path
	}
	return protocol.PermissionRequest{
		RequestID:  requestID("replace", path+"\n"+contentHash(oldContent)+"\n"+oldText+"\n"+input.NewText),
		SessionID:  cfg.SessionID,
		NodeID:     "workspace_replace_text",
		Tool:       string(protocol.NodeKindWorkspaceEdit),
		Operation:  "replace",
		Title:      "Edit file",
		Subtitle:   path,
		Question:   fmt.Sprintf("Do you want to make this edit to %s?", filepath.Base(path)),
		Summary:    summary,
		TargetPath: path,
		Preview: protocol.PermissionPreview{
			Kind:           "diff",
			Summary:        summary,
			Diff:           CompactUnifiedDiff(path, oldContent, newContent),
			OldContentHash: contentHash(oldContent),
			NewContent:     newContent,
			OldText:        oldText,
			NewText:        input.NewText,
		},
		Options:   PermissionOptions(protocol.NodeKindWorkspaceEdit),
		CreatedAt: time.Now().UTC(),
	}, nil, nil
}

func BuildCommandPermission(cfg WorkspaceToolsConfig, input CommandInput) (protocol.PermissionRequest, error) {
	command := strings.TrimSpace(input.Command)
	if command == "" {
		return protocol.PermissionRequest{}, fmt.Errorf("workspace_run_command requires command")
	}
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		summary = command
	}
	return protocol.PermissionRequest{
		RequestID: requestID("cmd", command),
		SessionID: cfg.SessionID,
		NodeID:    "workspace_run_command",
		Tool:      string(protocol.NodeKindWorkspaceCommand),
		Operation: "shell",
		Title:     "Run command",
		Subtitle:  command,
		Question:  "Do you want to run this command?",
		Summary:   summary,
		Command:   command,
		Preview: protocol.PermissionPreview{
			Kind:          "command",
			Summary:       fmt.Sprintf("cwd: %s", workspaceRoot(cfg.Store)),
			CommandPrefix: ShellCommandPrefix(command),
		},
		Options:   PermissionOptions(protocol.NodeKindWorkspaceCommand),
		CreatedAt: time.Now().UTC(),
	}, nil
}

func applyWrite(cfg WorkspaceToolsConfig, request protocol.PermissionRequest) (*ToolResult, error) {
	if request.TargetPath == "" || request.Preview.Kind != "diff" {
		return nil, fmt.Errorf("approved write request is missing diff preview")
	}
	current, existed, err := readFileForEdit(cfg.Store, request.TargetPath)
	if err != nil {
		return nil, err
	}
	if request.Preview.OldContentHash == "" {
		if existed {
			result := conflictResult(request.TargetPath, "file was created since preview was generated: "+request.TargetPath)
			recordWorkspaceToolCall(cfg, "workspace_write_file", request, result)
			return result, nil
		}
	} else if contentHash(current) != request.Preview.OldContentHash {
		result := conflictResult(request.TargetPath, "file changed since preview was generated: "+request.TargetPath)
		recordWorkspaceToolCall(cfg, "workspace_write_file", request, result)
		return result, nil
	}
	if err := cfg.Registry.WriteWorkspaceFile(cfg.Store, request.TargetPath, request.Preview.NewContent); err != nil {
		return nil, err
	}
	result := &ToolResult{Status: "completed", Summary: request.Preview.Summary, TargetPath: request.TargetPath, Changed: true}
	recordWorkspaceToolCall(cfg, "workspace_write_file", request, result)
	return result, nil
}

func applyReplaceText(cfg WorkspaceToolsConfig, request protocol.PermissionRequest) (*ToolResult, error) {
	if request.TargetPath == "" || request.Preview.Kind != "diff" {
		return nil, fmt.Errorf("approved replace request is missing diff preview")
	}
	if request.Preview.OldText == "" {
		return nil, fmt.Errorf("approved replace request is missing old_text")
	}
	current, existed, err := readFileForEdit(cfg.Store, request.TargetPath)
	if err != nil {
		return nil, err
	}
	if !existed {
		result := conflictResult(request.TargetPath, "file does not exist: "+request.TargetPath)
		recordWorkspaceToolCall(cfg, "workspace_replace_text", request, result)
		return result, nil
	}
	if request.Preview.OldContentHash != "" && contentHash(current) != request.Preview.OldContentHash {
		result := conflictResult(request.TargetPath, "file changed since preview was generated: "+request.TargetPath)
		recordWorkspaceToolCall(cfg, "workspace_replace_text", request, result)
		return result, nil
	}
	if result := replaceTextMatchConflict(request.TargetPath, current, request.Preview.OldText); result != nil {
		recordWorkspaceToolCall(cfg, "workspace_replace_text", request, result)
		return result, nil
	}
	if err := cfg.Registry.WriteWorkspaceFile(cfg.Store, request.TargetPath, strings.Replace(current, request.Preview.OldText, request.Preview.NewText, 1)); err != nil {
		return nil, err
	}
	result := &ToolResult{Status: "completed", Summary: request.Preview.Summary, TargetPath: request.TargetPath, Changed: true}
	recordWorkspaceToolCall(cfg, "workspace_replace_text", request, result)
	return result, nil
}

func applyCommand(cfg WorkspaceToolsConfig, request protocol.PermissionRequest) (*ToolResult, error) {
	record, err := cfg.Registry.RunWorkspaceCommand(cfg.Store, request.Command)
	result := commandToolResult(record, err)
	recordWorkspaceToolCall(cfg, "workspace_run_command", request, result)
	if err != nil {
		if CommandExecutionFailure(record, err) {
			return result, nil
		}
		return result, err
	}
	return result, nil
}

func requestAllowedByRules(store *storage.Store, sessionID string, request protocol.PermissionRequest) bool {
	rules, err := store.LoadPermissionRules(sessionID)
	if err != nil {
		return false
	}
	for _, rule := range rules {
		if PermissionAllowedByRule(request, rule) {
			return true
		}
	}
	return false
}

func persistAcceptSessionRule(cfg WorkspaceToolsConfig, request protocol.PermissionRequest, decision protocol.PermissionDecision) error {
	if decision.Value != PermissionAcceptSession {
		return nil
	}
	if err := AddPermissionRule(cfg.Store, cfg.SessionID, request, decision); err != nil {
		return fmt.Errorf("save session permission rule: %w", err)
	}
	return nil
}

func AddPermissionRule(store *storage.Store, sessionID string, request protocol.PermissionRequest, decision protocol.PermissionDecision) error {
	scope := strings.TrimSpace(decision.Scope)
	if scope == "" {
		scope = PermissionScopeSession
	}
	rule := protocol.PermissionRule{
		RuleID:    fmt.Sprintf("rule_%d", time.Now().UnixNano()),
		Tool:      request.Tool,
		Operation: request.Operation,
		Scope:     scope,
		NodeKind:  request.Tool,
		CreatedAt: time.Now().UTC(),
	}
	switch scope {
	case PermissionScopePath:
		rule.TargetPath = request.TargetPath
	case PermissionScopeDirectory:
		if request.TargetPath != "" {
			rule.Directory = filepath.Dir(request.TargetPath)
		}
	case PermissionScopeCommandPrefix:
		rule.CommandPrefix = firstNonEmpty(request.Preview.CommandPrefix, ShellCommandPrefix(request.Command))
	case PermissionScopeNode:
		rule.NodeKind = request.Tool
	}
	return store.AddPermissionRule(sessionID, rule)
}

func PermissionAllowedByRule(request protocol.PermissionRequest, rule protocol.PermissionRule) bool {
	if rule.Tool != "" && rule.Tool != request.Tool {
		return false
	}
	if rule.Operation != "" && request.Operation != "" && rule.Operation != request.Operation {
		return false
	}
	switch rule.Scope {
	case PermissionScopePath:
		return rule.TargetPath != "" && rule.TargetPath == request.TargetPath
	case PermissionScopeDirectory:
		if rule.Directory == "" || request.TargetPath == "" {
			return false
		}
		rel, err := filepath.Rel(rule.Directory, request.TargetPath)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	case PermissionScopeCommandPrefix:
		return commandPrefixMatches(request.Command, rule.CommandPrefix)
	case PermissionScopeNode:
		return rule.NodeKind != "" && rule.NodeKind == request.Tool
	case PermissionScopeSession:
		return true
	default:
		return false
	}
}

func PermissionOptions(kind protocol.NodeKind) []protocol.PermissionOption {
	sessionScope := PermissionScopeSession
	switch kind {
	case protocol.NodeKindWorkspaceEdit:
		sessionScope = PermissionScopePath
	case protocol.NodeKindWorkspaceCommand:
		sessionScope = PermissionScopeCommandPrefix
	}
	return []protocol.PermissionOption{
		{Value: PermissionAcceptOnce, Label: "Yes", Description: "Allow this tool use once", Scope: PermissionScopeNode, Feedback: "accept"},
		{Value: PermissionAcceptSession, Label: "Yes, during this session", Description: "Allow matching tool uses for this session", Scope: sessionScope, Feedback: "accept"},
		{Value: PermissionReject, Label: "No", Description: "Reject this tool use", Scope: PermissionScopeNode, Feedback: "reject"},
	}
}

func CompactUnifiedDiff(path, oldContent, newContent string) string {
	if oldContent == newContent {
		return fmt.Sprintf("--- %s\n+++ %s\n", path, path)
	}
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	lines := []string{fmt.Sprintf("--- %s", path), fmt.Sprintf("+++ %s", path)}
	i, j := 0, 0
	for (i < len(oldLines) || j < len(newLines)) && len(lines) < 80 {
		if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
			i++
			j++
			continue
		}
		if i < len(oldLines) {
			lines = append(lines, "-"+oldLines[i])
			i++
		}
		if j < len(newLines) {
			lines = append(lines, "+"+newLines[j])
			j++
		}
	}
	if i < len(oldLines) || j < len(newLines) {
		lines = append(lines, "... diff truncated ...")
	}
	return strings.Join(lines, "\n")
}

func ShellCommandPrefix(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	if len(fields) == 1 {
		return fields[0]
	}
	return strings.Join(fields[:2], " ")
}

func readFileForEdit(store *storage.Store, targetPath string) (string, bool, error) {
	content, err := store.ReadWorkspaceFile(targetPath)
	if err == nil {
		return content, true, nil
	}
	if os.IsNotExist(err) {
		return "", false, nil
	}
	return "", false, err
}

func conflictResult(path, summary string) *ToolResult {
	return &ToolResult{
		Status:     "conflict",
		Summary:    summary,
		TargetPath: path,
		Conflict:   true,
		Changed:    false,
	}
}

func replaceTextMatchConflict(path, content, oldText string) *ToolResult {
	count := strings.Count(content, oldText)
	switch {
	case count == 0:
		return conflictResult(path, "target text not found in "+path)
	case count > 1:
		return conflictResult(path, fmt.Sprintf("target text is not unique in %s: %d matches", path, count))
	default:
		return nil
	}
}

func commandToolResult(record protocol.WorkspaceCommandRecord, runErr error) *ToolResult {
	status := "completed"
	if runErr != nil || record.ExitCode != 0 {
		status = "failed"
	}
	exitCode := record.ExitCode
	return &ToolResult{
		Status:          status,
		Summary:         formatCommandRecord(record),
		Output:          strings.TrimSpace(strings.Join([]string{record.Stdout, record.Stderr}, "\n")),
		Command:         record.Command,
		Cwd:             record.Cwd,
		ExitCode:        &exitCode,
		Stdout:          record.Stdout,
		Stderr:          record.Stderr,
		StdoutTruncated: record.StdoutTruncated,
		StderrTruncated: record.StderrTruncated,
		Changed:         false,
	}
}

func recordWorkspaceToolCall(cfg WorkspaceToolsConfig, toolName string, request protocol.PermissionRequest, result *ToolResult) {
	if cfg.Store == nil || strings.TrimSpace(cfg.SessionID) == "" || result == nil {
		return
	}
	if _, err := os.Stat(cfg.Store.SessionDir(cfg.SessionID)); err != nil {
		return
	}
	_ = cfg.Store.AppendToolCall(cfg.SessionID, map[string]any{
		"tool":               toolName,
		"request_id":         request.RequestID,
		"target_path":        request.TargetPath,
		"command":            request.Command,
		"cwd":                result.Cwd,
		"summary":            result.Summary,
		"status":             result.Status,
		"tool_result_status": result.Status,
		"changed":            result.Changed,
		"conflict":           result.Conflict,
		"exit_code":          result.ExitCode,
		"stdout":             result.Stdout,
		"stderr":             result.Stderr,
		"stdout_truncated":   result.StdoutTruncated,
		"stderr_truncated":   result.StderrTruncated,
		"diff":               request.Preview.Diff,
		"created_at":         time.Now().UTC(),
	})
}

func CommandExecutionFailure(record protocol.WorkspaceCommandRecord, err error) bool {
	if err == nil || strings.TrimSpace(record.Command) == "" {
		return false
	}
	if record.ExitCode == 0 {
		return false
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "exit status") ||
		strings.Contains(message, "signal:") ||
		strings.Contains(message, "command timed out") ||
		record.ExitCode < 0
}

func workspaceRoot(store *storage.Store) string {
	if store == nil {
		return "current workspace"
	}
	return store.WorkspaceRoot()
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func requestID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(sum[:])[:12])
}

func commandPrefixMatches(command, prefix string) bool {
	commandFields := strings.Fields(command)
	prefixFields := strings.Fields(prefix)
	if len(prefixFields) == 0 || len(commandFields) < len(prefixFields) {
		return false
	}
	for i, field := range prefixFields {
		if commandFields[i] != field {
			return false
		}
	}
	return true
}

func formatCommandRecord(record protocol.WorkspaceCommandRecord) string {
	parts := []string{fmt.Sprintf("command exited %d", record.ExitCode)}
	if strings.TrimSpace(record.Stdout) != "" {
		parts = append(parts, strings.TrimSpace(record.Stdout))
	}
	if strings.TrimSpace(record.Stderr) != "" {
		parts = append(parts, strings.TrimSpace(record.Stderr))
	}
	return strings.Join(parts, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
