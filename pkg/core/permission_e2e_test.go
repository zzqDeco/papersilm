package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestPermissionE2EWorkspaceCommandAcceptOnce(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "run command `printf %s command-smoke`",
		PermissionMode: protocol.PermissionModeConfirm,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(confirm command): %v", err)
	}
	request := requirePermissionRequest(t, planned.Approval, string(protocol.NodeKindWorkspaceCommand))
	if request.Command != "printf %s command-smoke" {
		t.Fatalf("unexpected command request: %+v", request)
	}
	if request.Preview.Kind != "command" || request.Preview.CommandPrefix != "printf %s" {
		t.Fatalf("expected command preview with prefix, got %+v", request.Preview)
	}

	result, err := svc.DecidePermission(ctx, planned.Session.Meta.SessionID, protocol.PermissionDecision{
		RequestID: request.RequestID,
		Value:     "accept-once",
	})
	if err != nil {
		t.Fatalf("DecidePermission(accept-once): %v", err)
	}
	if result.Session.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("expected completed session, got %s", result.Session.Meta.State)
	}
	execState := requireExecutionState(t, svc, result.Session.Meta.SessionID)
	if !executionOutputContains(execState, "command-smoke") {
		t.Fatalf("expected command output in execution state, got %+v", execState.Outputs)
	}
	if rules, err := svc.store.LoadPermissionRules(result.Session.Meta.SessionID); err != nil {
		t.Fatalf("LoadPermissionRules: %v", err)
	} else if len(rules) != 0 {
		t.Fatalf("accept-once should not persist session rules, got %+v", rules)
	}
}

func TestPermissionE2EWorkspaceEditPreviewAcceptAppliesFile(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()
	sessionID := seedPendingPermissionSession(t, svc, "create `notes.md` with smoke content", []protocol.PlanNode{
		permissionPlanNode("edit_notes", protocol.NodeKindWorkspaceEdit, "create `notes.md` with smoke content"),
	}, []protocol.PermissionRequest{
		{
			RequestID:  "req_edit_notes",
			NodeID:     "edit_notes",
			Tool:       string(protocol.NodeKindWorkspaceEdit),
			Operation:  "write",
			Title:      "Edit file",
			Subtitle:   "notes.md",
			Question:   "Do you want to make this edit to notes.md?",
			TargetPath: "notes.md",
			Preview: protocol.PermissionPreview{
				Kind:       "diff",
				Summary:    "Create notes.md",
				Diff:       "--- notes.md\n+++ notes.md\n+permission smoke\n",
				NewContent: "permission smoke\n",
			},
			Options: permissionSmokeOptions(protocol.NodeKindWorkspaceEdit),
		},
	})

	result, err := svc.DecidePermission(ctx, sessionID, protocol.PermissionDecision{
		RequestID: "req_edit_notes",
		Value:     "accept-once",
	})
	if err != nil {
		t.Fatalf("DecidePermission(edit accept): %v", err)
	}
	if result.Session.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("expected completed session, got %s", result.Session.Meta.State)
	}
	content, err := os.ReadFile(filepath.Join(svc.store.WorkspaceRoot(), "notes.md"))
	if err != nil {
		t.Fatalf("ReadFile(notes.md): %v", err)
	}
	if string(content) != "permission smoke\n" {
		t.Fatalf("unexpected notes.md content: %q", string(content))
	}
	execState := requireExecutionState(t, svc, result.Session.Meta.SessionID)
	if !executionOutputContains(execState, "Create notes.md") {
		t.Fatalf("expected edit summary output, got %+v", execState.Outputs)
	}
}

func TestPermissionE2ERejectWithFeedbackDoesNotExecute(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()
	sessionID := seedPendingPermissionSession(t, svc, "run command `printf %s rejected > rejected.txt`", []protocol.PlanNode{
		permissionPlanNode("cmd_reject", protocol.NodeKindWorkspaceCommand, "run command `printf %s rejected > rejected.txt`"),
	}, []protocol.PermissionRequest{
		commandPermissionRequest("req_cmd_reject", "cmd_reject", "printf %s rejected > rejected.txt"),
	})

	result, err := svc.DecidePermission(ctx, sessionID, protocol.PermissionDecision{
		RequestID: "req_cmd_reject",
		Value:     "reject",
		Feedback:  "Use a read-only command first.",
	})
	if err != nil {
		t.Fatalf("DecidePermission(reject): %v", err)
	}
	if result.Session.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("expected completed session after rejection, got %s", result.Session.Meta.State)
	}
	if _, err := os.Stat(filepath.Join(svc.store.WorkspaceRoot(), "rejected.txt")); !os.IsNotExist(err) {
		t.Fatalf("rejected command should not create rejected.txt, stat err=%v", err)
	}
	execState := requireExecutionState(t, svc, sessionID)
	node := requireExecutionNode(t, execState, "cmd_reject")
	if node.Status != protocol.NodeStatusFailed {
		t.Fatalf("expected rejected node failed, got %+v", node)
	}
	if !strings.Contains(node.Error, "Use a read-only command first.") {
		t.Fatalf("expected rejection feedback in node error, got %q", node.Error)
	}
}

func TestPermissionE2EAcceptSessionAutoAllowsMatchingCommandPrefix(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()
	sessionID := seedPendingPermissionSession(t, svc, "run command `printf %s first`", []protocol.PlanNode{
		permissionPlanNode("cmd_first", protocol.NodeKindWorkspaceCommand, "run command `printf %s first`"),
		permissionPlanNode("cmd_second", protocol.NodeKindWorkspaceCommand, "run command `printf %s second`"),
	}, []protocol.PermissionRequest{
		commandPermissionRequest("req_cmd_first", "cmd_first", "printf %s first"),
		commandPermissionRequest("req_cmd_second", "cmd_second", "printf %s second"),
	})

	result, err := svc.DecidePermission(ctx, sessionID, protocol.PermissionDecision{
		RequestID: "req_cmd_first",
		Value:     "accept-session",
		Scope:     "command-prefix",
	})
	if err != nil {
		t.Fatalf("DecidePermission(accept-session): %v", err)
	}
	if result.Session.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("expected auto-allowed session to complete, got %s approval=%+v", result.Session.Meta.State, result.Session.Approval)
	}
	if result.Session.Approval != nil {
		t.Fatalf("expected no remaining approval after auto-allow, got %+v", result.Session.Approval)
	}
	rules, err := svc.store.LoadPermissionRules(sessionID)
	if err != nil {
		t.Fatalf("LoadPermissionRules: %v", err)
	}
	if len(rules) != 1 || rules[0].Scope != "command-prefix" || rules[0].CommandPrefix != "printf %s" {
		t.Fatalf("expected command-prefix session rule, got %+v", rules)
	}
	execState := requireExecutionState(t, svc, sessionID)
	if requireExecutionNode(t, execState, "cmd_first").Status != protocol.NodeStatusCompleted {
		t.Fatalf("first command did not complete: %+v", execState.Nodes)
	}
	if requireExecutionNode(t, execState, "cmd_second").Status != protocol.NodeStatusCompleted {
		t.Fatalf("second command was not auto-allowed: %+v", execState.Nodes)
	}
}

func seedPendingPermissionSession(t *testing.T, svc *Service, goal string, nodes []protocol.PlanNode, requests []protocol.PermissionRequest) string {
	t.Helper()
	if len(nodes) == 0 {
		t.Fatalf("seedPendingPermissionSession requires nodes")
	}
	if len(requests) == 0 {
		t.Fatalf("seedPendingPermissionSession requires requests")
	}
	meta, err := svc.NewSession(protocol.PermissionModeConfirm, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	now := time.Now().UTC()
	planID := "plan_" + meta.SessionID
	checkpointID := "checkpoint_" + meta.SessionID
	interruptID := "approval_" + meta.SessionID
	pendingNodeIDs := make([]string, 0, len(nodes))
	execNodes := make([]protocol.NodeExecutionState, 0, len(nodes))
	for i := range nodes {
		if nodes[i].Status == "" {
			nodes[i].Status = protocol.NodeStatusReady
		}
		if nodes[i].WorkerProfile == "" {
			nodes[i].WorkerProfile = protocol.WorkerProfileSupervisor
		}
		nodes[i].Required = true
		pendingNodeIDs = append(pendingNodeIDs, nodes[i].ID)
		execNodes = append(execNodes, protocol.NodeExecutionState{
			NodeID:        nodes[i].ID,
			WorkerProfile: nodes[i].WorkerProfile,
			Status:        protocol.NodeStatusReady,
		})
	}
	for i := range requests {
		requests[i].SessionID = meta.SessionID
		requests[i].PlanID = planID
		if requests[i].CreatedAt.IsZero() {
			requests[i].CreatedAt = now
		}
		if len(requests[i].Options) == 0 {
			requests[i].Options = permissionSmokeOptions(protocol.NodeKind(requests[i].Tool))
		}
	}
	meta.State = protocol.SessionStateAwaitingApproval
	meta.LastTask = goal
	meta.ApprovalPending = true
	meta.ActivePlanID = planID
	meta.ActiveCheckpointID = checkpointID
	meta.PendingInterruptID = interruptID
	meta.UpdatedAt = now
	if err := svc.store.SaveMeta(meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	if err := svc.store.SavePlan(meta.SessionID, protocol.PlanResult{
		PlanID:           planID,
		Goal:             goal,
		DAG:              protocol.PlanDAG{Nodes: nodes},
		ApprovalRequired: true,
		CreatedAt:        now,
	}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := svc.store.SaveExecutionState(meta.SessionID, protocol.ExecutionState{
		PlanID:         planID,
		CurrentBatchID: "batch_" + meta.SessionID,
		PendingNodeIDs: pendingNodeIDs,
		Nodes:          execNodes,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("SaveExecutionState: %v", err)
	}
	if err := svc.store.SavePendingApproval(meta.SessionID, protocol.ApprovalRequest{
		PlanID:          planID,
		CheckpointID:    checkpointID,
		InterruptID:     interruptID,
		PendingNodeIDs:  pendingNodeIDs,
		Summary:         goal,
		RequiresInput:   true,
		CreatedAt:       now,
		Mode:            "task",
		ActiveRequestID: requests[0].RequestID,
		Requests:        requests,
	}); err != nil {
		t.Fatalf("SavePendingApproval: %v", err)
	}
	return meta.SessionID
}

func permissionPlanNode(id string, kind protocol.NodeKind, goal string) protocol.PlanNode {
	return protocol.PlanNode{
		ID:            id,
		Kind:          kind,
		Goal:          goal,
		WorkerProfile: protocol.WorkerProfileSupervisor,
		Required:      true,
		Status:        protocol.NodeStatusReady,
		ParallelGroup: "workspace",
	}
}

func commandPermissionRequest(requestID, nodeID, command string) protocol.PermissionRequest {
	return protocol.PermissionRequest{
		RequestID: requestID,
		NodeID:    nodeID,
		Tool:      string(protocol.NodeKindWorkspaceCommand),
		Operation: "shell",
		Title:     "Run command",
		Subtitle:  command,
		Question:  "Do you want to run this command?",
		Command:   command,
		Preview: protocol.PermissionPreview{
			Kind:          "command",
			Summary:       "cwd: workspace",
			CommandPrefix: "printf %s",
		},
		Options: permissionSmokeOptions(protocol.NodeKindWorkspaceCommand),
	}
}

func permissionSmokeOptions(kind protocol.NodeKind) []protocol.PermissionOption {
	sessionScope := "session"
	if kind == protocol.NodeKindWorkspaceEdit {
		sessionScope = "path"
	}
	if kind == protocol.NodeKindWorkspaceCommand {
		sessionScope = "command-prefix"
	}
	return []protocol.PermissionOption{
		{Value: "accept-once", Label: "Yes", Scope: "node", Feedback: "accept"},
		{Value: "accept-session", Label: "Yes, during this session", Scope: sessionScope, Feedback: "accept"},
		{Value: "reject", Label: "No", Scope: "node", Feedback: "reject"},
	}
}

func requirePermissionRequest(t *testing.T, approval *protocol.ApprovalRequest, tool string) protocol.PermissionRequest {
	t.Helper()
	if approval == nil {
		t.Fatalf("expected approval")
	}
	for _, request := range approval.Requests {
		if request.Tool == tool {
			return request
		}
	}
	t.Fatalf("expected permission request for %s, got %+v", tool, approval.Requests)
	return protocol.PermissionRequest{}
}

func requireExecutionState(t *testing.T, svc *Service, sessionID string) *protocol.ExecutionState {
	t.Helper()
	execState, err := svc.store.LoadExecutionState(sessionID)
	if err != nil {
		t.Fatalf("LoadExecutionState: %v", err)
	}
	if execState == nil {
		t.Fatalf("expected execution state")
	}
	return execState
}

func requireExecutionNode(t *testing.T, state *protocol.ExecutionState, nodeID string) protocol.NodeExecutionState {
	t.Helper()
	for _, node := range state.Nodes {
		if node.NodeID == nodeID {
			return node
		}
	}
	t.Fatalf("expected execution node %s, got %+v", nodeID, state.Nodes)
	return protocol.NodeExecutionState{}
}

func executionOutputContains(state *protocol.ExecutionState, needle string) bool {
	for _, output := range state.Outputs {
		if strings.Contains(fmt.Sprint(output.Data), needle) {
			return true
		}
	}
	return false
}
