package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/internal/runtime/agenttool"
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

var (
	arxivIDPattern = regexp.MustCompile(`\b(?:\d{4}\.\d{4,5}|[a-z\-]+(?:\.[A-Za-z\-]+)?/\d{7})(?:v\d+)?\b`)
	urlPattern     = regexp.MustCompile(`https?://[^\s]+`)
)

func (r *Runtime) ensurePaperContext(ctx context.Context, sessionID, goal string) ([]protocol.PaperRef, error) {
	refs, err := r.store.LoadSources(sessionID)
	if err != nil {
		return nil, err
	}
	if len(refs) > 0 {
		return refs, nil
	}
	promptSources := extractPromptPaperSources(goal)
	if len(promptSources) == 0 && goalNeedsPaperContext(goal) {
		if candidates, err := r.registry.WorkspacePaperCandidates(r.store); err == nil {
			promptSources = choosePaperCandidates(goal, candidates)
		}
	}
	if len(promptSources) == 0 {
		return refs, nil
	}
	snapshot, err := r.AttachSources(ctx, sessionID, promptSources, false)
	if err != nil {
		return nil, err
	}
	return snapshot.Sources, nil
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
	step := protocol.PlanStep{ID: nodeID, Tool: string(intent.kind), Goal: strings.TrimSpace(goal), ExpectedArtifact: "workspace_response"}
	plan := protocol.PlanResult{
		PlanID:           newPlanID(),
		Goal:             strings.TrimSpace(goal),
		DAG:              protocol.PlanDAG{Nodes: []protocol.PlanNode{node}},
		Steps:            []protocol.PlanStep{step},
		Risks:            []string{"workspace tools may read files, edit files, or run commands depending on permission mode"},
		ApprovalRequired: approvalRequired,
		CreatedAt:        now,
	}
	board := taskBoardForPlan(plan)
	plan.TaskBoard = &board
	return plan
}

func buildPaperPlan(goal string, refs []protocol.PaperRef, approvalRequired bool) protocol.PlanResult {
	now := time.Now().UTC()
	nodes := make([]protocol.PlanNode, 0, len(refs)+1)
	steps := make([]protocol.PlanStep, 0, len(refs)+1)
	extractable := make([]protocol.PaperRef, 0, len(refs))
	for _, ref := range refs {
		if ref.Status == protocol.SourceStatusFailed {
			continue
		}
		extractable = append(extractable, ref)
		nodeID := "distill_" + safeID(ref.PaperID)
		nodes = append(nodes, protocol.PlanNode{
			ID:            nodeID,
			Kind:          protocol.NodeKindPaperSummary,
			Goal:          fmt.Sprintf("Distill %s", ref.PaperID),
			PaperIDs:      []string{ref.PaperID},
			WorkerProfile: protocol.WorkerProfilePaperSummary,
			Required:      true,
			Status:        protocol.NodeStatusReady,
			ParallelGroup: "paper",
		})
		steps = append(steps, protocol.PlanStep{ID: nodeID, Tool: "distill_paper", PaperIDs: []string{ref.PaperID}, Goal: goal, ExpectedArtifact: ref.PaperID})
	}
	if len(extractable) > 1 {
		paperIDs := make([]string, 0, len(extractable))
		deps := make([]string, 0, len(extractable))
		for _, ref := range extractable {
			paperIDs = append(paperIDs, ref.PaperID)
			deps = append(deps, "distill_"+safeID(ref.PaperID))
		}
		nodes = append(nodes, protocol.PlanNode{
			ID:            "compare_papers",
			Kind:          protocol.NodeKindFinalSynthesis,
			Goal:          goal,
			PaperIDs:      paperIDs,
			WorkerProfile: protocol.WorkerProfileMethodCompare,
			DependsOn:     deps,
			Required:      true,
			Status:        protocol.NodeStatusPending,
			ParallelGroup: "compare",
		})
		steps = append(steps, protocol.PlanStep{ID: "compare_papers", Tool: "compare_papers", PaperIDs: paperIDs, Goal: goal, ExpectedArtifact: "comparison"})
	}
	plan := protocol.PlanResult{
		PlanID:           newPlanID(),
		Goal:             strings.TrimSpace(goal),
		SourceSummary:    refs,
		DAG:              protocol.PlanDAG{Nodes: nodes},
		Steps:            steps,
		WillCompare:      len(extractable) > 1,
		Risks:            []string{"paper processing may download or write session artifacts"},
		ApprovalRequired: approvalRequired,
		CreatedAt:        now,
	}
	board := taskBoardForPlan(plan)
	plan.TaskBoard = &board
	return plan
}

func taskBoardForPlan(plan protocol.PlanResult) protocol.TaskBoard {
	tasks := make([]protocol.TaskCard, 0, len(plan.DAG.Nodes))
	for _, node := range plan.DAG.Nodes {
		status := protocol.TaskStatusReady
		if node.Status == protocol.NodeStatusPending {
			status = protocol.TaskStatusBlocked
		}
		tasks = append(tasks, protocol.TaskCard{
			TaskID:      node.ID,
			NodeID:      node.ID,
			Kind:        node.Kind,
			Title:       node.Goal,
			PaperIDs:    node.PaperIDs,
			GroupID:     firstNonEmpty(node.ParallelGroup, "default"),
			Status:      status,
			DependsOn:   node.DependsOn,
			Produces:    node.Produces,
			Description: string(node.WorkerProfile),
			AvailableActions: []protocol.TaskAction{
				{Type: protocol.TaskActionRun, Label: "Run"},
			},
		})
	}
	return protocol.TaskBoard{PlanID: plan.PlanID, Goal: plan.Goal, Tasks: tasks, UpdatedAt: time.Now().UTC()}
}

func (r *Runtime) savePlanState(sessionID string, plan protocol.PlanResult) error {
	if err := r.store.SavePlan(sessionID, plan); err != nil {
		return err
	}
	nodes := make([]protocol.NodeExecutionState, 0, len(plan.DAG.Nodes))
	for _, node := range plan.DAG.Nodes {
		nodes = append(nodes, protocol.NodeExecutionState{NodeID: node.ID, WorkerProfile: node.WorkerProfile, Status: node.Status})
	}
	return r.store.SaveExecutionState(sessionID, protocol.ExecutionState{
		PlanID:    plan.PlanID,
		Nodes:     nodes,
		UpdatedAt: time.Now().UTC(),
	})
}

func findPlanStepForTask(plan protocol.PlanResult, taskID string) (protocol.PlanStep, bool) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return protocol.PlanStep{}, false
	}
	stepIDs := map[string]struct{}{taskID: {}}
	if plan.TaskBoard != nil {
		for _, task := range plan.TaskBoard.Tasks {
			if task.TaskID == taskID || task.NodeID == taskID {
				stepIDs[task.NodeID] = struct{}{}
				stepIDs[task.TaskID] = struct{}{}
			}
		}
	}
	for _, step := range plan.Steps {
		if _, ok := stepIDs[step.ID]; ok {
			return step, true
		}
	}
	return protocol.PlanStep{}, false
}

func (r *Runtime) permissionRequestIDForTask(sessionID, taskID string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", fmt.Errorf("task id is required")
	}
	approval, err := r.store.LoadPendingApproval(sessionID)
	if err != nil {
		return "", err
	}
	if approval == nil {
		return "", fmt.Errorf("session has no pending approval")
	}
	for _, request := range approval.Requests {
		if request.NodeID == taskID || request.RequestID == taskID {
			return request.RequestID, nil
		}
	}
	for _, pendingID := range approval.PendingNodeIDs {
		if pendingID == taskID && len(approval.Requests) == 1 {
			return approval.Requests[0].RequestID, nil
		}
	}
	return "", fmt.Errorf("pending approval does not target task: %s", taskID)
}

func (r *Runtime) markStep(sessionID, planID, nodeID string, status protocol.NodeStatus, errText string, output protocol.NodeOutputRef) error {
	execState, err := r.store.LoadExecutionState(sessionID)
	if err != nil {
		return err
	}
	if execState == nil {
		return nil
	}
	for i := range execState.Nodes {
		if execState.Nodes[i].NodeID != nodeID {
			continue
		}
		execState.Nodes[i].Status = status
		execState.Nodes[i].Error = errText
		if status == protocol.NodeStatusCompleted {
			execState.Nodes[i].CompletedAt = time.Now().UTC()
			execState.Nodes[i].Outputs = append(execState.Nodes[i].Outputs, output)
			execState.Outputs = append(execState.Outputs, output)
		}
	}
	execState.Finalized = executionFinalized(execState.Nodes)
	execState.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveExecutionState(sessionID, *execState); err != nil {
		return err
	}
	plan, err := r.store.LoadPlan(sessionID)
	if err != nil || plan == nil {
		return err
	}
	for i := range plan.DAG.Nodes {
		if plan.DAG.Nodes[i].ID == nodeID {
			plan.DAG.Nodes[i].Status = status
		}
	}
	board := taskBoardForPlan(*plan)
	for i := range board.Tasks {
		if board.Tasks[i].NodeID != nodeID {
			continue
		}
		board.Tasks[i].Status = taskStatusForNode(status)
		board.Tasks[i].Error = errText
	}
	plan.TaskBoard = &board
	return r.store.SavePlan(sessionID, *plan)
}

func (r *Runtime) nextPermissionRequest(ctx context.Context, sessionID string, plan protocol.PlanResult) (protocol.ApprovalRequest, bool, error) {
	for _, step := range plan.Steps {
		switch step.Tool {
		case string(protocol.NodeKindWorkspaceCommand):
			request := commandPermissionRequest(sessionID, plan.PlanID, step.ID, extractBacktickCommand(step.Goal))
			return approvalFromRequest(plan, request, "plan"), true, nil
		case string(protocol.NodeKindWorkspaceEdit):
			files, _ := r.registry.LoadWorkspaceFiles(r.store)
			intent := inferWorkspaceIntent(step.Goal, files)
			request, err := editPermissionRequest(r.store, sessionID, plan.PlanID, step.ID, step.Goal, intent.targetPath)
			if err != nil {
				return protocol.ApprovalRequest{}, false, err
			}
			return approvalFromRequest(plan, request, "plan"), true, nil
		}
		_ = ctx
	}
	if plan.ApprovalRequired {
		return approvalFromRequest(plan, planPermissionRequest(sessionID, plan), "plan"), true, nil
	}
	return protocol.ApprovalRequest{}, false, nil
}

func planPermissionRequest(sessionID string, plan protocol.PlanResult) protocol.PermissionRequest {
	nodeID := "plan"
	if len(plan.Steps) > 0 && strings.TrimSpace(plan.Steps[0].ID) != "" {
		nodeID = plan.Steps[0].ID
	}
	summary := strings.TrimSpace(plan.Goal)
	if summary == "" {
		summary = "Run the planned work"
	}
	return protocol.PermissionRequest{
		RequestID: fmt.Sprintf("plan_%s", safeID(plan.PlanID)),
		SessionID: sessionID,
		PlanID:    plan.PlanID,
		NodeID:    nodeID,
		Tool:      "plan_checkpoint",
		Operation: "plan",
		Title:     "Run plan",
		Question:  "Do you want to run this plan?",
		Summary:   summary,
		Preview: protocol.PermissionPreview{
			Kind:    "plan",
			Summary: summarizePlanSteps(plan),
		},
		Options: []protocol.PermissionOption{
			{Value: agenttool.PermissionAcceptOnce, Label: "Yes", Description: "Run this plan once", Scope: agenttool.PermissionScopeNode, Feedback: "accept"},
			{Value: agenttool.PermissionReject, Label: "No", Description: "Do not run this plan", Scope: agenttool.PermissionScopeNode, Feedback: "reject"},
		},
		CreatedAt: time.Now().UTC(),
	}
}

func summarizePlanSteps(plan protocol.PlanResult) string {
	if len(plan.Steps) == 0 {
		return "No planned steps."
	}
	parts := make([]string, 0, min(4, len(plan.Steps)))
	for _, step := range plan.Steps {
		label := strings.TrimSpace(step.Goal)
		if label == "" {
			label = step.Tool
		}
		parts = append(parts, label)
		if len(parts) == 4 {
			break
		}
	}
	if len(plan.Steps) > len(parts) {
		parts = append(parts, fmt.Sprintf("%d more step(s)", len(plan.Steps)-len(parts)))
	}
	return strings.Join(parts, "\n")
}

func approvalFromRequest(plan protocol.PlanResult, request protocol.PermissionRequest, mode string) protocol.ApprovalRequest {
	if strings.TrimSpace(mode) == "" {
		mode = "plan"
	}
	return protocol.ApprovalRequest{
		PlanID:          plan.PlanID,
		CheckpointID:    "checkpoint_" + plan.PlanID,
		InterruptID:     "permission_" + request.RequestID,
		PendingNodeIDs:  []string{request.NodeID},
		Summary:         request.Question,
		RequiresInput:   true,
		CreatedAt:       time.Now().UTC(),
		Mode:            mode,
		ActiveRequestID: request.RequestID,
		Requests:        []protocol.PermissionRequest{request},
	}
}

func (r *Runtime) saveApproval(sessionID string, plan protocol.PlanResult, approval protocol.ApprovalRequest) (protocol.RunResult, error) {
	meta, err := r.store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	meta.State = protocol.SessionStateAwaitingApproval
	meta.ApprovalPending = true
	meta.ActivePlanID = plan.PlanID
	meta.ActiveCheckpointID = approval.CheckpointID
	meta.PendingInterruptID = approval.InterruptID
	meta.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	if err := r.store.SavePendingApproval(sessionID, approval); err != nil {
		return protocol.RunResult{}, err
	}
	if err := r.emit(sessionID, protocol.EventApprovalRequired, "permission required", approval); err != nil {
		return protocol.RunResult{}, err
	}
	snapshot, err := r.store.Snapshot(sessionID)
	return protocol.RunResult{Session: snapshot, Plan: snapshot.Plan, Approval: snapshot.Approval}, err
}

func (r *Runtime) continueAfterPermission(ctx context.Context, sessionID, turnID string, approval *protocol.ApprovalRequest, approvedRequest protocol.PermissionRequest) (protocol.RunResult, error) {
	if err := r.store.DeletePendingApproval(sessionID); err != nil {
		return protocol.RunResult{}, err
	}
	plan, err := r.store.LoadPlan(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if plan == nil {
		return protocol.RunResult{}, fmt.Errorf("no saved plan available")
	}
	if approval != nil && approval.Mode == "task" {
		return r.finishTaskAfterPermission(sessionID, turnID, *plan, approvedRequest)
	}
	for _, step := range plan.Steps {
		state, _ := r.store.LoadExecutionState(sessionID)
		if state != nil && nodeCompleted(state.Nodes, step.ID) {
			continue
		}
		if sideEffectTool(step.Tool) {
			rules, _ := r.store.LoadPermissionRules(sessionID)
			request, err := permissionRequestForStep(r.store, sessionID, plan.PlanID, step)
			if err != nil {
				return protocol.RunResult{}, err
			}
			if !permissionAllowedByRules(request, rules) {
				return r.saveApproval(sessionID, *plan, approvalFromRequest(*plan, request, "plan"))
			}
			if _, err := r.applyPermissionRequest(sessionID, request); err != nil {
				return protocol.RunResult{}, err
			}
			continue
		}
		output, err := r.executeStep(ctx, sessionID, plan.Goal, step)
		if err != nil {
			_ = r.markStep(sessionID, plan.PlanID, step.ID, protocol.NodeStatusFailed, err.Error(), output)
			return protocol.RunResult{}, err
		}
		_ = r.markStep(sessionID, plan.PlanID, step.ID, protocol.NodeStatusCompleted, "", output)
	}
	return r.finishCompleted(sessionID, turnID)
}

func (r *Runtime) finishTaskAfterPermission(sessionID, turnID string, plan protocol.PlanResult, request protocol.PermissionRequest) (protocol.RunResult, error) {
	meta, err := r.store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	state, err := r.store.LoadExecutionState(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if state != nil && executionFinalized(state.Nodes) {
		meta.State = protocol.SessionStateCompleted
	} else {
		meta.State = protocol.SessionStatePlanned
	}
	meta.ApprovalPending = false
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
	meta.ActivePlanID = plan.PlanID
	meta.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	snapshot, err := r.store.Snapshot(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	response := outputResponseForNode(snapshot.Execution.Outputs, request.NodeID)
	if strings.TrimSpace(response) == "" {
		response = fmt.Sprintf("Task %s completed.", request.NodeID)
	}
	result := protocol.RunResult{Session: snapshot, Plan: snapshot.Plan, Digests: snapshot.Digests, Comparison: snapshot.Compare, Artifacts: snapshot.Artifacts, Response: response}
	if err := r.emit(sessionID, protocol.EventResult, "task completed", map[string]any{
		"turn_id":  turnID,
		"task_id":  request.NodeID,
		"response": response,
	}); err != nil {
		return protocol.RunResult{}, err
	}
	return result, nil
}

func (r *Runtime) finishCompleted(sessionID, turnID string) (protocol.RunResult, error) {
	meta, err := r.store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	meta.State = protocol.SessionStateCompleted
	meta.ApprovalPending = false
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
	meta.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	snapshot, err := r.store.Snapshot(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	result := protocol.RunResult{Session: snapshot, Plan: snapshot.Plan, Digests: snapshot.Digests, Comparison: snapshot.Compare, Artifacts: snapshot.Artifacts, Response: summarizeResult(protocol.RunResult{Session: snapshot, Digests: snapshot.Digests, Comparison: snapshot.Compare})}
	if err := r.emit(sessionID, protocol.EventResult, "run completed", map[string]any{"turn_id": turnID, "response": result.Response}); err != nil {
		return protocol.RunResult{}, err
	}
	return result, nil
}

func (r *Runtime) applyPermissionRequest(sessionID string, request protocol.PermissionRequest) (protocol.NodeOutputRef, error) {
	switch request.Tool {
	case string(protocol.NodeKindWorkspaceEdit):
		result, err := applyApprovedEdit(r, sessionID, request)
		out := nodeOutput(request.NodeID, "workspace_response", result, time.Now().UTC())
		if err != nil {
			_ = r.markStep(sessionID, "", request.NodeID, protocol.NodeStatusFailed, err.Error(), out)
			return out, err
		}
		_ = r.markStep(sessionID, "", request.NodeID, protocol.NodeStatusCompleted, "", out)
		return out, nil
	case string(protocol.NodeKindWorkspaceCommand):
		record, err := r.registry.RunWorkspaceCommand(r.store, request.Command)
		text := formatCommandRecord(record)
		out := nodeOutput(request.NodeID, "workspace_response", text, time.Now().UTC())
		if err != nil {
			_ = r.markStep(sessionID, "", request.NodeID, protocol.NodeStatusFailed, err.Error(), out)
			return out, err
		}
		_ = r.markStep(sessionID, "", request.NodeID, protocol.NodeStatusCompleted, "", out)
		return out, nil
	default:
		return nodeOutput(request.NodeID, "permission", "approved", time.Now().UTC()), nil
	}
}

func applyApprovedEdit(r *Runtime, _ string, request protocol.PermissionRequest) (string, error) {
	if request.TargetPath == "" || request.Preview.Kind != "diff" {
		return "", fmt.Errorf("approved edit preview is missing")
	}
	if strings.TrimSpace(request.Preview.ConflictMessage) != "" {
		return "", fmt.Errorf("approved edit preview is invalid: %s", request.Preview.ConflictMessage)
	}
	current, existed, err := readWorkspaceFileForEdit(r, request.TargetPath)
	if err != nil {
		return "", err
	}
	if request.Preview.OldContentHash == "" {
		if existed {
			return "", fmt.Errorf("file was created since preview was generated: %s", request.TargetPath)
		}
	} else if contentHash(current) != request.Preview.OldContentHash {
		return "", fmt.Errorf("file changed since preview was generated: %s", request.TargetPath)
	}
	if err := r.registry.WriteWorkspaceFile(r.store, request.TargetPath, request.Preview.NewContent); err != nil {
		return "", err
	}
	output := firstNonEmpty(request.Preview.Summary, "Updated "+request.TargetPath)
	return output, nil
}

func (r *Runtime) rejectPermission(sessionID string, approval *protocol.ApprovalRequest, request protocol.PermissionRequest, decision protocol.PermissionDecision) (protocol.RunResult, error) {
	feedback := firstNonEmpty(decision.Feedback, "Tool use rejected")
	_ = r.markStep(sessionID, "", request.NodeID, protocol.NodeStatusFailed, feedback, nodeOutput(request.NodeID, "permission", feedback, time.Now().UTC()))
	if err := r.store.DeletePendingApproval(sessionID); err != nil {
		return protocol.RunResult{}, err
	}
	meta, err := r.store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if approval != nil && approval.Mode == "task" {
		state, err := r.store.LoadExecutionState(sessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		if state != nil && executionFinalized(state.Nodes) {
			meta.State = protocol.SessionStateCompleted
		} else {
			meta.State = protocol.SessionStatePlanned
		}
	} else {
		meta.State = protocol.SessionStateCompleted
	}
	meta.ApprovalPending = false
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
	meta.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	snapshot, err := r.store.Snapshot(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	_ = r.emit(sessionID, protocol.EventResult, "permission rejected", map[string]any{"request_id": request.RequestID, "feedback": feedback})
	return protocol.RunResult{Session: snapshot, Plan: snapshot.Plan, Response: feedback}, nil
}

func outputResponseForNode(outputs []protocol.NodeOutputRef, nodeID string) string {
	for i := len(outputs) - 1; i >= 0; i-- {
		output := outputs[i]
		if output.NodeID != nodeID {
			continue
		}
		if text, ok := output.Data["response"].(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
		for _, value := range output.Data {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" {
				return text
			}
		}
	}
	return ""
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

func permissionRequestForStep(store workspaceStore, sessionID, planID string, step protocol.PlanStep) (protocol.PermissionRequest, error) {
	switch step.Tool {
	case string(protocol.NodeKindWorkspaceCommand):
		return commandPermissionRequest(sessionID, planID, step.ID, extractBacktickCommand(step.Goal)), nil
	case string(protocol.NodeKindWorkspaceEdit):
		target := extractWorkspacePathMention(step.Goal)
		return editPermissionRequest(store, sessionID, planID, step.ID, step.Goal, target)
	default:
		return protocol.PermissionRequest{}, fmt.Errorf("task %s does not require permission", step.ID)
	}
}

type workspaceStore interface {
	ReadWorkspaceFile(string) (string, error)
	WorkspaceRoot() string
}

func commandPermissionRequest(sessionID, planID, nodeID, command string) protocol.PermissionRequest {
	return protocol.PermissionRequest{
		RequestID: "cmd_" + safeID(command),
		SessionID: sessionID,
		PlanID:    planID,
		NodeID:    nodeID,
		Tool:      string(protocol.NodeKindWorkspaceCommand),
		Operation: "shell",
		Title:     "Run command",
		Subtitle:  command,
		Question:  "Do you want to run this command?",
		Summary:   command,
		Command:   command,
		Preview: protocol.PermissionPreview{
			Kind:          "command",
			Summary:       "cwd: current workspace",
			CommandPrefix: agenttool.ShellCommandPrefix(command),
		},
		Options:   agenttool.PermissionOptions(protocol.NodeKindWorkspaceCommand),
		CreatedAt: time.Now().UTC(),
	}
}

func editPermissionRequest(store workspaceStore, sessionID, planID, nodeID, goal, targetPath string) (protocol.PermissionRequest, error) {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return protocol.PermissionRequest{}, fmt.Errorf("workspace edit requires an explicit target file path")
	}
	old, existed, readErr := readWorkspaceFileForEditStore(store, targetPath)
	oldHash := ""
	if existed {
		oldHash = contentHash(old)
	}
	newContent := ""
	contentErr := readErr
	if contentErr == nil {
		newContent, contentErr = inferEditContent(goal, targetPath, old, existed)
	}
	if contentErr != nil {
		return protocol.PermissionRequest{}, contentErr
	}
	summary := "Update " + targetPath
	if !existed {
		summary = "Create " + targetPath
	}
	return protocol.PermissionRequest{
		RequestID:  "edit_" + safeID(targetPath),
		SessionID:  sessionID,
		PlanID:     planID,
		NodeID:     nodeID,
		Tool:       string(protocol.NodeKindWorkspaceEdit),
		Operation:  "write",
		Title:      "Edit file",
		Subtitle:   targetPath,
		Question:   fmt.Sprintf("Do you want to make this edit to %s?", filepath.Base(targetPath)),
		Summary:    summary,
		TargetPath: targetPath,
		Preview: protocol.PermissionPreview{
			Kind:            "diff",
			Summary:         summary,
			Diff:            agenttool.CompactUnifiedDiff(targetPath, old, newContent),
			OldContentHash:  oldHash,
			NewContent:      newContent,
			ConflictMessage: errorString(contentErr),
		},
		Options:   agenttool.PermissionOptions(protocol.NodeKindWorkspaceEdit),
		CreatedAt: time.Now().UTC(),
	}, nil
}

func inferWorkspaceIntent(goal string, files []protocol.WorkspaceFile) workspaceIntent {
	lower := strings.ToLower(strings.TrimSpace(goal))
	intent := workspaceIntent{kind: protocol.NodeKindWorkspaceInspect}
	if command := extractBacktickCommand(goal); command != "" && containsAny(lower, "run", "execute", "shell", "command", "运行", "执行", "命令") {
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
	default:
		intent.kind = protocol.NodeKindWorkspaceInspect
	}
	intent.targetPath = findWorkspaceTargetPath(goal, files, intent.kind != protocol.NodeKindWorkspaceEdit)
	return intent
}

func findWorkspaceTargetPath(goal string, files []protocol.WorkspaceFile, allowDefault bool) string {
	if explicit := extractWorkspacePathMention(goal); explicit != "" {
		if indexed := matchIndexedWorkspacePath(explicit, files); indexed != "" {
			return indexed
		}
		return explicit
	}
	lower := strings.ToLower(goal)
	best := ""
	score := 0
	for _, file := range files {
		base := strings.ToLower(filepath.Base(file.Path))
		next := 0
		if strings.Contains(lower, strings.ToLower(file.Path)) {
			next += 10
		}
		if base != "" && strings.Contains(lower, base) {
			next += 8
		}
		if next > score {
			score = next
			best = file.Path
		}
	}
	if best == "" && allowDefault {
		for _, file := range files {
			if strings.EqualFold(filepath.Base(file.Path), "README.md") {
				return file.Path
			}
		}
	}
	return best
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
	if value == "" || strings.Contains(value, "://") || strings.ContainsAny(value, "\n\r*?") {
		return false
	}
	switch strings.ToLower(filepath.Ext(value)) {
	case ".md", ".txt", ".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".json", ".yaml", ".yml", ".toml", ".tex", ".csv":
		return true
	default:
		return false
	}
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
		if workspacePathMatchKey(filepath.Base(file.Path)) != key {
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
	return strings.ToLower(strings.TrimPrefix(value, "./"))
}

func extractBacktickCommand(goal string) string {
	for _, segment := range backtickSegments(goal) {
		if segment != "" {
			return segment
		}
	}
	return ""
}

func backtickSegments(value string) []string {
	parts := strings.Split(value, "`")
	if len(parts) < 3 {
		return nil
	}
	out := make([]string, 0, len(parts)/2)
	for i := 1; i < len(parts); i += 2 {
		if segment := strings.TrimSpace(parts[i]); segment != "" {
			out = append(out, segment)
		}
	}
	return out
}

func extractSearchQuery(goal string) string {
	trimmed := strings.TrimSpace(goal)
	for _, quote := range []string{`"`, `'`, "`"} {
		parts := strings.Split(trimmed, quote)
		if len(parts) >= 3 && strings.TrimSpace(parts[1]) != "" {
			return strings.TrimSpace(parts[1])
		}
	}
	return trimmed
}

func extractPromptPaperSources(goal string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	urls := make([]string, 0)
	for _, match := range urlPattern.FindAllString(goal, -1) {
		match = strings.TrimRight(strings.TrimSpace(match), ".,);]")
		if match == "" {
			continue
		}
		seen[match] = struct{}{}
		urls = append(urls, match)
		out = append(out, match)
	}
	for _, match := range arxivIDPattern.FindAllString(goal, -1) {
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

func goalNeedsPaperContext(goal string) bool {
	lower := strings.ToLower(goal)
	return containsAny(lower, "paper", "papers", "arxiv", "论文", "文献", "摘要", "实验", "公式")
}

func choosePaperCandidates(goal string, files []protocol.WorkspaceFile) []string {
	type scored struct {
		raw   string
		score int
	}
	var scoredFiles []scored
	lowerGoal := strings.ToLower(goal)
	for _, file := range files {
		if !strings.HasSuffix(strings.ToLower(file.Path), ".pdf") {
			continue
		}
		score := 5
		base := strings.ToLower(file.Path)
		for _, token := range strings.Fields(lowerGoal) {
			if len(token) >= 3 && strings.Contains(base, token) {
				score += 3
			}
		}
		scoredFiles = append(scoredFiles, scored{raw: file.AbsolutePath, score: score})
	}
	sort.SliceStable(scoredFiles, func(i, j int) bool {
		if scoredFiles[i].score == scoredFiles[j].score {
			return scoredFiles[i].raw < scoredFiles[j].raw
		}
		return scoredFiles[i].score > scoredFiles[j].score
	})
	if len(scoredFiles) == 0 {
		return nil
	}
	limit := 1
	if strings.Contains(lowerGoal, "compare") || strings.Contains(lowerGoal, "比较") || strings.Contains(lowerGoal, "对比") {
		limit = min(4, len(scoredFiles))
	}
	out := make([]string, 0, limit)
	for _, file := range scoredFiles[:limit] {
		out = append(out, file.raw)
	}
	return out
}

func sideEffectKind(kind protocol.NodeKind) bool {
	return kind == protocol.NodeKindWorkspaceEdit || kind == protocol.NodeKindWorkspaceCommand
}

func sideEffectTool(toolName string) bool {
	return toolName == string(protocol.NodeKindWorkspaceEdit) || toolName == string(protocol.NodeKindWorkspaceCommand)
}

func preferWorkspacePlan(goal string, intent workspaceIntent) bool {
	lower := strings.ToLower(strings.TrimSpace(goal))
	if intent.kind == protocol.NodeKindWorkspaceCommand || intent.kind == protocol.NodeKindWorkspaceEdit {
		return true
	}
	if containsAny(lower, "workspace", "current directory", "current repo", "repository", "repo", "readme", "工作区", "当前目录", "仓库", "代码", "文件") {
		return true
	}
	if extractWorkspacePathMention(goal) != "" {
		return true
	}
	if intent.kind == protocol.NodeKindWorkspaceSearch && !goalNeedsPaperContext(goal) {
		return true
	}
	return false
}

func permissionAllowedByRules(request protocol.PermissionRequest, rules []protocol.PermissionRule) bool {
	for _, rule := range rules {
		if agenttool.PermissionAllowedByRule(request, rule) {
			return true
		}
	}
	return false
}

func hasPendingApproval(store interface {
	LoadPendingApproval(string) (*protocol.ApprovalRequest, error)
}, sessionID string, meta protocol.SessionMeta) bool {
	if meta.State == protocol.SessionStateAwaitingApproval || meta.ApprovalPending {
		return true
	}
	approval, err := store.LoadPendingApproval(sessionID)
	return err == nil && approval != nil
}

func stepDependenciesSatisfied(plan protocol.PlanResult, state *protocol.ExecutionState, nodeID string) (bool, []string) {
	node, ok := planNodeByID(plan.DAG, nodeID)
	if !ok {
		return false, []string{nodeID}
	}
	if len(node.DependsOn) == 0 {
		return true, nil
	}
	execByNodeID := map[string]protocol.NodeExecutionState{}
	stale := map[string]struct{}{}
	if state != nil {
		for _, exec := range state.Nodes {
			execByNodeID[exec.NodeID] = exec
		}
		for _, staleID := range state.StaleNodeIDs {
			stale[staleID] = struct{}{}
		}
	}
	blockedBy := make([]string, 0, len(node.DependsOn))
	for _, depID := range node.DependsOn {
		depNode, ok := planNodeByID(plan.DAG, depID)
		if !ok {
			blockedBy = append(blockedBy, depID)
			continue
		}
		depStatus := depNode.Status
		if exec, ok := execByNodeID[depID]; ok && exec.Status != "" {
			depStatus = exec.Status
		}
		if _, ok := stale[depID]; ok {
			blockedBy = append(blockedBy, depID)
			continue
		}
		switch depStatus {
		case protocol.NodeStatusCompleted, protocol.NodeStatusSkipped:
			continue
		case protocol.NodeStatusFailed:
			if !depNode.Required {
				continue
			}
		}
		blockedBy = append(blockedBy, depID)
	}
	return len(blockedBy) == 0, blockedBy
}

func planNodeByID(dag protocol.PlanDAG, nodeID string) (protocol.PlanNode, bool) {
	for _, node := range dag.Nodes {
		if node.ID == nodeID {
			return node, true
		}
	}
	return protocol.PlanNode{}, false
}

func nodeCompleted(nodes []protocol.NodeExecutionState, nodeID string) bool {
	for _, node := range nodes {
		if node.NodeID == nodeID {
			return node.Status == protocol.NodeStatusCompleted || node.Status == protocol.NodeStatusFailed
		}
	}
	return false
}

func executionFinalized(nodes []protocol.NodeExecutionState) bool {
	if len(nodes) == 0 {
		return false
	}
	for _, node := range nodes {
		if node.Status != protocol.NodeStatusCompleted && node.Status != protocol.NodeStatusFailed && node.Status != protocol.NodeStatusSkipped {
			return false
		}
	}
	return true
}

func taskStatusForNode(status protocol.NodeStatus) protocol.TaskStatus {
	switch status {
	case protocol.NodeStatusCompleted:
		return protocol.TaskStatusCompleted
	case protocol.NodeStatusFailed:
		return protocol.TaskStatusFailed
	case protocol.NodeStatusRunning:
		return protocol.TaskStatusRunning
	default:
		return protocol.TaskStatusReady
	}
}

func summarizeWorkspace(store interface{ WorkspaceRoot() string }, files []protocol.WorkspaceFile) string {
	textCount := 0
	paperCount := 0
	preview := make([]string, 0, min(8, len(files)))
	for _, file := range files {
		if file.Kind == protocol.WorkspaceFileKindText || file.Kind == protocol.WorkspaceFileKindCode {
			textCount++
		}
		if file.PaperCandidate {
			paperCount++
		}
		if len(preview) < 8 {
			preview = append(preview, file.Path)
		}
	}
	return fmt.Sprintf("Workspace %s has %d indexed files (%d text/code, %d paper candidates). Top files: %s",
		store.WorkspaceRoot(), len(files), textCount, paperCount, strings.Join(preview, ", "))
}

func formatSearchHits(query string, hits []protocol.WorkspaceSearchHit) string {
	if len(hits) == 0 {
		return fmt.Sprintf("No matches for %q in the current workspace.", query)
	}
	lines := []string{fmt.Sprintf("Search results for %q:", query)}
	for _, hit := range hits {
		lines = append(lines, fmt.Sprintf("- %s:%d %s", hit.Path, hit.Line, hit.Snippet))
	}
	return strings.Join(lines, "\n")
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

func inferEditContent(goal, targetPath, oldContent string, existed bool) (string, error) {
	if oldValue, newValue, ok := inferEditReplacement(goal); ok {
		if !existed {
			return "", fmt.Errorf("cannot replace text in missing file %s", targetPath)
		}
		if !strings.Contains(oldContent, oldValue) {
			return "", fmt.Errorf("replacement text not found in %s", targetPath)
		}
		return strings.Replace(oldContent, oldValue, newValue, 1), nil
	}
	if content, ok := inferExplicitEditContent(goal); ok {
		return content, nil
	}
	if !existed {
		return "", fmt.Errorf("new file content is required for %s", targetPath)
	}
	return "", fmt.Errorf("workspace edit requires explicit content or a replacement for %s", targetPath)
}

func inferEditReplacement(goal string) (string, string, bool) {
	lower := strings.ToLower(goal)
	if !containsAny(lower, "replace", "替换", "改成", "更改") {
		return "", "", false
	}
	segments := backtickSegments(goal)
	values := make([]string, 0, len(segments))
	for _, segment := range segments {
		if !pathLooksLikeWorkspaceFile(segment) {
			values = append(values, segment)
		}
	}
	if len(values) < 2 {
		return "", "", false
	}
	return values[0], values[1], true
}

func inferExplicitEditContent(goal string) (string, bool) {
	lower := strings.ToLower(goal)
	for _, marker := range []string{"with ", "内容为", "写入"} {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		value := strings.TrimSpace(goal[idx+len(marker):])
		if value == "" {
			continue
		}
		return strings.Trim(value, "\"'`") + "\n", true
	}
	return "", false
}

func readWorkspaceFileForEdit(r *Runtime, path string) (string, bool, error) {
	return readWorkspaceFileForEditStore(r.store, path)
}

func readWorkspaceFileForEditStore(store interface{ ReadWorkspaceFile(string) (string, error) }, targetPath string) (string, bool, error) {
	content, err := store.ReadWorkspaceFile(targetPath)
	if err == nil {
		return content, true, nil
	}
	if os.IsNotExist(err) {
		return "", false, nil
	}
	return "", false, err
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func containsAny(input string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(input, token) {
			return true
		}
	}
	return false
}

func safeID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "item"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "_") {
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "item"
	}
	if len(out) > 40 {
		return out[:40]
	}
	return out
}

func newPlanID() string {
	return fmt.Sprintf("plan_%d", time.Now().UnixNano())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
