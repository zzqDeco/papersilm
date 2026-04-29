package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

const (
	permissionModeTask = "task"

	permissionValueAcceptOnce    = "accept-once"
	permissionValueAcceptSession = "accept-session"
	permissionValueReject        = "reject"

	permissionScopeNode          = "node"
	permissionScopePath          = "path"
	permissionScopeDirectory     = "directory"
	permissionScopeCommandPrefix = "command-prefix"
	permissionScopeSession       = "session"
)

func (a *Agent) buildApprovalRequest(
	ctx context.Context,
	store *storage.Store,
	sessionID string,
	meta protocol.SessionMeta,
	planResult protocol.PlanResult,
	execState *protocol.ExecutionState,
	nodeIDs []string,
	checkpointID string,
	interruptID string,
) (protocol.ApprovalRequest, error) {
	requests := make([]protocol.PermissionRequest, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node, ok := dagNode(planResult.DAG, nodeID)
		if !ok {
			continue
		}
		requests = append(requests, a.permissionRequestForNode(ctx, store, sessionID, meta, planResult, node, interruptID))
	}
	activeRequestID := ""
	if len(requests) > 0 {
		activeRequestID = requests[0].RequestID
	}
	return protocol.ApprovalRequest{
		PlanID:          planResult.PlanID,
		CheckpointID:    checkpointID,
		InterruptID:     interruptID,
		PendingNodeIDs:  append([]string(nil), nodeIDs...),
		Summary:         approvalSummary(planResult, nodeIDs),
		RequiresInput:   true,
		CreatedAt:       time.Now().UTC(),
		Mode:            permissionModeTask,
		ActiveRequestID: activeRequestID,
		Requests:        requests,
	}, nil
}

func (a *Agent) permissionRequestForNode(
	ctx context.Context,
	store *storage.Store,
	sessionID string,
	meta protocol.SessionMeta,
	planResult protocol.PlanResult,
	node protocol.PlanNode,
	interruptID string,
) protocol.PermissionRequest {
	task := taskCardForNode(store, sessionID, node.ID)
	title := permissionTitle(node.Kind)
	subtitle := strings.TrimSpace(task.Title)
	if subtitle == "" {
		subtitle = strings.TrimSpace(node.Goal)
	}
	request := protocol.PermissionRequest{
		RequestID: fmt.Sprintf("%s:%s", interruptID, node.ID),
		SessionID: sessionID,
		PlanID:    planResult.PlanID,
		NodeID:    node.ID,
		Tool:      string(node.Kind),
		Operation: permissionOperation(node.Kind),
		Title:     title,
		Subtitle:  subtitle,
		Question:  permissionQuestion(node.Kind, subtitle),
		Summary:   strings.TrimSpace(node.Goal),
		Options:   permissionOptionsForNode(node),
		CreatedAt: time.Now().UTC(),
	}
	switch node.Kind {
	case protocol.NodeKindWorkspaceEdit:
		files, err := a.tools.LoadWorkspaceFiles(store)
		if err != nil {
			request.Preview = protocol.PermissionPreview{Kind: "error", Summary: err.Error()}
			return request
		}
		intent := inferWorkspaceIntent(node.Goal, files)
		request.TargetPath = intent.targetPath
		request.Subtitle = firstNonEmpty(intent.targetPath, request.Subtitle)
		request.Question = permissionQuestion(node.Kind, firstNonEmpty(intent.targetPath, subtitle))
		request.Preview = a.prepareWorkspaceEditPreview(ctx, store, node.Goal, intent.targetPath)
	case protocol.NodeKindWorkspaceCommand:
		files, err := a.tools.LoadWorkspaceFiles(store)
		if err != nil {
			request.Preview = protocol.PermissionPreview{Kind: "error", Summary: err.Error()}
			return request
		}
		intent := inferWorkspaceIntent(node.Goal, files)
		request.Command = intent.command
		request.Subtitle = firstNonEmpty(intent.command, request.Subtitle)
		request.Question = "Do you want to run this command?"
		request.Preview = protocol.PermissionPreview{
			Kind:          "command",
			Summary:       fmt.Sprintf("cwd: %s", store.WorkspaceRoot()),
			CommandPrefix: shellCommandPrefix(intent.command),
		}
	default:
		request.Preview = protocol.PermissionPreview{
			Kind:    "task",
			Summary: fmt.Sprintf("%s · %s", node.Kind, firstNonEmpty(node.Goal, task.Description)),
		}
	}
	return request
}

func taskCardForNode(store *storage.Store, sessionID, nodeID string) protocol.TaskCard {
	snapshot, err := store.Snapshot(sessionID)
	if err != nil || snapshot.TaskBoard == nil {
		return protocol.TaskCard{}
	}
	for _, task := range snapshot.TaskBoard.Tasks {
		if task.NodeID == nodeID || task.TaskID == nodeID {
			return task
		}
	}
	return protocol.TaskCard{}
}

func permissionTitle(kind protocol.NodeKind) string {
	switch kind {
	case protocol.NodeKindWorkspaceEdit:
		return "Edit file"
	case protocol.NodeKindWorkspaceCommand:
		return "Run command"
	case protocol.NodeKindWorkspaceSearch, protocol.NodeKindWorkspaceInspect:
		return "Inspect workspace"
	default:
		return "Run task"
	}
}

func permissionOperation(kind protocol.NodeKind) string {
	switch kind {
	case protocol.NodeKindWorkspaceEdit:
		return "write"
	case protocol.NodeKindWorkspaceCommand:
		return "shell"
	case protocol.NodeKindWorkspaceSearch, protocol.NodeKindWorkspaceInspect:
		return "read"
	default:
		return "plan"
	}
}

func permissionQuestion(kind protocol.NodeKind, target string) string {
	target = strings.TrimSpace(target)
	switch kind {
	case protocol.NodeKindWorkspaceEdit:
		if target != "" {
			return fmt.Sprintf("Do you want to make this edit to %s?", filepath.Base(target))
		}
		return "Do you want to make this workspace edit?"
	case protocol.NodeKindWorkspaceCommand:
		return "Do you want to run this command?"
	default:
		return "Do you want to run this task?"
	}
}

func permissionOptionsForNode(node protocol.PlanNode) []protocol.PermissionOption {
	sessionLabel := "Yes, during this session"
	sessionScope := permissionScopeSession
	switch node.Kind {
	case protocol.NodeKindWorkspaceEdit:
		sessionLabel = "Yes, allow edits to this file during this session"
		sessionScope = permissionScopePath
	case protocol.NodeKindWorkspaceCommand:
		sessionLabel = "Yes, and do not ask again for this command prefix"
		sessionScope = permissionScopeCommandPrefix
	}
	return []protocol.PermissionOption{
		{Value: permissionValueAcceptOnce, Label: "Yes", Scope: permissionScopeNode, Feedback: "accept"},
		{Value: permissionValueAcceptSession, Label: sessionLabel, Scope: sessionScope, Feedback: "accept"},
		{Value: permissionValueReject, Label: "No", Scope: permissionScopeNode, Feedback: "reject"},
	}
}

func (a *Agent) prepareWorkspaceEditPreview(ctx context.Context, store *storage.Store, goal, targetPath string) protocol.PermissionPreview {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return protocol.PermissionPreview{Kind: "error", Summary: "workspace edit requires a concrete target file"}
	}
	content, err := a.tools.ReadWorkspaceFile(store, targetPath)
	if err != nil {
		return protocol.PermissionPreview{Kind: "error", Summary: err.Error()}
	}
	rewritten, err := a.workspaceRewriteFile(ctx, targetPath, goal, content)
	if err != nil {
		return protocol.PermissionPreview{Kind: "error", Summary: err.Error()}
	}
	return protocol.PermissionPreview{
		Kind:           "diff",
		Summary:        firstNonEmpty(strings.TrimSpace(rewritten.Summary), fmt.Sprintf("Update %s", targetPath)),
		Diff:           compactUnifiedDiff(targetPath, content, rewritten.Content),
		OldContentHash: contentHash(content),
		NewContent:     rewritten.Content,
	}
}

func (a *Agent) applyWorkspaceEditPreview(store *storage.Store, intent workspaceIntent, request protocol.PermissionRequest) (string, bool, error) {
	if strings.TrimSpace(request.TargetPath) == "" || request.Preview.Kind != "diff" || request.Preview.NewContent == "" {
		return "", false, nil
	}
	current, err := a.tools.ReadWorkspaceFile(store, request.TargetPath)
	if err != nil {
		return "", true, err
	}
	if got := contentHash(current); request.Preview.OldContentHash != "" && got != request.Preview.OldContentHash {
		return "", true, fmt.Errorf("file changed since approval preview was created: %s", request.TargetPath)
	}
	if err := a.tools.WriteWorkspaceFile(store, request.TargetPath, request.Preview.NewContent); err != nil {
		return "", true, err
	}
	return firstNonEmpty(request.Preview.Summary, fmt.Sprintf("Updated %s", request.TargetPath)), true, nil
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func compactUnifiedDiff(path, oldContent, newContent string) string {
	if oldContent == newContent {
		return fmt.Sprintf("--- %s\n+++ %s\n", path, path)
	}
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	const maxLines = 80
	lines := []string{fmt.Sprintf("--- %s", path), fmt.Sprintf("+++ %s", path)}
	i, j := 0, 0
	for (i < len(oldLines) || j < len(newLines)) && len(lines) < maxLines {
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

func shellCommandPrefix(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	if len(fields) == 1 {
		return fields[0]
	}
	return strings.Join(fields[:2], " ")
}

func findPermissionRequest(approval *protocol.ApprovalRequest, requestID string) (protocol.PermissionRequest, bool) {
	if approval == nil {
		return protocol.PermissionRequest{}, false
	}
	if strings.TrimSpace(requestID) == "" {
		requestID = approval.ActiveRequestID
	}
	for _, request := range approval.Requests {
		if request.RequestID == requestID {
			return request, true
		}
	}
	return protocol.PermissionRequest{}, false
}

func permissionRuleForDecision(request protocol.PermissionRequest, decision protocol.PermissionDecision) protocol.PermissionRule {
	scope := firstNonEmpty(decision.Scope, permissionScopeSession)
	rule := protocol.PermissionRule{
		RuleID:    fmt.Sprintf("rule_%d", time.Now().UnixNano()),
		Tool:      request.Tool,
		Operation: request.Operation,
		Scope:     scope,
		NodeKind:  request.Tool,
		CreatedAt: time.Now().UTC(),
	}
	switch scope {
	case permissionScopePath:
		rule.TargetPath = request.TargetPath
	case permissionScopeDirectory:
		if request.TargetPath != "" {
			rule.Directory = filepath.Dir(request.TargetPath)
		}
	case permissionScopeCommandPrefix:
		rule.CommandPrefix = firstNonEmpty(request.Preview.CommandPrefix, shellCommandPrefix(request.Command))
	case permissionScopeNode:
		rule.NodeKind = request.Tool
	case permissionScopeSession:
		// Tool + operation already limit this rule to a concrete class.
	}
	return rule
}

func permissionAllowedByRule(request protocol.PermissionRequest, rule protocol.PermissionRule) bool {
	if rule.Tool != "" && rule.Tool != request.Tool {
		return false
	}
	if rule.Operation != "" && request.Operation != "" && rule.Operation != request.Operation {
		return false
	}
	switch rule.Scope {
	case permissionScopePath:
		return rule.TargetPath != "" && rule.TargetPath == request.TargetPath
	case permissionScopeDirectory:
		if rule.Directory == "" || request.TargetPath == "" {
			return false
		}
		rel, err := filepath.Rel(rule.Directory, request.TargetPath)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	case permissionScopeCommandPrefix:
		return rule.CommandPrefix != "" && strings.HasPrefix(request.Command, rule.CommandPrefix)
	case permissionScopeNode:
		return rule.NodeKind != "" && rule.NodeKind == request.Tool
	case permissionScopeSession:
		return true
	default:
		return false
	}
}

func permissionAllowedBySessionRules(request protocol.PermissionRequest, rules []protocol.PermissionRule) bool {
	for _, rule := range rules {
		if permissionAllowedByRule(request, rule) {
			return true
		}
	}
	return false
}

func (a *Agent) DecidePermission(ctx context.Context, store *storage.Store, sink EventSink, sessionID string, decision protocol.PermissionDecision) (protocol.RunResult, error) {
	approval, err := store.LoadPendingApproval(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	request, ok := findPermissionRequest(approval, decision.RequestID)
	if !ok {
		return protocol.RunResult{}, fmt.Errorf("permission request not found")
	}
	if strings.TrimSpace(decision.Value) == "" {
		decision.Value = permissionValueAcceptOnce
	}
	if decision.Value == permissionValueAcceptSession {
		rule := permissionRuleForDecision(request, decision)
		if err := store.AddPermissionRule(sessionID, rule); err != nil {
			return protocol.RunResult{}, err
		}
	}

	var result protocol.RunResult
	switch decision.Value {
	case permissionValueAcceptOnce, permissionValueAcceptSession:
		if request.NodeID != "" {
			result, err = a.ApproveTask(ctx, store, sink, sessionID, request.NodeID, true, strings.TrimSpace(decision.Feedback))
		} else {
			result, err = a.Approve(ctx, store, sink, sessionID, true, strings.TrimSpace(decision.Feedback))
		}
	case permissionValueReject:
		if request.NodeID != "" {
			result, err = a.ApproveTask(ctx, store, sink, sessionID, request.NodeID, false, strings.TrimSpace(decision.Feedback))
		} else {
			result, err = a.Approve(ctx, store, sink, sessionID, false, strings.TrimSpace(decision.Feedback))
		}
	default:
		return protocol.RunResult{}, fmt.Errorf("unknown permission decision: %s", decision.Value)
	}
	if err != nil {
		return protocol.RunResult{}, err
	}
	if decision.Value == permissionValueAcceptSession {
		return a.applyAutoPermissionRules(ctx, store, sink, result)
	}
	return result, nil
}

func (a *Agent) applyAutoPermissionRules(ctx context.Context, store *storage.Store, sink EventSink, result protocol.RunResult) (protocol.RunResult, error) {
	for {
		approval := result.Session.Approval
		request, ok := findPermissionRequest(approval, "")
		if !ok {
			return result, nil
		}
		rules, err := store.LoadPermissionRules(result.Session.Meta.SessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		if !permissionAllowedBySessionRules(request, rules) {
			return result, nil
		}
		next, err := a.ApproveTask(ctx, store, sink, result.Session.Meta.SessionID, request.NodeID, true, "allowed by session rule")
		if err != nil {
			return protocol.RunResult{}, err
		}
		result = next
	}
}
