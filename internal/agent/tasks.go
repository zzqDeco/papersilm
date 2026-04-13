package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func (a *Agent) RunTask(ctx context.Context, store *storage.Store, sink EventSink, sessionID, taskID, lang, style string) (protocol.RunResult, error) {
	meta, err := store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	lang, style, err = validatePlannedExecutionConfig(meta, lang, style)
	if err != nil {
		return protocol.RunResult{}, err
	}

	snapshot, err := store.Snapshot(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if snapshot.Plan == nil || snapshot.Execution == nil {
		return protocol.RunResult{}, fmt.Errorf("no saved plan available")
	}
	task, ok := findTaskCard(snapshot.TaskBoard, taskID)
	if !ok {
		return protocol.RunResult{}, fmt.Errorf("unknown task: %s", taskID)
	}
	if meta.State == protocol.SessionStateAwaitingApproval || len(snapshot.Execution.PendingNodeIDs) > 0 {
		if containsString(snapshot.Execution.PendingNodeIDs, taskID) {
			return protocol.RunResult{}, fmt.Errorf("task %s is awaiting approval; use /task approve %s or /task reject %s", taskID, taskID, taskID)
		}
		return protocol.RunResult{}, fmt.Errorf("session is awaiting approval; only the current pending batch can be approved or rejected")
	}
	switch task.Status {
	case protocol.TaskStatusRunning:
		return protocol.RunResult{}, fmt.Errorf("task is already running: %s", taskID)
	case protocol.TaskStatusAwaitingApproval:
		return protocol.RunResult{}, fmt.Errorf("task %s is awaiting approval; use /task approve %s or /task reject %s", taskID, taskID, taskID)
	case protocol.TaskStatusCompleted:
		if err := cascadeRerun(store, sessionID, snapshot.Plan, snapshot.Execution, taskID); err != nil {
			return protocol.RunResult{}, err
		}
	case protocol.TaskStatusFailed:
		resetExecutionNode(snapshot.Execution, taskID)
		setNodeStatus(&snapshot.Plan.DAG, taskID, protocol.NodeStatusPending)
	}

	scope := dependencyClosure(snapshot.Plan.DAG, snapshot.Execution, taskID)
	meta.State = protocol.SessionStateRunning
	meta.ApprovalPending = false
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
	meta.UpdatedAt = time.Now().UTC()
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	return a.runScopedExecution(ctx, store, sink, sessionID, meta, *snapshot.Plan, snapshot.Execution, meta.LastTask, lang, style, scope, taskID)
}

func (a *Agent) ApproveTask(ctx context.Context, store *storage.Store, sink EventSink, sessionID, taskID string, approved bool, comment string) (protocol.RunResult, error) {
	meta, err := store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	planResult, err := store.LoadPlan(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if planResult == nil {
		return protocol.RunResult{}, fmt.Errorf("no saved plan available")
	}
	execState, err := store.LoadExecutionState(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if execState == nil {
		return protocol.RunResult{}, fmt.Errorf("session has no pending execution state")
	}
	if !approved {
		return a.rejectTask(store, sessionID, meta, planResult, execState, taskID, comment)
	}
	if meta.State != protocol.SessionStateAwaitingApproval || len(execState.PendingNodeIDs) == 0 {
		return protocol.RunResult{}, fmt.Errorf("session has no pending approval tasks")
	}
	if !containsString(execState.PendingNodeIDs, taskID) {
		return protocol.RunResult{}, fmt.Errorf("task %s is not awaiting approval", taskID)
	}

	batchID := fmt.Sprintf("task_batch_%d", time.Now().UnixNano())
	execState.CurrentBatchID = batchID
	execState.BatchHistory = append(execState.BatchHistory, protocol.ExecutionBatch{
		BatchID:   batchID,
		NodeIDs:   []string{taskID},
		Status:    protocol.BatchStatusRunning,
		StartedAt: time.Now().UTC(),
	})
	setNodeStatus(&planResult.DAG, taskID, protocol.NodeStatusRunning)
	node, _ := dagNode(planResult.DAG, taskID)
	updateExecutionNode(execState, taskID, protocol.NodeStatusRunning, "", nil)
	if err := a.emit(store, sink, sessionID, protocol.EventProgress, "node execution started", protocol.PlanProgress{
		PlanID:        planResult.PlanID,
		NodeID:        taskID,
		Tool:          string(node.Kind),
		WorkerProfile: node.WorkerProfile,
		BatchID:       batchID,
		Status:        protocol.PlanProgressStarted,
		Message:       "node execution started",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		return protocol.RunResult{}, err
	}
	if err := store.SavePlan(sessionID, *planResult); err != nil {
		return protocol.RunResult{}, err
	}
	if err := store.SaveExecutionState(sessionID, *execState); err != nil {
		return protocol.RunResult{}, err
	}

	results := a.executeBatch(ctx, store, sessionID, meta.LastTask, meta.Language, meta.Style, execState, planResult.DAG, []string{taskID})
	for _, result := range results {
		node, _ := dagNode(planResult.DAG, result.nodeID)
		if result.err != nil {
			setNodeStatus(&planResult.DAG, result.nodeID, protocol.NodeStatusFailed)
			updateExecutionNode(execState, result.nodeID, protocol.NodeStatusFailed, result.err.Error(), nil)
			if err := a.emit(store, sink, sessionID, protocol.EventProgress, "node execution failed", protocol.PlanProgress{
				PlanID:        planResult.PlanID,
				NodeID:        result.nodeID,
				Tool:          string(node.Kind),
				WorkerProfile: node.WorkerProfile,
				BatchID:       batchID,
				Status:        protocol.PlanProgressFailed,
				Message:       "node execution failed",
				Error:         result.err.Error(),
				CreatedAt:     time.Now().UTC(),
			}); err != nil {
				return protocol.RunResult{}, err
			}
			continue
		}
		setNodeStatus(&planResult.DAG, result.nodeID, protocol.NodeStatusCompleted)
		updateExecutionNode(execState, result.nodeID, protocol.NodeStatusCompleted, "", result.outputs)
		execState.Outputs = append(execState.Outputs, result.outputs...)
		if err := a.emit(store, sink, sessionID, protocol.EventProgress, "node execution completed", protocol.PlanProgress{
			PlanID:        planResult.PlanID,
			NodeID:        result.nodeID,
			Tool:          string(node.Kind),
			WorkerProfile: node.WorkerProfile,
			BatchID:       batchID,
			Status:        protocol.PlanProgressCompleted,
			Message:       "node execution completed",
			CreatedAt:     time.Now().UTC(),
		}); err != nil {
			return protocol.RunResult{}, err
		}
	}
	execState.PendingNodeIDs = removeString(execState.PendingNodeIDs, taskID)
	markBatchCompleted(execState, batchID)
	refreshReadyNodes(&planResult.DAG)
	if err := store.SavePlan(sessionID, *planResult); err != nil {
		return protocol.RunResult{}, err
	}
	if err := store.SaveExecutionState(sessionID, *execState); err != nil {
		return protocol.RunResult{}, err
	}
	return a.finishTaskApproval(store, sessionID, meta, planResult, execState)
}

func (a *Agent) RejectTask(ctx context.Context, store *storage.Store, sink EventSink, sessionID, taskID, comment string) (protocol.RunResult, error) {
	return a.ApproveTask(ctx, store, sink, sessionID, taskID, false, comment)
}

func (a *Agent) runScopedExecution(
	ctx context.Context,
	store *storage.Store,
	sink EventSink,
	sessionID string,
	meta protocol.SessionMeta,
	planResult protocol.PlanResult,
	execState *protocol.ExecutionState,
	goal, lang, style string,
	allowed map[string]struct{},
	targetTaskID string,
) (protocol.RunResult, error) {
	for {
		refreshReadyNodes(&planResult.DAG)
		if nodeSettledForStop(planResult.DAG, execState, targetTaskID) {
			break
		}

		batch := selectBatchFromSet(planResult.DAG, allowed)
		if len(batch) == 0 {
			return protocol.RunResult{}, fmt.Errorf("task %s has no runnable dependency closure", targetTaskID)
		}

		batchID := fmt.Sprintf("task_batch_%d", time.Now().UnixNano())
		execState.CurrentBatchID = batchID
		execState.PendingNodeIDs = nil
		execState.BatchHistory = append(execState.BatchHistory, protocol.ExecutionBatch{
			BatchID:   batchID,
			NodeIDs:   append([]string(nil), batch...),
			Status:    protocol.BatchStatusRunning,
			StartedAt: time.Now().UTC(),
		})
		for _, nodeID := range batch {
			setNodeStatus(&planResult.DAG, nodeID, protocol.NodeStatusRunning)
			node, _ := dagNode(planResult.DAG, nodeID)
			updateExecutionNode(execState, nodeID, protocol.NodeStatusRunning, "", nil)
			if err := a.emit(store, sink, sessionID, protocol.EventProgress, "node execution started", protocol.PlanProgress{
				PlanID:        planResult.PlanID,
				NodeID:        nodeID,
				Tool:          string(node.Kind),
				WorkerProfile: node.WorkerProfile,
				BatchID:       batchID,
				Status:        protocol.PlanProgressStarted,
				Message:       "node execution started",
				CreatedAt:     time.Now().UTC(),
			}); err != nil {
				return protocol.RunResult{}, err
			}
		}
		if err := store.SavePlan(sessionID, planResult); err != nil {
			return protocol.RunResult{}, err
		}
		if err := store.SaveExecutionState(sessionID, *execState); err != nil {
			return protocol.RunResult{}, err
		}

		results := a.executeBatch(ctx, store, sessionID, goal, lang, style, execState, planResult.DAG, batch)
		for _, result := range results {
			node, _ := dagNode(planResult.DAG, result.nodeID)
			if result.err != nil {
				setNodeStatus(&planResult.DAG, result.nodeID, protocol.NodeStatusFailed)
				updateExecutionNode(execState, result.nodeID, protocol.NodeStatusFailed, result.err.Error(), nil)
				if err := a.emit(store, sink, sessionID, protocol.EventProgress, "node execution failed", protocol.PlanProgress{
					PlanID:        planResult.PlanID,
					NodeID:        result.nodeID,
					Tool:          string(node.Kind),
					WorkerProfile: node.WorkerProfile,
					BatchID:       batchID,
					Status:        protocol.PlanProgressFailed,
					Message:       "node execution failed",
					Error:         result.err.Error(),
					CreatedAt:     time.Now().UTC(),
				}); err != nil {
					return protocol.RunResult{}, err
				}
				continue
			}
			setNodeStatus(&planResult.DAG, result.nodeID, protocol.NodeStatusCompleted)
			updateExecutionNode(execState, result.nodeID, protocol.NodeStatusCompleted, "", result.outputs)
			execState.Outputs = append(execState.Outputs, result.outputs...)
			if err := a.emit(store, sink, sessionID, protocol.EventProgress, "node execution completed", protocol.PlanProgress{
				PlanID:        planResult.PlanID,
				NodeID:        result.nodeID,
				Tool:          string(node.Kind),
				WorkerProfile: node.WorkerProfile,
				BatchID:       batchID,
				Status:        protocol.PlanProgressCompleted,
				Message:       "node execution completed",
				CreatedAt:     time.Now().UTC(),
			}); err != nil {
				return protocol.RunResult{}, err
			}
		}

		markBatchCompleted(execState, batchID)
		refreshReadyNodes(&planResult.DAG)
		if err := store.SavePlan(sessionID, planResult); err != nil {
			return protocol.RunResult{}, err
		}
		if err := store.SaveExecutionState(sessionID, *execState); err != nil {
			return protocol.RunResult{}, err
		}
	}

	meta.State = protocol.SessionStatePlanned
	meta.ApprovalPending = false
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
	if planFullySettled(planResult.DAG) {
		meta.State = protocol.SessionStateCompleted
	}
	meta.UpdatedAt = time.Now().UTC()
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	if err := store.SavePlan(sessionID, planResult); err != nil {
		return protocol.RunResult{}, err
	}
	if err := store.SaveExecutionState(sessionID, *execState); err != nil {
		return protocol.RunResult{}, err
	}

	snapshot, err := store.Snapshot(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if err := a.emit(store, sink, sessionID, protocol.EventResult, "task run completed", map[string]any{
		"task_id":    targetTaskID,
		"digests":    len(snapshot.Digests),
		"comparison": snapshot.Compare != nil,
		"artifacts":  len(snapshot.Artifacts),
	}); err != nil {
		return protocol.RunResult{}, err
	}
	return protocol.RunResult{
		Session:    snapshot,
		Plan:       snapshot.Plan,
		Digests:    snapshot.Digests,
		Comparison: snapshot.Compare,
		Artifacts:  snapshot.Artifacts,
	}, nil
}

func (a *Agent) finishTaskApproval(store *storage.Store, sessionID string, meta protocol.SessionMeta, planResult *protocol.PlanResult, execState *protocol.ExecutionState) (protocol.RunResult, error) {
	if len(execState.PendingNodeIDs) > 0 {
		meta.State = protocol.SessionStateAwaitingApproval
		meta.ApprovalPending = true
		meta.UpdatedAt = time.Now().UTC()
		if err := store.SaveMeta(meta); err != nil {
			return protocol.RunResult{}, err
		}
		if err := store.SavePlan(sessionID, *planResult); err != nil {
			return protocol.RunResult{}, err
		}
		if err := store.SaveExecutionState(sessionID, *execState); err != nil {
			return protocol.RunResult{}, err
		}
		snapshot, err := store.Snapshot(sessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return protocol.RunResult{
			Session: snapshot,
			Plan:    snapshot.Plan,
			Approval: &protocol.ApprovalRequest{
				PlanID:         planResult.PlanID,
				CheckpointID:   meta.ActiveCheckpointID,
				InterruptID:    meta.PendingInterruptID,
				PendingNodeIDs: append([]string(nil), execState.PendingNodeIDs...),
				Summary:        approvalSummary(*planResult, execState.PendingNodeIDs),
				RequiresInput:  true,
				CreatedAt:      time.Now().UTC(),
			},
		}, nil
	}

	refreshReadyNodes(&planResult.DAG)
	if planFullySettled(planResult.DAG) {
		meta.State = protocol.SessionStateCompleted
		meta.ApprovalPending = false
		meta.ActiveCheckpointID = ""
		meta.PendingInterruptID = ""
		meta.UpdatedAt = time.Now().UTC()
		if err := store.SaveMeta(meta); err != nil {
			return protocol.RunResult{}, err
		}
		if err := store.SavePlan(sessionID, *planResult); err != nil {
			return protocol.RunResult{}, err
		}
		if err := store.SaveExecutionState(sessionID, *execState); err != nil {
			return protocol.RunResult{}, err
		}
		snapshot, err := store.Snapshot(sessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return protocol.RunResult{
			Session:    snapshot,
			Plan:       snapshot.Plan,
			Digests:    snapshot.Digests,
			Comparison: snapshot.Compare,
			Artifacts:  snapshot.Artifacts,
		}, nil
	}

	nextBatch := selectBatch(planResult.DAG)
	if len(nextBatch) == 0 {
		meta.State = protocol.SessionStatePlanned
		meta.ApprovalPending = false
		meta.ActiveCheckpointID = ""
		meta.PendingInterruptID = ""
		meta.UpdatedAt = time.Now().UTC()
		if err := store.SaveMeta(meta); err != nil {
			return protocol.RunResult{}, err
		}
		if err := store.SavePlan(sessionID, *planResult); err != nil {
			return protocol.RunResult{}, err
		}
		if err := store.SaveExecutionState(sessionID, *execState); err != nil {
			return protocol.RunResult{}, err
		}
		snapshot, err := store.Snapshot(sessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return protocol.RunResult{Session: snapshot, Plan: snapshot.Plan}, nil
	}

	checkpointID := fmt.Sprintf("%s_confirm_%d", sessionID, time.Now().UnixNano())
	interruptID := fmt.Sprintf("approval_%d", time.Now().UnixNano())
	execState.PendingNodeIDs = append([]string(nil), nextBatch...)
	execState.CurrentBatchID = fmt.Sprintf("batch_%d", time.Now().UnixNano())
	execState.UpdatedAt = time.Now().UTC()
	meta.State = protocol.SessionStateAwaitingApproval
	meta.ApprovalPending = true
	meta.ActiveCheckpointID = checkpointID
	meta.PendingInterruptID = interruptID
	meta.UpdatedAt = time.Now().UTC()
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	if err := store.SavePlan(sessionID, *planResult); err != nil {
		return protocol.RunResult{}, err
	}
	if err := store.SaveExecutionState(sessionID, *execState); err != nil {
		return protocol.RunResult{}, err
	}
	snapshot, err := store.Snapshot(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	return protocol.RunResult{
		Session: snapshot,
		Plan:    snapshot.Plan,
		Approval: &protocol.ApprovalRequest{
			PlanID:         planResult.PlanID,
			CheckpointID:   checkpointID,
			InterruptID:    interruptID,
			PendingNodeIDs: append([]string(nil), nextBatch...),
			Summary:        approvalSummary(*planResult, nextBatch),
			RequiresInput:  true,
			CreatedAt:      time.Now().UTC(),
		},
	}, nil
}

func (a *Agent) rejectTask(store *storage.Store, sessionID string, meta protocol.SessionMeta, planResult *protocol.PlanResult, execState *protocol.ExecutionState, taskID, comment string) (protocol.RunResult, error) {
	if meta.State != protocol.SessionStateAwaitingApproval || len(execState.PendingNodeIDs) == 0 {
		return protocol.RunResult{}, fmt.Errorf("session has no pending approval tasks")
	}
	if !containsString(execState.PendingNodeIDs, taskID) {
		return protocol.RunResult{}, fmt.Errorf("task %s is not awaiting approval", taskID)
	}
	node, ok := dagNode(planResult.DAG, taskID)
	if !ok {
		return protocol.RunResult{}, fmt.Errorf("unknown task: %s", taskID)
	}

	reason := rejectionReason(comment)
	status := protocol.NodeStatusFailed
	if !node.Required {
		status = protocol.NodeStatusSkipped
	}
	setNodeStatus(&planResult.DAG, taskID, status)
	updateExecutionNode(execState, taskID, status, reason, nil)
	execState.PendingNodeIDs = removeString(execState.PendingNodeIDs, taskID)
	refreshReadyNodes(&planResult.DAG)
	return a.finishTaskApproval(store, sessionID, meta, planResult, execState)
}

func dependencyClosure(dag protocol.PlanDAG, execState *protocol.ExecutionState, targetID string) map[string]struct{} {
	execByNodeID := make(map[string]protocol.NodeExecutionState, len(execState.Nodes))
	for _, node := range execState.Nodes {
		execByNodeID[node.NodeID] = node
	}
	out := map[string]struct{}{}
	var visit func(string)
	visit = func(nodeID string) {
		node, ok := dagNode(dag, nodeID)
		if !ok {
			return
		}
		for _, depID := range node.DependsOn {
			depNode, ok := dagNode(dag, depID)
			if !ok {
				continue
			}
			status := depNode.Status
			if exec, ok := execByNodeID[depID]; ok && exec.Status != "" {
				status = exec.Status
			}
			if !nodeSettledForDependency(depNode, status) {
				visit(depID)
				out[depID] = struct{}{}
			}
		}
		out[nodeID] = struct{}{}
	}
	visit(targetID)
	return out
}

func cascadeRerun(store *storage.Store, sessionID string, plan *protocol.PlanResult, execState *protocol.ExecutionState, targetID string) error {
	affected := descendantClosure(plan.DAG, targetID)
	affected[targetID] = struct{}{}
	affectedIDs := sortedKeys(affected)
	for _, nodeID := range affectedIDs {
		setNodeStatus(&plan.DAG, nodeID, protocol.NodeStatusPending)
		resetExecutionNode(execState, nodeID)
		addStaleNodeID(execState, nodeID)
	}
	execState.PendingNodeIDs = filterOutStrings(execState.PendingNodeIDs, affectedIDs)
	execState.Outputs = filterOutputs(execState.Outputs, affected)
	if err := deleteArtifactsForNodes(store, sessionID, plan.DAG, affected); err != nil {
		return err
	}
	execState.UpdatedAt = time.Now().UTC()
	return nil
}

func deleteArtifactsForNodes(store *storage.Store, sessionID string, dag protocol.PlanDAG, affected map[string]struct{}) error {
	for nodeID := range affected {
		node, ok := dagNode(dag, nodeID)
		if !ok {
			continue
		}
		switch node.Kind {
		case protocol.NodeKindMergeDigest, protocol.NodeKind("distill_paper"):
			for _, paperID := range node.PaperIDs {
				if err := store.DeletePaperDigestArtifacts(sessionID, paperID); err != nil {
					return err
				}
			}
		case protocol.NodeKindFinalSynthesis, protocol.NodeKind("compare_papers"):
			if err := store.DeleteComparisonArtifacts(sessionID); err != nil {
				return err
			}
		}
	}
	return nil
}

func descendantClosure(dag protocol.PlanDAG, nodeID string) map[string]struct{} {
	out := map[string]struct{}{}
	queue := []string{nodeID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range outgoingNodeIDs(dag, current) {
			if _, ok := out[next]; ok {
				continue
			}
			out[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	return out
}

func nodeSettledForDependency(node protocol.PlanNode, status protocol.NodeStatus) bool {
	switch status {
	case protocol.NodeStatusCompleted, protocol.NodeStatusSkipped:
		return true
	case protocol.NodeStatusFailed:
		return !node.Required
	default:
		return false
	}
}

func nodeSettledForStop(dag protocol.PlanDAG, execState *protocol.ExecutionState, nodeID string) bool {
	node, ok := dagNode(dag, nodeID)
	if !ok {
		return false
	}
	status := node.Status
	for _, execNode := range execState.Nodes {
		if execNode.NodeID == nodeID && execNode.Status != "" {
			status = execNode.Status
			break
		}
	}
	switch status {
	case protocol.NodeStatusCompleted, protocol.NodeStatusFailed, protocol.NodeStatusSkipped:
		return true
	default:
		return false
	}
}

func findTaskCard(board *protocol.TaskBoard, taskID string) (protocol.TaskCard, bool) {
	if board == nil {
		return protocol.TaskCard{}, false
	}
	for _, task := range board.Tasks {
		if task.TaskID == taskID {
			return task, true
		}
	}
	return protocol.TaskCard{}, false
}

func filterOutputs(outputs []protocol.NodeOutputRef, remove map[string]struct{}) []protocol.NodeOutputRef {
	filtered := outputs[:0]
	for _, output := range outputs {
		if _, ok := remove[output.NodeID]; ok {
			continue
		}
		filtered = append(filtered, output)
	}
	return filtered
}

func filterOutStrings(values []string, remove []string) []string {
	removeSet := make(map[string]struct{}, len(remove))
	for _, value := range remove {
		removeSet[value] = struct{}{}
	}
	filtered := values[:0]
	for _, value := range values {
		if _, ok := removeSet[value]; ok {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}

func removeString(values []string, target string) []string {
	filtered := values[:0]
	for _, value := range values {
		if value == target {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func rejectionReason(comment string) string {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return "rejected by user"
	}
	return "rejected by user: " + comment
}
