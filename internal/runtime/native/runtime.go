package native

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/runtime/agenttool"
	"github.com/zzqDeco/papersilm/internal/runtime/einoagent"
	runtimeinput "github.com/zzqDeco/papersilm/internal/runtime/input"
	"github.com/zzqDeco/papersilm/internal/runtime/prepare"
	"github.com/zzqDeco/papersilm/internal/runtime/turnloop"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/internal/tools"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type EventSink interface {
	Emit(event protocol.StreamEvent) error
}

type Runtime struct {
	cfg      config.Config
	store    *storage.Store
	registry *tools.Registry
	sink     EventSink
}

func New(cfg config.Config, store *storage.Store, registry *tools.Registry, sink EventSink) *Runtime {
	return &Runtime{cfg: cfg, store: store, registry: registry, sink: sink}
}

func (r *Runtime) HandleTurn(ctx context.Context, envelope turnloop.TurnEnvelope) (protocol.RunResult, error) {
	if len(envelope.Items) == 0 {
		return protocol.RunResult{}, fmt.Errorf("turn has no input items")
	}
	_ = r.store.AppendTurn(envelope.SessionID, map[string]any{
		"turn_id":    envelope.TurnID,
		"session_id": envelope.SessionID,
		"item_count": len(envelope.Items),
		"created_at": envelope.CreatedAt,
	})
	var result protocol.RunResult
	for _, item := range envelope.Items {
		next, err := r.handleItem(ctx, envelope.TurnID, item)
		if err != nil {
			return protocol.RunResult{}, err
		}
		result = next
	}
	return result, nil
}

func (r *Runtime) handleItem(ctx context.Context, turnID string, item runtimeinput.Item) (protocol.RunResult, error) {
	switch item.Kind {
	case runtimeinput.KindPermissionDecision:
		decision, _ := item.Payload.(protocol.PermissionDecision)
		return r.DecidePermission(ctx, item.SessionID, decision, turnID)
	case runtimeinput.KindRunPlanned:
		payload, _ := item.Payload.(runtimeinput.RunPlannedPayload)
		return r.RunPlanned(ctx, item.SessionID, payload.Language, payload.Style, turnID)
	case runtimeinput.KindTaskAction:
		payload, _ := item.Payload.(runtimeinput.TaskActionPayload)
		return r.HandleTaskAction(ctx, item.SessionID, payload, turnID)
	default:
		req, ok := item.Payload.(protocol.ClientRequest)
		if !ok {
			req = protocol.ClientRequest{
				SessionID: item.SessionID,
				Task:      item.Text,
			}
		}
		return r.Execute(ctx, req, turnID)
	}
}

func (r *Runtime) Execute(ctx context.Context, req protocol.ClientRequest, turnID string) (protocol.RunResult, error) {
	meta, err := r.store.LoadMeta(req.SessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	meta = syncMeta(meta, req)
	if len(req.Sources) > 0 {
		if _, err := r.AttachSources(ctx, req.SessionID, req.Sources, false); err != nil {
			return protocol.RunResult{}, err
		}
		meta, err = r.store.LoadMeta(req.SessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		meta = syncMeta(meta, req)
	}
	goal := strings.TrimSpace(req.Task)
	if goal == "" {
		goal = strings.TrimSpace(meta.LastTask)
	}
	if goal == "" {
		return protocol.RunResult{}, fmt.Errorf("task is required")
	}
	if err := r.store.InvalidatePlanState(req.SessionID); err != nil {
		return protocol.RunResult{}, err
	}
	meta, err = r.store.LoadMeta(req.SessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	meta = syncMeta(meta, req)
	meta.ApprovalPending = false
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
	meta.LastTask = goal
	meta.PermissionMode = req.PermissionMode
	meta.UpdatedAt = time.Now().UTC()

	plan, err := r.BuildPlan(ctx, req.SessionID, goal, req.PermissionMode == protocol.PermissionModeConfirm)
	if err != nil {
		_ = r.markSessionFailed(req.SessionID, err)
		return protocol.RunResult{}, err
	}
	if err := r.savePlanState(req.SessionID, plan); err != nil {
		_ = r.markSessionFailed(req.SessionID, err)
		return protocol.RunResult{}, err
	}
	if err := r.emit(req.SessionID, protocol.EventPlan, "plan created", plan); err != nil {
		return protocol.RunResult{}, err
	}
	if req.PermissionMode == protocol.PermissionModePlan {
		meta.State = protocol.SessionStatePlanned
		meta.ActivePlanID = plan.PlanID
		meta.ApprovalPending = false
		meta.UpdatedAt = time.Now().UTC()
		if err := r.store.SaveMeta(meta); err != nil {
			return protocol.RunResult{}, err
		}
		snapshot, err := r.store.Snapshot(req.SessionID)
		return protocol.RunResult{Session: snapshot, Plan: snapshot.Plan}, err
	}
	meta.State = protocol.SessionStateRunning
	meta.ApprovalPending = false
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
	meta.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	if req.PermissionMode == protocol.PermissionModeConfirm {
		approval, ok, err := r.nextPermissionRequest(ctx, req.SessionID, plan)
		if err != nil {
			_ = r.markSessionFailed(req.SessionID, err)
			return protocol.RunResult{}, err
		}
		if ok {
			return r.saveApproval(req.SessionID, plan, approval)
		}
	}
	return r.executePlan(ctx, req.SessionID, plan, turnID)
}

func (r *Runtime) RunPlanned(ctx context.Context, sessionID, lang, style, turnID string) (protocol.RunResult, error) {
	meta, err := r.store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if hasPendingApproval(r.store, sessionID, meta) {
		return protocol.RunResult{}, fmt.Errorf("session is awaiting approval; approve or reject the pending request before running")
	}
	if strings.TrimSpace(lang) != "" {
		meta.Language = lang
	}
	if strings.TrimSpace(style) != "" {
		meta.Style = style
	}
	meta.PermissionMode = protocol.PermissionModeAuto
	meta.State = protocol.SessionStateRunning
	meta.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	plan, err := r.store.LoadPlan(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if plan == nil {
		return protocol.RunResult{}, fmt.Errorf("no saved plan available")
	}
	return r.executePlan(ctx, sessionID, *plan, turnID)
}

func (r *Runtime) RunTask(ctx context.Context, sessionID, taskID, lang, style, turnID string) (protocol.RunResult, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return protocol.RunResult{}, fmt.Errorf("task id is required")
	}
	meta, err := r.store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if hasPendingApproval(r.store, sessionID, meta) {
		return protocol.RunResult{}, fmt.Errorf("session is awaiting approval; approve or reject the pending request before running a task")
	}
	if strings.TrimSpace(lang) != "" {
		meta.Language = lang
	}
	if strings.TrimSpace(style) != "" {
		meta.Style = style
	}
	permissionMode := meta.PermissionMode
	plan, err := r.store.LoadPlan(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if plan == nil {
		return protocol.RunResult{}, fmt.Errorf("no saved plan available")
	}
	step, ok := findPlanStepForTask(*plan, taskID)
	if !ok {
		return protocol.RunResult{}, fmt.Errorf("task not found: %s", taskID)
	}
	execState, err := r.store.LoadExecutionState(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if ok, blockedBy := stepDependenciesSatisfied(*plan, execState, step.ID); !ok {
		return protocol.RunResult{}, fmt.Errorf("task %s is blocked by unmet dependencies: %s", taskID, strings.Join(blockedBy, ", "))
	}
	if sideEffectTool(step.Tool) && permissionMode == protocol.PermissionModeConfirm {
		meta.PermissionMode = permissionMode
		meta.ActivePlanID = plan.PlanID
		meta.UpdatedAt = time.Now().UTC()
		if err := r.store.SaveMeta(meta); err != nil {
			return protocol.RunResult{}, err
		}
		request, err := permissionRequestForStep(r.store, sessionID, plan.PlanID, step)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return r.saveApproval(sessionID, *plan, approvalFromRequest(*plan, request, "task"))
	}
	runMode := permissionMode
	if runMode == "" || runMode == protocol.PermissionModePlan {
		runMode = protocol.PermissionModeAuto
	}
	meta.PermissionMode = runMode
	meta.State = protocol.SessionStateRunning
	meta.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	out, err := r.executeStep(ctx, sessionID, plan.Goal, step)
	if err != nil {
		_ = r.markStep(sessionID, plan.PlanID, step.ID, protocol.NodeStatusFailed, err.Error(), out)
		_ = r.markSessionFailed(sessionID, err)
		return protocol.RunResult{}, err
	}
	if err := r.markStep(sessionID, plan.PlanID, step.ID, protocol.NodeStatusCompleted, "", out); err != nil {
		return protocol.RunResult{}, err
	}
	state, _ := r.store.LoadExecutionState(sessionID)
	if state != nil && executionFinalized(state.Nodes) {
		meta.State = protocol.SessionStateCompleted
	} else {
		meta.State = protocol.SessionStatePlanned
	}
	meta.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	snapshot, err := r.store.Snapshot(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	response := fmt.Sprintf("Task %s completed.", step.ID)
	if text, ok := out.Data["response"].(string); ok && strings.TrimSpace(text) != "" {
		response = text
	}
	result := protocol.RunResult{Session: snapshot, Plan: snapshot.Plan, Digests: snapshot.Digests, Comparison: snapshot.Compare, Artifacts: snapshot.Artifacts, Response: response}
	_ = r.emit(sessionID, protocol.EventResult, "task completed", map[string]any{
		"turn_id":  turnID,
		"task_id":  step.ID,
		"response": response,
	})
	return result, nil
}

func (r *Runtime) HandleTaskAction(ctx context.Context, sessionID string, payload runtimeinput.TaskActionPayload, turnID string) (protocol.RunResult, error) {
	switch payload.Action {
	case "approve":
		requestID, err := r.permissionRequestIDForTask(sessionID, payload.TaskID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return r.DecidePermission(ctx, sessionID, protocol.PermissionDecision{RequestID: requestID, Value: agenttool.PermissionAcceptOnce, Feedback: payload.Comment}, turnID)
	case "reject":
		requestID, err := r.permissionRequestIDForTask(sessionID, payload.TaskID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return r.DecidePermission(ctx, sessionID, protocol.PermissionDecision{RequestID: requestID, Value: agenttool.PermissionReject, Feedback: payload.Comment}, turnID)
	case "run":
		return r.RunTask(ctx, sessionID, payload.TaskID, payload.Language, payload.Style, turnID)
	default:
		return protocol.RunResult{}, fmt.Errorf("unknown task action: %s", payload.Action)
	}
}

func (r *Runtime) DecidePermission(ctx context.Context, sessionID string, decision protocol.PermissionDecision, turnID string) (protocol.RunResult, error) {
	approval, err := r.store.LoadPendingApproval(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if approval == nil {
		return protocol.RunResult{}, fmt.Errorf("session has no pending approval")
	}
	request, ok := findPermissionRequest(approval, decision.RequestID)
	if !ok {
		return protocol.RunResult{}, fmt.Errorf("permission request not found")
	}
	if strings.TrimSpace(decision.Value) == "" {
		decision.Value = agenttool.PermissionAcceptOnce
	}
	if approval.Mode == "tool" && approval.CheckpointID != "" && approval.InterruptID != "" {
		if decision.Value == agenttool.PermissionAcceptSession {
			if err := agenttool.AddPermissionRule(r.store, sessionID, request, decision); err != nil {
				return protocol.RunResult{}, err
			}
		}
		interruptID := firstNonEmpty(request.InterruptID, approval.InterruptID)
		meta, err := r.store.LoadMeta(sessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		prepared, err := prepare.Agent(ctx, prepare.Request{
			Config:         r.cfg,
			Store:          r.store,
			Registry:       r.registry,
			SessionID:      sessionID,
			PermissionMode: protocol.PermissionModeConfirm,
			Language:       meta.Language,
			Style:          meta.Style,
		})
		if err != nil {
			return protocol.RunResult{}, err
		}
		output, err := einoagent.Resume(ctx, einoagent.ResumeRequest{
			Store:      r.store,
			Prepared:   prepared,
			SessionID:  sessionID,
			TurnID:     turnID,
			Checkpoint: approval.CheckpointID,
			Targets: map[string]any{
				interruptID: decision,
			},
		})
		if err != nil {
			return protocol.RunResult{}, err
		}
		return r.finishEinoOutput(sessionID, output)
	}
	if decision.Value == agenttool.PermissionAcceptSession {
		if err := agenttool.AddPermissionRule(r.store, sessionID, request, decision); err != nil {
			return protocol.RunResult{}, err
		}
	}
	if decision.Value == agenttool.PermissionReject {
		return r.rejectPermission(sessionID, approval, request, decision)
	}
	if _, err := r.applyPermissionRequest(sessionID, request); err != nil {
		return protocol.RunResult{}, err
	}
	return r.continueAfterPermission(ctx, sessionID, turnID, approval, request)
}

func (r *Runtime) AttachSources(ctx context.Context, sessionID string, sources []string, replace bool) (protocol.SessionSnapshot, error) {
	existing, err := r.store.LoadSources(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	base := existing
	if replace {
		base = nil
	}
	refs, err := r.registry.ResolveSources(ctx, sessionID, base, sources)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	if err := r.registry.CommitSources(r.store, sessionID, refs); err != nil {
		return protocol.SessionSnapshot{}, err
	}
	if replace {
		_ = r.store.DeleteWorkspaceStates(sessionID, removedWorkspacePaperIDs(existing, refs))
	}
	if err := r.emit(sessionID, protocol.EventSourceAttached, "sources attached", refs); err != nil {
		return protocol.SessionSnapshot{}, err
	}
	return r.store.Snapshot(sessionID)
}

func (r *Runtime) BuildPlan(ctx context.Context, sessionID, goal string, approvalRequired bool) (protocol.PlanResult, error) {
	files, _ := r.registry.LoadWorkspaceFiles(r.store)
	intent := inferWorkspaceIntent(goal, files)
	if preferWorkspacePlan(goal, intent) {
		return buildWorkspacePlan(goal, approvalRequired, intent), nil
	}
	refs, err := r.ensurePaperContext(ctx, sessionID, goal)
	if err != nil {
		return protocol.PlanResult{}, err
	}
	if len(refs) == 0 {
		return buildWorkspacePlan(goal, approvalRequired, intent), nil
	}
	return buildPaperPlan(goal, refs, approvalRequired), nil
}

func (r *Runtime) executePlan(ctx context.Context, sessionID string, plan protocol.PlanResult, turnID string) (protocol.RunResult, error) {
	if len(plan.Steps) == 0 {
		return r.runEinoAssistant(ctx, sessionID, plan.Goal, protocol.PermissionModeAuto, turnID)
	}
	var response string
	for _, step := range plan.Steps {
		out, err := r.executeStep(ctx, sessionID, plan.Goal, step)
		if err != nil {
			_ = r.markStep(sessionID, plan.PlanID, step.ID, protocol.NodeStatusFailed, err.Error(), protocol.NodeOutputRef{})
			_ = r.markSessionFailed(sessionID, err)
			return protocol.RunResult{}, err
		}
		_ = r.markStep(sessionID, plan.PlanID, step.ID, protocol.NodeStatusCompleted, "", out)
		if text, ok := out.Data["response"].(string); ok {
			response = appendText(response, text)
		}
	}
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
	result := protocol.RunResult{
		Session:    snapshot,
		Plan:       snapshot.Plan,
		Digests:    snapshot.Digests,
		Comparison: snapshot.Compare,
		Artifacts:  snapshot.Artifacts,
		Response:   strings.TrimSpace(response),
	}
	if result.Response == "" {
		result.Response = summarizeResult(result)
	}
	if err := r.emit(sessionID, protocol.EventResult, "run completed", map[string]any{
		"turn_id":  turnID,
		"response": result.Response,
	}); err != nil {
		return protocol.RunResult{}, err
	}
	return result, nil
}

func (r *Runtime) runEinoAssistant(ctx context.Context, sessionID, userMessage string, mode protocol.PermissionMode, turnID string) (protocol.RunResult, error) {
	meta, err := r.store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	prepared, err := prepare.Agent(ctx, prepare.Request{
		Config:         r.cfg,
		Store:          r.store,
		Registry:       r.registry,
		SessionID:      sessionID,
		PermissionMode: mode,
		Language:       meta.Language,
		Style:          meta.Style,
	})
	if err != nil {
		return protocol.RunResult{}, err
	}
	checkpointID := fmt.Sprintf("turn_%s", turnID)
	output, err := einoagent.Run(ctx, einoagent.RunRequest{
		Store:       r.store,
		Prepared:    prepared,
		SessionID:   sessionID,
		TurnID:      turnID,
		Checkpoint:  checkpointID,
		UserMessage: userMessage,
	})
	if err != nil {
		if persistErr := r.persistEinoEvents(sessionID, output); persistErr != nil {
			return protocol.RunResult{}, persistErr
		}
		_ = r.markSessionFailed(sessionID, err)
		return protocol.RunResult{}, err
	}
	return r.finishEinoOutput(sessionID, output)
}

func (r *Runtime) finishEinoOutput(sessionID string, output einoagent.RunOutput) (protocol.RunResult, error) {
	if err := r.persistEinoEvents(sessionID, output); err != nil {
		return protocol.RunResult{}, err
	}
	meta, err := r.store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if output.Interrupted != nil {
		meta.State = protocol.SessionStateAwaitingApproval
		meta.ApprovalPending = true
		meta.ActiveCheckpointID = output.Interrupted.CheckpointID
		meta.PendingInterruptID = output.Interrupted.InterruptID
		meta.UpdatedAt = time.Now().UTC()
		if err := r.store.SaveMeta(meta); err != nil {
			return protocol.RunResult{}, err
		}
		if err := r.store.SavePendingApproval(sessionID, *output.Interrupted); err != nil {
			return protocol.RunResult{}, err
		}
		snapshot, err := r.store.Snapshot(sessionID)
		return protocol.RunResult{Session: snapshot, Approval: snapshot.Approval, Response: output.Response}, err
	}
	meta.State = protocol.SessionStateCompleted
	meta.ApprovalPending = false
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
	meta.UpdatedAt = time.Now().UTC()
	if err := r.store.DeletePendingApproval(sessionID); err != nil {
		return protocol.RunResult{}, err
	}
	if err := r.store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	snapshot, err := r.store.Snapshot(sessionID)
	return protocol.RunResult{Session: snapshot, Plan: snapshot.Plan, Digests: snapshot.Digests, Comparison: snapshot.Compare, Artifacts: snapshot.Artifacts, Response: output.Response}, err
}

func (r *Runtime) persistEinoEvents(sessionID string, output einoagent.RunOutput) error {
	for _, event := range output.Events {
		if err := r.store.AppendEvent(sessionID, event); err != nil {
			return err
		}
		if r.sink != nil {
			if err := r.sink.Emit(event); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Runtime) markSessionFailed(sessionID string, runErr error) error {
	_ = runErr
	meta, err := r.store.LoadMeta(sessionID)
	if err != nil {
		return err
	}
	meta.State = protocol.SessionStateFailed
	meta.ApprovalPending = false
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
	meta.UpdatedAt = time.Now().UTC()
	return r.store.SaveMeta(meta)
}

func (r *Runtime) executeStep(ctx context.Context, sessionID, goal string, step protocol.PlanStep) (protocol.NodeOutputRef, error) {
	now := time.Now().UTC()
	switch step.Tool {
	case string(protocol.NodeKindWorkspaceInspect):
		files, err := r.registry.LoadWorkspaceFiles(r.store)
		if err != nil {
			return protocol.NodeOutputRef{}, err
		}
		response := summarizeWorkspace(r.store, files)
		return nodeOutput(step.ID, "workspace_response", response, now), nil
	case string(protocol.NodeKindWorkspaceSearch):
		hits, err := r.registry.SearchWorkspace(ctx, r.store, step.Goal, 20)
		if err != nil {
			return protocol.NodeOutputRef{}, err
		}
		return nodeOutput(step.ID, "workspace_response", formatSearchHits(step.Goal, hits), now), nil
	case string(protocol.NodeKindWorkspaceCommand):
		intent := workspaceIntent{kind: protocol.NodeKindWorkspaceCommand, command: extractBacktickCommand(step.Goal)}
		record, err := r.registry.RunWorkspaceCommand(r.store, intent.command)
		text := formatCommandRecord(record)
		if err != nil {
			return nodeOutput(step.ID, "workspace_response", text, now), err
		}
		return nodeOutput(step.ID, "workspace_response", text, now), nil
	case string(protocol.NodeKindWorkspaceEdit):
		files, _ := r.registry.LoadWorkspaceFiles(r.store)
		intent := inferWorkspaceIntent(step.Goal, files)
		request, err := editPermissionRequest(r.store, sessionID, "", step.ID, step.Goal, intent.targetPath)
		if err != nil {
			return protocol.NodeOutputRef{}, err
		}
		result, err := applyApprovedEdit(r, sessionID, request)
		return nodeOutput(step.ID, "workspace_response", result, now), err
	case "distill_paper":
		out, err := r.invokeExecutionTool(ctx, sessionID, "distill_paper", tools.DistillToolInput{
			PaperID: step.PaperIDs[0],
			Goal:    goal,
			Lang:    "",
			Style:   "",
		})
		return protocol.NodeOutputRef{NodeID: step.ID, Kind: "paper_digest", Ref: step.PaperIDs[0], Data: map[string]any{"response": out}, CreatedAt: now}, err
	case "compare_papers":
		out, err := r.invokeExecutionTool(ctx, sessionID, "compare_papers", tools.CompareToolInput{
			PaperIDs: step.PaperIDs,
			Goal:     goal,
		})
		return protocol.NodeOutputRef{NodeID: step.ID, Kind: "comparison", Ref: "comparison", Data: map[string]any{"response": out}, CreatedAt: now}, err
	default:
		result, err := r.runEinoAssistant(ctx, sessionID, step.Goal, protocol.PermissionModeAuto, step.ID)
		return nodeOutput(step.ID, "assistant_response", result.Response, now), err
	}
}

func (r *Runtime) invokeExecutionTool(ctx context.Context, sessionID, name string, input any) (string, error) {
	execTools, err := r.registry.BuildExecutionTools(ctx, r.store, sessionID, false)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	for _, base := range execTools {
		info, err := base.Info(ctx)
		if err != nil || info == nil || info.Name != name {
			continue
		}
		invokable, ok := base.(tool.InvokableTool)
		if !ok {
			return "", fmt.Errorf("tool %s is not invokable", name)
		}
		return invokable.InvokableRun(ctx, string(raw))
	}
	return "", fmt.Errorf("tool not found: %s", name)
}

func (r *Runtime) emit(sessionID string, eventType protocol.StreamEventType, message string, payload interface{}) error {
	event := protocol.StreamEvent{Type: eventType, SessionID: sessionID, Message: message, Payload: payload, CreatedAt: time.Now().UTC()}
	if r.sink != nil {
		if err := r.sink.Emit(event); err != nil {
			return err
		}
	}
	return r.store.AppendEvent(sessionID, event)
}

func syncMeta(meta protocol.SessionMeta, req protocol.ClientRequest) protocol.SessionMeta {
	if strings.TrimSpace(req.Language) != "" {
		meta.Language = req.Language
	}
	if strings.TrimSpace(req.Style) != "" {
		meta.Style = req.Style
	}
	if req.PermissionMode != "" {
		meta.PermissionMode = req.PermissionMode
	}
	return meta
}

func nodeOutput(nodeID, kind, response string, createdAt time.Time) protocol.NodeOutputRef {
	return protocol.NodeOutputRef{NodeID: nodeID, Kind: kind, Data: map[string]any{"response": response}, CreatedAt: createdAt}
}

func appendText(current, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return current
	}
	if strings.TrimSpace(current) == "" {
		return next
	}
	return current + "\n\n" + next
}

func summarizeResult(result protocol.RunResult) string {
	switch {
	case len(result.Digests) > 0 && result.Comparison != nil:
		return fmt.Sprintf("Generated %d paper digests and a comparison.", len(result.Digests))
	case len(result.Digests) > 0:
		return fmt.Sprintf("Generated %d paper digest(s).", len(result.Digests))
	default:
		return "Task completed."
	}
}

func removedWorkspacePaperIDs(existing, next []protocol.PaperRef) []string {
	kept := make(map[string]struct{}, len(next))
	for _, ref := range next {
		kept[ref.PaperID] = struct{}{}
	}
	removed := make([]string, 0)
	for _, ref := range existing {
		if ref.PaperID == "" {
			continue
		}
		if _, ok := kept[ref.PaperID]; !ok {
			removed = append(removed, ref.PaperID)
		}
	}
	sort.Strings(removed)
	return removed
}
