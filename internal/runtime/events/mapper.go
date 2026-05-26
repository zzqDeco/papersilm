package events

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func FromAgentEvent(sessionID, turnID string, event *adk.TypedAgentEvent[*schema.AgenticMessage]) []protocol.StreamEvent {
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
		base.Type = protocol.EventApprovalRequired
		base.Message = "permission required"
		base.Payload = event.Action.Interrupted
		return []protocol.StreamEvent{base}
	}
	if event.Output == nil || event.Output.MessageOutput == nil {
		return nil
	}
	msg := event.Output.MessageOutput.Message
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
