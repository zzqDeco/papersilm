package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/internal/tools"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type EventSink interface {
	Emit(event protocol.StreamEvent) error
}

type Agent struct {
	cfg   config.Config
	tools *tools.Registry
}

func New(registry *tools.Registry, cfg config.Config) *Agent {
	return &Agent{
		cfg:   cfg,
		tools: registry,
	}
}

func (a *Agent) AttachSources(ctx context.Context, store *storage.Store, sink EventSink, sessionID string, sources []string, replace bool) (protocol.SessionSnapshot, error) {
	if replace {
		if err := store.SaveSources(sessionID, nil); err != nil {
			return protocol.SessionSnapshot{}, err
		}
		if err := store.InvalidatePlanState(sessionID); err != nil {
			return protocol.SessionSnapshot{}, err
		}
	}
	refs, err := a.tools.AttachSources(ctx, store, sessionID, sources)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	if err := a.emit(store, sink, sessionID, protocol.EventSourceAttached, "sources attached", refs); err != nil {
		return protocol.SessionSnapshot{}, err
	}
	return store.Snapshot(sessionID)
}

func (a *Agent) Execute(ctx context.Context, store *storage.Store, sink EventSink, req protocol.ClientRequest) (protocol.RunResult, error) {
	meta, err := store.LoadMeta(req.SessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	meta, err = a.syncSessionConfig(store, meta, req.Language, req.Style)
	if err != nil {
		return protocol.RunResult{}, err
	}

	if len(req.Sources) > 0 {
		refs, err := a.tools.AttachSources(ctx, store, req.SessionID, req.Sources)
		if err != nil {
			return protocol.RunResult{}, err
		}
		if err := a.emit(store, sink, req.SessionID, protocol.EventSourceAttached, "sources attached", refs); err != nil {
			return protocol.RunResult{}, err
		}
		meta, err = store.LoadMeta(req.SessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
	}

	goal := strings.TrimSpace(req.Task)
	if goal == "" {
		goal = strings.TrimSpace(meta.LastTask)
	}
	if goal == "" {
		return protocol.RunResult{}, fmt.Errorf("task is required")
	}

	planResult, execState, err := a.planSession(ctx, store, sink, req.SessionID, goal, req.PermissionMode == protocol.PermissionModeConfirm)
	if err != nil {
		return protocol.RunResult{}, err
	}

	meta, err = store.LoadMeta(req.SessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	meta.LastTask = goal
	meta.ActivePlanID = planResult.PlanID
	meta.PermissionMode = req.PermissionMode
	meta.UpdatedAt = time.Now().UTC()
	switch req.PermissionMode {
	case protocol.PermissionModePlan:
		meta.State = protocol.SessionStatePlanned
		meta.ApprovalPending = false
		meta.ActiveCheckpointID = ""
		meta.PendingInterruptID = ""
		if err := store.SaveMeta(meta); err != nil {
			return protocol.RunResult{}, err
		}
		snapshot, err := store.Snapshot(req.SessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return protocol.RunResult{Session: snapshot, Plan: &planResult}, nil
	case protocol.PermissionModeConfirm:
		return a.startConfirmExecution(store, req.SessionID, meta, planResult, execState)
	default:
		meta.State = protocol.SessionStateRunning
		meta.ApprovalPending = false
		if err := store.SaveMeta(meta); err != nil {
			return protocol.RunResult{}, err
		}
		return a.runDAGExecution(ctx, store, sink, req.SessionID, meta, planResult, execState, goal, req.Language, req.Style)
	}
}

func (a *Agent) RunPlanned(ctx context.Context, store *storage.Store, sink EventSink, sessionID, lang, style string) (protocol.RunResult, error) {
	meta, err := store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if meta.State == protocol.SessionStateAwaitingApproval {
		return protocol.RunResult{}, fmt.Errorf("session is awaiting approval; use /approve")
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
		state := buildExecutionState(planResult.PlanID, planResult.DAG)
		execState = &state
		if err := store.SaveExecutionState(sessionID, *execState); err != nil {
			return protocol.RunResult{}, err
		}
	}
	meta, err = a.syncSessionConfig(store, meta, lang, style)
	if err != nil {
		return protocol.RunResult{}, err
	}
	meta.State = protocol.SessionStateRunning
	meta.PermissionMode = protocol.PermissionModeAuto
	meta.ApprovalPending = false
	meta.UpdatedAt = time.Now().UTC()
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	return a.runDAGExecution(ctx, store, sink, sessionID, meta, *planResult, execState, meta.LastTask, meta.Language, meta.Style)
}

func (a *Agent) Approve(ctx context.Context, store *storage.Store, sink EventSink, sessionID string, approved bool, comment string) (protocol.RunResult, error) {
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
	if meta.ActiveCheckpointID == "" || meta.PendingInterruptID == "" {
		return protocol.RunResult{}, fmt.Errorf("session has no pending approval")
	}
	if !approved {
		meta.State = protocol.SessionStatePlanned
		meta.ApprovalPending = false
		meta.ActiveCheckpointID = ""
		meta.PendingInterruptID = ""
		meta.UpdatedAt = time.Now().UTC()
		execState.PendingNodeIDs = nil
		if err := store.SaveExecutionState(sessionID, *execState); err != nil {
			return protocol.RunResult{}, err
		}
		if err := store.SaveMeta(meta); err != nil {
			return protocol.RunResult{}, err
		}
		snapshot, err := store.Snapshot(sessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return protocol.RunResult{Session: snapshot, Plan: planResult}, nil
	}

	meta.State = protocol.SessionStateRunning
	meta.ApprovalPending = false
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
	meta.UpdatedAt = time.Now().UTC()
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	return a.runDAGExecution(ctx, store, sink, sessionID, meta, *planResult, execState, meta.LastTask, meta.Language, meta.Style)
}

func (a *Agent) syncSessionConfig(store *storage.Store, meta protocol.SessionMeta, lang, style string) (protocol.SessionMeta, error) {
	changed := false
	if strings.TrimSpace(lang) != "" && meta.Language != lang {
		meta.Language = lang
		changed = true
	}
	if strings.TrimSpace(style) != "" && meta.Style != style {
		meta.Style = style
		changed = true
	}
	if !changed {
		return meta, nil
	}
	if err := store.SaveMeta(meta); err != nil {
		return protocol.SessionMeta{}, err
	}
	if err := store.InvalidatePlanState(meta.SessionID); err != nil {
		return protocol.SessionMeta{}, err
	}
	return store.LoadMeta(meta.SessionID)
}

func (a *Agent) planSession(ctx context.Context, store *storage.Store, sink EventSink, sessionID, goal string, approvalRequired bool) (protocol.PlanResult, *protocol.ExecutionState, error) {
	if err := store.InvalidatePlanState(sessionID); err != nil {
		return protocol.PlanResult{}, nil, err
	}
	refs, err := a.tools.InspectSources(ctx, store, sessionID, nil)
	if err != nil {
		return protocol.PlanResult{}, nil, err
	}
	if len(refs) == 0 {
		return protocol.PlanResult{}, nil, fmt.Errorf("no sources attached")
	}
	if err := a.emit(store, sink, sessionID, protocol.EventAnalysis, "source inspection complete", refs); err != nil {
		return protocol.PlanResult{}, nil, err
	}

	specs := buildTaskSpecs(goal, refs)
	dag := compileDAG(goal, refs, specs)
	planResult := protocol.PlanResult{
		PlanID:           newPlanID(),
		Goal:             strings.TrimSpace(goal),
		SourceSummary:    refs,
		DAG:              dag,
		Steps:            projectSteps(dag),
		WillCompare:      hasComparisonNode(dag),
		Risks:            planRisks(refs),
		ApprovalRequired: approvalRequired,
		CreatedAt:        time.Now().UTC(),
	}
	state := buildExecutionState(planResult.PlanID, dag)
	if err := store.SavePlan(sessionID, planResult); err != nil {
		return protocol.PlanResult{}, nil, err
	}
	if err := store.SaveExecutionState(sessionID, state); err != nil {
		return protocol.PlanResult{}, nil, err
	}
	if err := a.emit(store, sink, sessionID, protocol.EventPlan, "plan ready", planResult); err != nil {
		return protocol.PlanResult{}, nil, err
	}
	return planResult, &state, nil
}

func (a *Agent) startConfirmExecution(store *storage.Store, sessionID string, meta protocol.SessionMeta, planResult protocol.PlanResult, execState *protocol.ExecutionState) (protocol.RunResult, error) {
	batch := selectBatch(planResult.DAG)
	checkpointID := fmt.Sprintf("%s_confirm_%d", sessionID, time.Now().UnixNano())
	interruptID := fmt.Sprintf("approval_%d", time.Now().UnixNano())
	execState.PendingNodeIDs = append([]string(nil), batch...)
	execState.CurrentBatchID = fmt.Sprintf("batch_%d", time.Now().UnixNano())
	execState.UpdatedAt = time.Now().UTC()
	if err := store.SaveExecutionState(sessionID, *execState); err != nil {
		return protocol.RunResult{}, err
	}
	meta.State = protocol.SessionStateAwaitingApproval
	meta.ApprovalPending = true
	meta.ActiveCheckpointID = checkpointID
	meta.PendingInterruptID = interruptID
	meta.UpdatedAt = time.Now().UTC()
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	snapshot, err := store.Snapshot(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	return protocol.RunResult{
		Session: snapshot,
		Plan:    &planResult,
		Approval: &protocol.ApprovalRequest{
			PlanID:         planResult.PlanID,
			CheckpointID:   checkpointID,
			InterruptID:    interruptID,
			PendingNodeIDs: append([]string(nil), batch...),
			Summary:        approvalSummary(planResult, batch),
			RequiresInput:  true,
			CreatedAt:      time.Now().UTC(),
		},
	}, nil
}

func (a *Agent) runDAGExecution(ctx context.Context, store *storage.Store, sink EventSink, sessionID string, meta protocol.SessionMeta, planResult protocol.PlanResult, execState *protocol.ExecutionState, goal, lang, style string) (protocol.RunResult, error) {
	for {
		refreshReadyNodes(&planResult.DAG)
		batch := selectBatch(planResult.DAG)
		if len(batch) == 0 {
			patch := deriveDagPatch(goal, planResult.DAG, *execState)
			if hasPatchWork(patch) {
				if err := applyDagPatch(&planResult.DAG, execState, patch); err != nil {
					return protocol.RunResult{}, err
				}
				planResult.Steps = projectSteps(planResult.DAG)
				planResult.WillCompare = hasComparisonNode(planResult.DAG)
				if err := store.SavePlan(sessionID, planResult); err != nil {
					return protocol.RunResult{}, err
				}
				if err := store.SaveExecutionState(sessionID, *execState); err != nil {
					return protocol.RunResult{}, err
				}
				if err := a.emit(store, sink, sessionID, protocol.EventProgress, "plan replanned", protocol.PlanProgress{
					PlanID:    planResult.PlanID,
					Status:    protocol.PlanProgressReplanned,
					Message:   fallbackPatchReason(patch.Reason),
					CreatedAt: time.Now().UTC(),
				}); err != nil {
					return protocol.RunResult{}, err
				}
				if execState.Finalized {
					break
				}
				continue
			}
			if execState.Finalized || planFullySettled(planResult.DAG) {
				break
			}
			return protocol.RunResult{}, fmt.Errorf("dag execution stalled with no ready nodes")
		}

		batchID := fmt.Sprintf("batch_%d", time.Now().UnixNano())
		execState.CurrentBatchID = batchID
		execState.PendingNodeIDs = append([]string(nil), batch...)
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
		execState.PendingNodeIDs = nil
		markBatchCompleted(execState, batchID)
		refreshReadyNodes(&planResult.DAG)
		if err := store.SavePlan(sessionID, planResult); err != nil {
			return protocol.RunResult{}, err
		}
		if err := store.SaveExecutionState(sessionID, *execState); err != nil {
			return protocol.RunResult{}, err
		}
	}

	meta.State = protocol.SessionStateCompleted
	meta.ApprovalPending = false
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
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
	if err := a.emit(store, sink, sessionID, protocol.EventResult, "run completed", map[string]any{
		"digests":    len(snapshot.Digests),
		"comparison": snapshot.Compare != nil,
		"artifacts":  len(snapshot.Artifacts),
	}); err != nil {
		return protocol.RunResult{}, err
	}
	return protocol.RunResult{
		Session:    snapshot,
		Plan:       &planResult,
		Digests:    snapshot.Digests,
		Comparison: snapshot.Compare,
		Artifacts:  snapshot.Artifacts,
	}, nil
}

func (a *Agent) executeBatch(ctx context.Context, store *storage.Store, sessionID, goal, lang, style string, execState *protocol.ExecutionState, dag protocol.PlanDAG, batch []string) []nodeResult {
	results := make([]nodeResult, 0, len(batch))
	ch := make(chan nodeResult, len(batch))
	var wg sync.WaitGroup
	for _, nodeID := range batch {
		node, ok := dagNode(dag, nodeID)
		if !ok {
			ch <- nodeResult{nodeID: nodeID, err: fmt.Errorf("node not found: %s", nodeID)}
			continue
		}
		wg.Add(1)
		go func(node protocol.PlanNode) {
			defer wg.Done()
			outputs, err := a.executeNode(ctx, store, sessionID, goal, lang, style, execState, node)
			ch <- nodeResult{nodeID: node.ID, outputs: outputs, err: err}
		}(node)
	}
	wg.Wait()
	close(ch)
	for result := range ch {
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].nodeID < results[j].nodeID
	})
	return results
}

func deriveDagPatch(goal string, dag protocol.PlanDAG, state protocol.ExecutionState) protocol.DagPatch {
	patch := protocol.DagPatch{}
	if goalNeedsMath(goal) {
		for _, node := range dag.Nodes {
			if node.Kind != protocol.NodeKindMergeDigest || node.Status == protocol.NodeStatusCompleted {
				continue
			}
			paperID := node.PaperIDs[0]
			mathID := "math_reasoner_" + paperID
			if _, ok := dagNode(dag, mathID); ok {
				continue
			}
			patch.AddNodes = append(patch.AddNodes, protocol.PlanNode{
				ID:            mathID,
				Kind:          protocol.NodeKindMathReasoner,
				Goal:          node.Goal,
				PaperIDs:      []string{paperID},
				WorkerProfile: protocol.WorkerProfileMathReasoner,
				Required:      false,
				Status:        protocol.NodeStatusPending,
				ParallelGroup: "research",
			})
			patch.AddEdges = append(patch.AddEdges, protocol.PlanEdge{From: mathID, To: node.ID})
			if patch.Reason == "" {
				patch.Reason = "added missing math reasoning nodes for detail-oriented request"
			}
		}
	}

	if shouldPruneCompare(dag) {
		for _, node := range dag.Nodes {
			switch node.Kind {
			case protocol.NodeKindMethodCompare, protocol.NodeKindExperimentCompare, protocol.NodeKindResultsCompare, protocol.NodeKindFinalSynthesis, protocol.NodeKind("compare_papers"):
				patch.MarkSkipped = append(patch.MarkSkipped, node.ID)
			}
		}
		if patch.Reason == "" {
			patch.Reason = "pruned comparison branch because fewer than two digest branches remain viable"
		}
	}

	if allTerminalNodesSettled(dag) {
		patch.Finalize = true
		if patch.Reason == "" {
			patch.Reason = "all terminal nodes settled"
		}
	}
	_ = state
	return patch
}

func shouldPruneCompare(dag protocol.PlanDAG) bool {
	possible := 0
	for _, node := range dag.Nodes {
		if node.Kind != protocol.NodeKindMergeDigest && node.Kind != protocol.NodeKind("distill_paper") {
			continue
		}
		switch node.Status {
		case protocol.NodeStatusCompleted, protocol.NodeStatusReady, protocol.NodeStatusPending, protocol.NodeStatusRunning:
			possible++
		case protocol.NodeStatusFailed:
			if !node.Required {
				possible++
			}
		}
	}
	hasCompare := false
	for _, node := range dag.Nodes {
		switch node.Kind {
		case protocol.NodeKindMethodCompare, protocol.NodeKindExperimentCompare, protocol.NodeKindResultsCompare, protocol.NodeKindFinalSynthesis, protocol.NodeKind("compare_papers"):
			hasCompare = true
		}
	}
	return hasCompare && possible < 2
}

func allTerminalNodesSettled(dag protocol.PlanDAG) bool {
	for _, node := range dag.Nodes {
		if len(outgoingNodeIDs(dag, node.ID)) > 0 {
			continue
		}
		switch node.Status {
		case protocol.NodeStatusCompleted, protocol.NodeStatusSkipped:
		default:
			return false
		}
	}
	return true
}

func outgoingNodeIDs(dag protocol.PlanDAG, nodeID string) []string {
	out := make([]string, 0, 2)
	for _, edge := range dag.Edges {
		if edge.From == nodeID {
			out = append(out, edge.To)
		}
	}
	return out
}

func markBatchCompleted(state *protocol.ExecutionState, batchID string) {
	now := time.Now().UTC()
	for i := range state.BatchHistory {
		if state.BatchHistory[i].BatchID != batchID {
			continue
		}
		state.BatchHistory[i].Status = protocol.BatchStatusCompleted
		state.BatchHistory[i].CompletedAt = now
	}
	state.CurrentBatchID = ""
	state.UpdatedAt = now
}

func hasComparisonNode(dag protocol.PlanDAG) bool {
	for _, node := range dag.Nodes {
		switch node.Kind {
		case protocol.NodeKindMethodCompare, protocol.NodeKindExperimentCompare, protocol.NodeKindResultsCompare, protocol.NodeKindFinalSynthesis, protocol.NodeKind("compare_papers"):
			return true
		}
	}
	return false
}

func planRisks(refs []protocol.PaperRef) []string {
	risks := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Inspection.FailureReason != "" {
			risks = append(risks, fmt.Sprintf("%s: %s", ref.PaperID, ref.Inspection.FailureReason))
			continue
		}
		if !ref.Inspection.ExtractableText {
			risks = append(risks, fmt.Sprintf("%s: pdf text extraction produced too little text", ref.PaperID))
		}
	}
	if len(risks) == 0 {
		risks = append(risks, "no major inspection risks detected")
	}
	return risks
}

func newPlanID() string {
	return fmt.Sprintf("plan_%d", time.Now().UnixNano())
}

func approvalSummary(plan protocol.PlanResult, nodeIDs []string) string {
	if len(nodeIDs) == 0 {
		return fmt.Sprintf("Plan %s is ready to execute.", plan.PlanID)
	}
	return fmt.Sprintf("Plan %s is ready to execute next batch: %s", plan.PlanID, strings.Join(nodeIDs, ", "))
}

func hasPatchWork(patch protocol.DagPatch) bool {
	return len(patch.AddNodes) > 0 || len(patch.AddEdges) > 0 || len(patch.RemoveNodes) > 0 ||
		len(patch.RemoveEdges) > 0 || len(patch.MarkSkipped) > 0 || len(patch.MarkComplete) > 0 || patch.Finalize
}

func fallbackPatchReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "dag updated"
	}
	return reason
}

func planFullySettled(dag protocol.PlanDAG) bool {
	for _, node := range dag.Nodes {
		switch node.Status {
		case protocol.NodeStatusCompleted, protocol.NodeStatusSkipped, protocol.NodeStatusFailed:
		default:
			return false
		}
	}
	return true
}

func (a *Agent) emit(store *storage.Store, sink EventSink, sessionID string, eventType protocol.StreamEventType, message string, payload any) error {
	event := protocol.StreamEvent{
		Type:      eventType,
		SessionID: sessionID,
		Message:   message,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}
	if sink != nil {
		if err := sink.Emit(event); err != nil {
			return err
		}
	}
	return store.AppendEvent(sessionID, event)
}
