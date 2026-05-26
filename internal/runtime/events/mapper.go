package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func FromAgentEvent(sessionID, turnID, checkpointID string, event *adk.TypedAgentEvent[*schema.AgenticMessage]) []protocol.StreamEvent {
	if event == nil {
		return nil
	}
	now := time.Now().UTC()
	base := protocol.StreamEvent{
		SessionID: sessionID,
		CreatedAt: now,
	}
	if event.Err != nil {
		base.Type = protocol.EventError
		base.Message = event.Err.Error()
		base.Payload = map[string]any{
			"turn_id": turnID,
			"agent":   event.AgentName,
		}
		return []protocol.StreamEvent{base}
	}
	if event.Action != nil && event.Action.Interrupted != nil {
		approval := approvalFromInterrupt(sessionID, turnID, checkpointID, event.Action.Interrupted)
		base.Type = protocol.EventApprovalRequired
		base.Message = "permission required"
		base.Payload = approval
		return []protocol.StreamEvent{base}
	}
	if event.Output == nil || event.Output.MessageOutput == nil {
		return nil
	}
	msg, err := event.Output.MessageOutput.GetMessage()
	if err != nil {
		base.Type = protocol.EventError
		base.Message = err.Error()
		base.Payload = map[string]any{
			"turn_id": turnID,
			"agent":   event.AgentName,
		}
		return []protocol.StreamEvent{base}
	}
	if msg == nil {
		return nil
	}
	text := AgenticText(msg)
	if strings.TrimSpace(text) == "" {
		if raw, err := json.Marshal(msg); err == nil {
			text = string(raw)
		}
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	base.Type = protocol.EventAssistant
	base.Message = text
	base.Payload = map[string]any{
		"turn_id": turnID,
		"agent":   event.AgentName,
	}
	return []protocol.StreamEvent{base}
}

func approvalFromInterrupt(sessionID, turnID, checkpointID string, info *adk.InterruptInfo) protocol.ApprovalRequest {
	if strings.TrimSpace(checkpointID) == "" {
		checkpointID = "turn_" + turnID
	}
	approval := protocol.ApprovalRequest{
		CheckpointID:  checkpointID,
		Summary:       "Permission required",
		RequiresInput: true,
		CreatedAt:     time.Now().UTC(),
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
				Tool:      "tool",
				Title:     "Tool permission",
				Question:  "Do you want to allow this tool use?",
				Summary:   fmt.Sprint(ctx.Info),
				Options: []protocol.PermissionOption{
					{Value: "accept-once", Label: "Yes", Scope: "node"},
					{Value: "accept-session", Label: "Yes, during this session", Scope: "session"},
					{Value: "reject", Label: "No", Scope: "node"},
				},
				CreatedAt: time.Now().UTC(),
			}
		}
		if request.RequestID == "" {
			request.RequestID = ctx.ID
		}
		request.InterruptID = ctx.ID
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

func AgenticText(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}
	parts := make([]string, 0, len(msg.ContentBlocks))
	for _, block := range msg.ContentBlocks {
		if block == nil {
			continue
		}
		switch {
		case block.AssistantGenText != nil:
			parts = append(parts, block.AssistantGenText.Text)
		case block.UserInputText != nil:
			parts = append(parts, block.UserInputText.Text)
		case block.FunctionToolCall != nil:
			parts = append(parts, "tool call: "+block.FunctionToolCall.Name)
		case block.FunctionToolResult != nil:
			for _, content := range block.FunctionToolResult.Content {
				if content != nil && content.Text != nil {
					parts = append(parts, content.Text.Text)
				}
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
