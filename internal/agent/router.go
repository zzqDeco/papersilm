package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/providers"
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

	planResult, execPlan, _, err := a.planSession(ctx, store, sink, req.SessionID, goal, req.PermissionMode == protocol.PermissionModeConfirm)
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
	if req.PermissionMode == protocol.PermissionModePlan {
		meta.State = protocol.SessionStatePlanned
		meta.ApprovalPending = false
	} else {
		meta.State = protocol.SessionStateRunning
		meta.ApprovalPending = false
	}
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}

	switch req.PermissionMode {
	case protocol.PermissionModePlan:
		snapshot, err := store.Snapshot(req.SessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return protocol.RunResult{Session: snapshot, Plan: &planResult}, nil
	case protocol.PermissionModeConfirm:
		return a.startConfirmExecution(ctx, store, sink, req.SessionID, meta, planResult, execPlan, req.Language, req.Style)
	default:
		return a.runExecution(ctx, store, sink, req.SessionID, meta, planResult, execPlan, req.Language, req.Style, false)
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
	meta, err = a.syncSessionConfig(store, meta, lang, style)
	if err != nil {
		return protocol.RunResult{}, err
	}
	meta.State = protocol.SessionStateRunning
	meta.PermissionMode = protocol.PermissionModeAuto
	meta.UpdatedAt = time.Now().UTC()
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	return a.runExecution(ctx, store, sink, sessionID, meta, *planResult, newExecutionPlan(planResult.Steps), lang, style, false)
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
	if meta.ActiveCheckpointID == "" || meta.PendingInterruptID == "" {
		return protocol.RunResult{}, fmt.Errorf("session has no pending approval")
	}
	if !approved {
		meta.State = protocol.SessionStatePlanned
		meta.ApprovalPending = false
		meta.ActiveCheckpointID = ""
		meta.PendingInterruptID = ""
		meta.UpdatedAt = time.Now().UTC()
		if err := store.SaveMeta(meta); err != nil {
			return protocol.RunResult{}, err
		}
		snapshot, err := store.Snapshot(sessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return protocol.RunResult{Session: snapshot, Plan: planResult}, nil
	}

	runner, err := a.buildExecutionRunner(ctx, store, sessionID, prependApprovalStep(newExecutionPlan(planResult.Steps), planResult.PlanID, approvalSummary(*planResult)), meta.LastTask, meta.Language, meta.Style, true)
	if err != nil {
		return protocol.RunResult{}, err
	}

	meta.State = protocol.SessionStateRunning
	meta.ApprovalPending = false
	meta.UpdatedAt = time.Now().UTC()
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}

	iter, err := runner.ResumeWithParams(ctx, meta.ActiveCheckpointID, &adk.ResumeParams{
		Targets: map[string]any{
			meta.PendingInterruptID: "approved",
		},
	})
	if err != nil {
		return protocol.RunResult{}, err
	}
	return a.finalizeExecution(ctx, store, sink, sessionID, meta, *planResult, iter)
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

func (a *Agent) planSession(ctx context.Context, store *storage.Store, sink EventSink, sessionID, goal string, approvalRequired bool) (protocol.PlanResult, *executionPlan, []protocol.PaperRef, error) {
	if err := store.InvalidatePlanState(sessionID); err != nil {
		return protocol.PlanResult{}, nil, nil, err
	}
	refs, err := a.tools.InspectSources(ctx, store, sessionID, nil)
	if err != nil {
		return protocol.PlanResult{}, nil, nil, err
	}
	if len(refs) == 0 {
		return protocol.PlanResult{}, nil, nil, fmt.Errorf("no sources attached")
	}
	if err := a.emit(store, sink, sessionID, protocol.EventAnalysis, "source inspection complete", refs); err != nil {
		return protocol.PlanResult{}, nil, nil, err
	}

	model, err := providers.BuildChatModel(ctx, a.cfg.Provider, a.cfg.ProviderTimeout())
	if err != nil {
		return protocol.PlanResult{}, nil, nil, err
	}
	planner, err := planexecute.NewPlanner(ctx, &planexecute.PlannerConfig{
		ToolCallingChatModel: model,
		ToolInfo:             executionPlanToolInfo(),
		NewPlan: func(context.Context) planexecute.Plan {
			return &executionPlan{}
		},
		GenInputFn: func(ctx context.Context, _ []adk.Message) ([]adk.Message, error) {
			return plannerInputMessages(goal, refs)
		},
	})
	if err != nil {
		return protocol.PlanResult{}, nil, nil, err
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: planner})
	iter := runner.Run(ctx, []adk.Message{schema.UserMessage(goal)})

	execPlan, err := parsePlanFromIterator(iter)
	if err != nil {
		return protocol.PlanResult{}, nil, nil, err
	}
	planResult := toProtocolPlan(goal, refs, execPlan, approvalRequired)
	if err := store.SavePlan(sessionID, planResult); err != nil {
		return protocol.PlanResult{}, nil, nil, err
	}
	if err := a.emit(store, sink, sessionID, protocol.EventPlan, "plan ready", planResult); err != nil {
		return protocol.PlanResult{}, nil, nil, err
	}
	return planResult, execPlan, refs, nil
}

func (a *Agent) startConfirmExecution(ctx context.Context, store *storage.Store, sink EventSink, sessionID string, meta protocol.SessionMeta, planResult protocol.PlanResult, execPlan *executionPlan, lang, style string) (protocol.RunResult, error) {
	approvalPlan := prependApprovalStep(execPlan, planResult.PlanID, approvalSummary(planResult))
	checkpointID := fmt.Sprintf("%s_confirm_%d", sessionID, time.Now().UnixNano())

	runner, err := a.buildExecutionRunner(ctx, store, sessionID, approvalPlan, meta.LastTask, lang, style, true)
	if err != nil {
		return protocol.RunResult{}, err
	}
	iter := runner.Run(ctx, []adk.Message{schema.UserMessage(meta.LastTask)}, adk.WithCheckPointID(checkpointID))
	approval, err := a.consumeExecutionEvents(store, sink, sessionID, planResult.PlanID, checkpointID, iter)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if approval == nil {
		return a.finalizeExecution(ctx, store, sink, sessionID, meta, planResult, nil)
	}

	meta.State = protocol.SessionStateAwaitingApproval
	meta.ApprovalPending = true
	meta.ActiveCheckpointID = approval.CheckpointID
	meta.PendingInterruptID = approval.InterruptID
	meta.UpdatedAt = time.Now().UTC()
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	snapshot, err := store.Snapshot(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	return protocol.RunResult{
		Session:  snapshot,
		Plan:     &planResult,
		Approval: approval,
	}, nil
}

func (a *Agent) runExecution(ctx context.Context, store *storage.Store, sink EventSink, sessionID string, meta protocol.SessionMeta, planResult protocol.PlanResult, execPlan *executionPlan, lang, style string, includeApproval bool) (protocol.RunResult, error) {
	runner, err := a.buildExecutionRunner(ctx, store, sessionID, execPlan, meta.LastTask, lang, style, includeApproval)
	if err != nil {
		return protocol.RunResult{}, err
	}
	iter := runner.Run(ctx, []adk.Message{schema.UserMessage(meta.LastTask)})
	return a.finalizeExecution(ctx, store, sink, sessionID, meta, planResult, iter)
}

func (a *Agent) finalizeExecution(ctx context.Context, store *storage.Store, sink EventSink, sessionID string, meta protocol.SessionMeta, planResult protocol.PlanResult, iter *adk.AsyncIterator[*adk.AgentEvent]) (protocol.RunResult, error) {
	if iter != nil {
		approval, err := a.consumeExecutionEvents(store, sink, sessionID, planResult.PlanID, meta.ActiveCheckpointID, iter)
		if err != nil {
			meta.State = protocol.SessionStateFailed
			meta.UpdatedAt = time.Now().UTC()
			_ = store.SaveMeta(meta)
			return protocol.RunResult{}, err
		}
		if approval != nil {
			meta.State = protocol.SessionStateAwaitingApproval
			meta.ApprovalPending = true
			meta.ActiveCheckpointID = approval.CheckpointID
			meta.PendingInterruptID = approval.InterruptID
			meta.UpdatedAt = time.Now().UTC()
			if err := store.SaveMeta(meta); err != nil {
				return protocol.RunResult{}, err
			}
			snapshot, err := store.Snapshot(sessionID)
			if err != nil {
				return protocol.RunResult{}, err
			}
			return protocol.RunResult{Session: snapshot, Plan: &planResult, Approval: approval}, nil
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

func (a *Agent) buildExecutionRunner(ctx context.Context, store *storage.Store, sessionID string, execPlan *executionPlan, goal, lang, style string, includeApproval bool) (*adk.Runner, error) {
	model, err := providers.BuildChatModel(ctx, a.cfg.Provider, a.cfg.ProviderTimeout())
	if err != nil {
		return nil, err
	}
	toolset, err := a.tools.BuildExecutionTools(ctx, store, sessionID, includeApproval)
	if err != nil {
		return nil, err
	}
	executor, err := planexecute.NewExecutor(ctx, &planexecute.ExecutorConfig{
		Model: model,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toolset,
			},
		},
		MaxIterations: 8,
		GenInputFn: func(ctx context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			return executorInputMessages(goal, lang, style, in)
		},
	})
	if err != nil {
		return nil, err
	}

	replanner, err := planexecute.NewReplanner(ctx, &planexecute.ReplannerConfig{
		ChatModel: model,
		PlanTool:  executionPlanToolInfo(),
		NewPlan: func(context.Context) planexecute.Plan {
			return &executionPlan{}
		},
		GenInputFn: func(ctx context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			digestIDs, hasComparison, assetsErr := currentAssets(store, sessionID)
			if assetsErr != nil {
				return nil, assetsErr
			}
			basePlan, _ := in.Plan.(*executionPlan)
			storedPlan, loadErr := store.LoadPlan(sessionID)
			if loadErr != nil {
				return nil, loadErr
			}
			if storedPlan != nil {
				basePlan = newExecutionPlan(storedPlan.Steps)
			}
			return replannerInputMessages(goal, basePlan, digestIDs, hasComparison, len(in.ExecutedSteps), lastExecutedStepID(in.ExecutedSteps), executedStepIDs(in.ExecutedSteps))
		},
	})
	if err != nil {
		return nil, err
	}

	agent, err := planexecute.New(ctx, &planexecute.Config{
		Planner:       &savedPlanPlannerAgent{plan: execPlan},
		Executor:      executor,
		Replanner:     replanner,
		MaxIterations: 16,
	})
	if err != nil {
		return nil, err
	}

	return adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: store.CheckPointStore(sessionID),
	}), nil
}

func currentAssets(store *storage.Store, sessionID string) ([]string, bool, error) {
	digests, err := store.LoadDigests(sessionID)
	if err != nil {
		return nil, false, err
	}
	out := make([]string, 0, len(digests))
	for _, digest := range digests {
		out = append(out, digest.PaperID)
	}
	cmp, err := store.LoadComparison(sessionID)
	if err != nil {
		return nil, false, err
	}
	return out, cmp != nil, nil
}

func parsePlanFromIterator(iter *adk.AsyncIterator[*adk.AgentEvent]) (*executionPlan, error) {
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return nil, event.Err
		}
		msg, _, err := adk.GetMessage(event)
		if err != nil {
			continue
		}
		var plan executionPlan
		if err := json.Unmarshal([]byte(msg.Content), &plan); err == nil {
			return &plan, nil
		}
	}
	return nil, fmt.Errorf("planner did not produce a valid plan")
}

func (a *Agent) consumeExecutionEvents(store *storage.Store, sink EventSink, sessionID, planID, checkpointID string, iter *adk.AsyncIterator[*adk.AgentEvent]) (*protocol.ApprovalRequest, error) {
	var (
		currentTool string
		stepIndex   int
	)
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return nil, event.Err
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			approval := approvalFromInterrupt(planID, checkpointID, event)
			if err := a.emit(store, sink, sessionID, protocol.EventApprovalRequired, "approval required", approval); err != nil {
				return nil, err
			}
			return approval, nil
		}
		msg, _, err := adk.GetMessage(event)
		if err != nil {
			continue
		}
		if msg == nil {
			continue
		}
		if len(msg.ToolCalls) > 0 {
			currentTool = msg.ToolCalls[0].Function.Name
			stepIndex++
			progress := protocol.PlanProgress{
				PlanID:    planID,
				StepID:    fmt.Sprintf("run_%02d", stepIndex),
				Tool:      currentTool,
				Status:    protocol.PlanProgressStarted,
				Message:   "tool execution started",
				CreatedAt: time.Now().UTC(),
			}
			if err := a.emit(store, sink, sessionID, protocol.EventProgress, "step started", progress); err != nil {
				return nil, err
			}
			continue
		}
		if msg.Role == schema.Assistant && strings.HasPrefix(strings.ToLower(strings.TrimSpace(msg.Content)), "step completed") {
			progress := protocol.PlanProgress{
				PlanID:    planID,
				StepID:    fmt.Sprintf("run_%02d", stepIndex),
				Tool:      currentTool,
				Status:    protocol.PlanProgressCompleted,
				Message:   msg.Content,
				CreatedAt: time.Now().UTC(),
			}
			if err := a.emit(store, sink, sessionID, protocol.EventProgress, "step completed", progress); err != nil {
				return nil, err
			}
		}
	}
	return nil, nil
}

func approvalFromInterrupt(planID, checkpointID string, event *adk.AgentEvent) *protocol.ApprovalRequest {
	request := &protocol.ApprovalRequest{
		PlanID:        planID,
		CheckpointID:  checkpointID,
		Summary:       "approval required",
		RequiresInput: true,
		CreatedAt:     time.Now().UTC(),
	}
	if event == nil || event.Action == nil || event.Action.Interrupted == nil {
		return request
	}
	for _, interrupt := range event.Action.Interrupted.InterruptContexts {
		if request.InterruptID == "" || interrupt.IsRootCause {
			request.InterruptID = interrupt.ID
		}
		if info, ok := interrupt.Info.(map[string]string); ok {
			if summary := strings.TrimSpace(info["summary"]); summary != "" {
				request.Summary = summary
			}
		}
		if info, ok := interrupt.Info.(map[string]any); ok {
			if summary, ok := info["summary"].(string); ok && strings.TrimSpace(summary) != "" {
				request.Summary = summary
			}
		}
	}
	return request
}

func approvalSummary(plan protocol.PlanResult) string {
	parts := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		parts = append(parts, fmt.Sprintf("%s:%s", step.Tool, step.Goal))
	}
	return strings.Join(parts, " | ")
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
