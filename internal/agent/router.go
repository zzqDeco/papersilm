package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"papersilm/internal/storage"
	"papersilm/internal/tools"
	"papersilm/pkg/protocol"
)

type EventSink interface {
	Emit(event protocol.StreamEvent) error
}

type Agent struct {
	tools *tools.Registry
}

func New(registry *tools.Registry) *Agent {
	return &Agent{tools: registry}
}

func (a *Agent) Execute(ctx context.Context, store *storage.Store, sink EventSink, req protocol.ClientRequest) (protocol.RunResult, error) {
	meta, err := store.LoadMeta(req.SessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if len(req.Sources) > 0 {
		refs, err := a.tools.AttachSources(ctx, store, req.SessionID, req.Sources)
		if err != nil {
			return protocol.RunResult{}, err
		}
		meta.State = protocol.SessionStateSourceAttached
		meta.UpdatedAt = time.Now().UTC()
		if err := store.SaveMeta(meta); err != nil {
			return protocol.RunResult{}, err
		}
		if sink != nil {
			_ = sink.Emit(protocol.StreamEvent{
				Type:      protocol.EventSourceAttached,
				SessionID: req.SessionID,
				Message:   "sources attached",
				Payload:   refs,
				CreatedAt: time.Now().UTC(),
			})
		}
	}

	sources, err := store.LoadSources(req.SessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if len(sources) == 0 {
		return protocol.RunResult{}, fmt.Errorf("no sources attached")
	}

	plan, err := a.tools.InspectAndPlan(ctx, store, req.SessionID, req.Task, sources)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if err := store.SavePlan(req.SessionID, plan); err != nil {
		return protocol.RunResult{}, err
	}
	meta.State = protocol.SessionStatePlanned
	meta.LastTask = req.Task
	meta.UpdatedAt = time.Now().UTC()
	if req.PermissionMode == protocol.PermissionModeConfirm || req.PermissionMode == protocol.PermissionModePlan {
		meta.ApprovalPending = true
		meta.State = protocol.SessionStateAwaitingApproval
	}
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	if sink != nil {
		_ = sink.Emit(protocol.StreamEvent{
			Type:      protocol.EventPlan,
			SessionID: req.SessionID,
			Message:   "plan ready",
			Payload:   plan,
			CreatedAt: time.Now().UTC(),
		})
	}

	if req.PermissionMode == protocol.PermissionModePlan {
		snapshot, err := store.Snapshot(req.SessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return protocol.RunResult{Session: snapshot, Plan: &plan}, nil
	}
	if req.PermissionMode == protocol.PermissionModeConfirm && !taskRequestsApprovalBypass(req.Task) {
		if sink != nil {
			_ = sink.Emit(protocol.StreamEvent{
				Type:      protocol.EventApprovalRequired,
				SessionID: req.SessionID,
				Message:   "approval required",
				Payload:   plan,
				CreatedAt: time.Now().UTC(),
			})
		}
		snapshot, err := store.Snapshot(req.SessionID)
		if err != nil {
			return protocol.RunResult{}, err
		}
		return protocol.RunResult{Session: snapshot, Plan: &plan}, nil
	}

	return a.runApproved(ctx, store, sink, req.SessionID, req.Language, req.Style)
}

func (a *Agent) RunApproved(ctx context.Context, store *storage.Store, sink EventSink, sessionID string, lang, style string) (protocol.RunResult, error) {
	return a.runApproved(ctx, store, sink, sessionID, lang, style)
}

func (a *Agent) runApproved(ctx context.Context, store *storage.Store, sink EventSink, sessionID string, lang, style string) (protocol.RunResult, error) {
	meta, err := store.LoadMeta(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	sources, err := store.LoadSources(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	meta.State = protocol.SessionStateRunning
	meta.ApprovalPending = false
	meta.UpdatedAt = time.Now().UTC()
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	digests, comparison, artifacts, err := a.tools.Run(ctx, store, sessionID, sources, lang, style)
	if err != nil {
		meta.State = protocol.SessionStateFailed
		meta.UpdatedAt = time.Now().UTC()
		_ = store.SaveMeta(meta)
		return protocol.RunResult{}, err
	}
	meta.State = protocol.SessionStateCompleted
	meta.UpdatedAt = time.Now().UTC()
	if err := store.SaveMeta(meta); err != nil {
		return protocol.RunResult{}, err
	}
	snapshot, err := store.Snapshot(sessionID)
	if err != nil {
		return protocol.RunResult{}, err
	}
	if sink != nil {
		_ = sink.Emit(protocol.StreamEvent{
			Type:      protocol.EventResult,
			SessionID: sessionID,
			Message:   "run completed",
			Payload: map[string]interface{}{
				"digests":    len(digests),
				"comparison": comparison != nil,
				"artifacts":  len(artifacts),
			},
			CreatedAt: time.Now().UTC(),
		})
	}
	return protocol.RunResult{
		Session:    snapshot,
		Plan:       snapshot.Plan,
		Digests:    digests,
		Comparison: comparison,
		Artifacts:  artifacts,
	}, nil
}

func taskRequestsApprovalBypass(task string) bool {
	normalized := strings.ToLower(task)
	return strings.Contains(normalized, "/approve") || strings.Contains(normalized, "approve")
}

