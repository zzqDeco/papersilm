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
	if projected := workspaceToolResultEvents(base, turnID, event.AgentName, msg); len(projected) > 0 {
		if text := agenticText(msg, true); strings.TrimSpace(text) != "" {
			assistant := base
			assistant.Type = protocol.EventAssistant
			assistant.Message = text
			assistant.Payload = map[string]any{
				"turn_id": turnID,
				"agent":   event.AgentName,
			}
			projected = append(projected, assistant)
		}
		return projected
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
	return agenticText(msg, false)
}

func agenticText(msg *schema.AgenticMessage, skipWorkspaceToolResults bool) string {
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
			if skipWorkspaceToolResults && isWorkspaceTool(block.FunctionToolResult.Name) {
				continue
			}
			for _, content := range block.FunctionToolResult.Content {
				if content != nil && content.Text != nil {
					parts = append(parts, content.Text.Text)
				}
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func workspaceToolResultEvents(base protocol.StreamEvent, turnID, agent string, msg *schema.AgenticMessage) []protocol.StreamEvent {
	if msg == nil {
		return nil
	}
	out := make([]protocol.StreamEvent, 0)
	for _, block := range msg.ContentBlocks {
		if block == nil || block.FunctionToolResult == nil || !isWorkspaceTool(block.FunctionToolResult.Name) {
			continue
		}
		result := block.FunctionToolResult
		summary, payload := workspaceToolProjection(result.Name, result.CallID, functionToolResultText(result))
		if summary == "" {
			continue
		}
		payload["turn_id"] = turnID
		payload["agent"] = agent
		event := base
		event.Type = protocol.EventProgress
		event.Message = summary
		event.Payload = payload
		out = append(out, event)
	}
	return out
}

func isWorkspaceTool(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), "workspace_")
}

func functionToolResultText(result *schema.FunctionToolResult) string {
	if result == nil {
		return ""
	}
	parts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		if content != nil && content.Text != nil {
			parts = append(parts, content.Text.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func workspaceToolProjection(toolName, callID, text string) (string, map[string]any) {
	payload := map[string]any{
		"subtype":      "workspace_tool",
		"tool":         toolName,
		"tool_call_id": callID,
	}
	raw := decodeToolResultJSON(text)
	if rawMap, ok := raw.(map[string]any); ok {
		copyProjectionFields(payload, rawMap)
	}
	if resultSummary := strings.TrimSpace(asString(payload["summary"])); resultSummary != "" {
		payload["result_summary"] = resultSummary
	}

	var summary string
	switch toolName {
	case "workspace_search":
		count := lenJSONArray(raw)
		payload["match_count"] = count
		if count == 0 {
			summary = "Search found no matches"
		} else {
			summary = fmt.Sprintf("Search found %d matches", count)
		}
	case "workspace_list_files":
		count := lenJSONArray(raw)
		payload["match_count"] = count
		if count == 1 {
			summary = "Listed 1 file"
		} else {
			summary = fmt.Sprintf("Listed %d files", count)
		}
	case "workspace_read_file":
		target := projectionPath(payload, raw)
		if target == "" {
			summary = "Read file"
		} else {
			payload["target_path"] = target
			summary = "Read " + target
		}
	case "workspace_replace_text":
		summary = editProjectionSummary(payload, "Edited")
	case "workspace_write_file":
		summary = editProjectionSummary(payload, "Wrote")
	case "workspace_run_command":
		summary = commandProjectionSummary(payload)
	default:
		summary = strings.TrimSpace(asString(payload["summary"]))
	}
	if summary == "" {
		summary = strings.TrimSpace(text)
	}
	payload["summary"] = summary
	return summary, payload
}

func decodeToolResultJSON(text string) any {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return nil
	}
	return value
}

func copyProjectionFields(payload map[string]any, values map[string]any) {
	for _, key := range []string{
		"status",
		"tool_result_status",
		"summary",
		"target_path",
		"command",
		"cwd",
		"exit_code",
		"stdout_truncated",
		"stderr_truncated",
		"changed",
		"conflict",
	} {
		if value, ok := values[key]; ok {
			payload[key] = value
		}
	}
	if _, ok := payload["tool_result_status"]; !ok {
		if status, ok := values["status"]; ok {
			payload["tool_result_status"] = status
		}
	}
}

func lenJSONArray(value any) int {
	if items, ok := value.([]any); ok {
		return len(items)
	}
	return 0
}

func projectionPath(payload map[string]any, raw any) string {
	if path := asString(payload["target_path"]); path != "" {
		return path
	}
	if summary := asString(payload["summary"]); summary != "" {
		return summary
	}
	if rawMap, ok := raw.(map[string]any); ok {
		return asString(rawMap["path"])
	}
	return ""
}

func editProjectionSummary(payload map[string]any, verb string) string {
	if isRejected(payload) {
		return "Tool use rejected"
	}
	target := projectionTarget(payload)
	if isConflict(payload) {
		if target == "" {
			return "Edit conflict"
		}
		return "Edit conflict in " + target
	}
	if target == "" {
		return verb + " file"
	}
	return verb + " " + target
}

func commandProjectionSummary(payload map[string]any) string {
	if isRejected(payload) {
		return "Tool use rejected"
	}
	if exitCode, ok := intValue(payload["exit_code"]); ok && exitCode != 0 {
		return fmt.Sprintf("Command exited %d", exitCode)
	}
	if strings.EqualFold(asString(payload["tool_result_status"]), "failed") {
		return "Command failed"
	}
	return "Command completed"
}

func projectionTarget(payload map[string]any) string {
	if target := asString(payload["target_path"]); target != "" {
		return target
	}
	if summary := asString(payload["summary"]); summary != "" {
		return summary
	}
	return ""
}

func isRejected(payload map[string]any) bool {
	return strings.EqualFold(asString(payload["tool_result_status"]), "rejected") ||
		strings.EqualFold(asString(payload["status"]), "rejected")
}

func isConflict(payload map[string]any) bool {
	if value, ok := payload["conflict"].(bool); ok && value {
		return true
	}
	return strings.EqualFold(asString(payload["tool_result_status"]), "conflict") ||
		strings.EqualFold(asString(payload["status"]), "conflict")
}

func asString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func intValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}
