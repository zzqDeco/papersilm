package einoagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/internal/runtime/events"
	"github.com/zzqDeco/papersilm/internal/runtime/prepare"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type RunRequest struct {
	Store       *storage.Store
	Prepared    prepare.PreparedAgent
	SessionID   string
	TurnID      string
	Checkpoint  string
	UserMessage string
}

type ResumeRequest struct {
	Store      *storage.Store
	Prepared   prepare.PreparedAgent
	SessionID  string
	TurnID     string
	Checkpoint string
	Targets    map[string]any
}

type RunOutput struct {
	Events      []protocol.StreamEvent
	Response    string
	Interrupted *protocol.ApprovalRequest
}

func Run(ctx context.Context, req RunRequest) (RunOutput, error) {
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{
		Agent:           req.Prepared.Agent,
		EnableStreaming: true,
		CheckPointStore: req.Store.CheckPointStore(req.SessionID),
	})
	iter := runner.Query(ctx, req.UserMessage, adk.WithCheckPointID(req.Checkpoint))
	return drain(ctx, req.Store, req.SessionID, req.TurnID, req.Checkpoint, iter)
}

func Resume(ctx context.Context, req ResumeRequest) (RunOutput, error) {
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{
		Agent:           req.Prepared.Agent,
		EnableStreaming: true,
		CheckPointStore: req.Store.CheckPointStore(req.SessionID),
	})
	iter, err := runner.ResumeWithParams(ctx, req.Checkpoint, &adk.ResumeParams{Targets: req.Targets})
	if err != nil {
		return RunOutput{}, err
	}
	return drain(ctx, req.Store, req.SessionID, req.TurnID, req.Checkpoint, iter)
}

func drain(ctx context.Context, store *storage.Store, sessionID, turnID, checkpointID string, iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]) (RunOutput, error) {
	var out RunOutput
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		_ = store.AppendAgentEvent(sessionID, map[string]any{
			"turn_id":     turnID,
			"agent":       event.AgentName,
			"has_output":  event.Output != nil,
			"has_action":  event.Action != nil,
			"error":       eventErrorString(event),
			"recorded_at": nowUTC(),
		})
		mapped := events.FromAgentEvent(sessionID, turnID, event)
		out.Events = append(out.Events, mapped...)
		for _, streamEvent := range mapped {
			if streamEvent.Type == protocol.EventAssistant {
				out.Response = appendResponse(out.Response, streamEvent.Message)
			}
		}
		if event.Err != nil {
			return out, event.Err
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			approval := ApprovalFromInterrupt(sessionID, checkpointID, event.Action.Interrupted)
			out.Interrupted = &approval
		}
		_ = ctx
	}
	return out, nil
}

func eventErrorString(event *adk.TypedAgentEvent[*schema.AgenticMessage]) string {
	if event == nil || event.Err == nil {
		return ""
	}
	return event.Err.Error()
}

func ApprovalFromInterrupt(sessionID, checkpointID string, info *adk.InterruptInfo) protocol.ApprovalRequest {
	approval := protocol.ApprovalRequest{
		CheckpointID:  checkpointID,
		Summary:       "Permission required",
		RequiresInput: true,
		CreatedAt:     nowUTC(),
		Mode:          "tool",
	}
	if info == nil {
		return approval
	}
	for _, ctx := range info.InterruptContexts {
		if ctx == nil {
			continue
		}
		request, ok := ctx.Info.(protocol.PermissionRequest)
		if !ok {
			request = protocol.PermissionRequest{
				RequestID: ctx.ID,
				SessionID: sessionID,
				Tool:      "tool",
				Title:     "Tool permission",
				Question:  "Do you want to allow this tool use?",
				Summary:   stringify(ctx.Info),
				Options:   defaultPermissionOptions(),
				CreatedAt: nowUTC(),
			}
		}
		if request.RequestID == "" {
			request.RequestID = ctx.ID
		}
		if request.SessionID == "" {
			request.SessionID = sessionID
		}
		approval.Requests = append(approval.Requests, request)
		approval.PendingNodeIDs = append(approval.PendingNodeIDs, request.NodeID)
		if approval.InterruptID == "" {
			approval.InterruptID = ctx.ID
		}
	}
	if len(approval.Requests) > 0 {
		approval.ActiveRequestID = approval.Requests[0].RequestID
		approval.Summary = approval.Requests[0].Question
	}
	return approval
}

func appendResponse(current, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return current
	}
	if strings.TrimSpace(current) == "" {
		return next
	}
	return current + "\n\n" + next
}

func stringify(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func defaultPermissionOptions() []protocol.PermissionOption {
	return []protocol.PermissionOption{
		{Value: "accept-once", Label: "Yes", Scope: "node", Feedback: "accept"},
		{Value: "accept-session", Label: "Yes, during this session", Scope: "session", Feedback: "accept"},
		{Value: "reject", Label: "No", Scope: "node", Feedback: "reject"},
	}
}
