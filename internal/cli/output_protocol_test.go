package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestOutputWriterJSONKeepsProtocolApprovalShape(t *testing.T) {
	t.Parallel()

	approval := protocol.ApprovalRequest{
		CheckpointID:    "checkpoint_1",
		InterruptID:     "interrupt_1",
		ActiveRequestID: "req_1",
		Mode:            "tool",
		Requests: []protocol.PermissionRequest{{
			RequestID:   "req_1",
			InterruptID: "interrupt_1",
			Tool:        string(protocol.NodeKindWorkspaceCommand),
			Operation:   "shell",
			Title:       "Run command",
			Question:    "Do you want to run this command?",
			Command:     "printf %s ok",
			Options: []protocol.PermissionOption{
				{Value: "accept-once", Label: "Yes", Scope: "node"},
				{Value: "reject", Label: "No", Scope: "node"},
			},
			CreatedAt: time.Unix(1, 0).UTC(),
		}},
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	result := protocol.RunResult{
		Approval: &approval,
		Session: protocol.SessionSnapshot{
			Meta: protocol.SessionMeta{SessionID: "sess_json", State: protocol.SessionStateAwaitingApproval},
		},
	}
	var out bytes.Buffer
	if err := NewOutputWriter(&out, protocol.OutputFormatJSON).PrintResult(result); err != nil {
		t.Fatalf("PrintResult(json): %v", err)
	}
	raw := out.String()
	for _, forbidden := range []string{"InterruptInfo", "InterruptCtx", "AgenticMessage", "TypedAgentEvent"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("json output leaked Eino internal type %q: %s", forbidden, raw)
		}
	}
	var decoded protocol.RunResult
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("Unmarshal(json result): %v\n%s", err, raw)
	}
	if decoded.Approval == nil || decoded.Approval.Requests[0].RequestID != "req_1" || decoded.Approval.Requests[0].Command != "printf %s ok" {
		t.Fatalf("approval payload lost protocol shape: %+v", decoded.Approval)
	}
}

func TestOutputWriterJSONPreservesReplacePermissionContract(t *testing.T) {
	t.Parallel()

	approval := protocol.ApprovalRequest{
		CheckpointID:    "checkpoint_replace",
		InterruptID:     "interrupt_replace",
		ActiveRequestID: "replace_req",
		Mode:            "tool",
		Requests: []protocol.PermissionRequest{{
			RequestID:  "replace_req",
			Tool:       string(protocol.NodeKindWorkspaceEdit),
			Operation:  "replace",
			Title:      "Edit file",
			TargetPath: "README.md",
			Question:   "Do you want to make this edit to README.md?",
			Preview: protocol.PermissionPreview{
				Kind:           "diff",
				Diff:           "--- README.md\n+++ README.md\n-typo\n+type",
				OldContentHash: "hash_before",
				OldText:        "typo",
				NewText:        "type",
			},
			Options: []protocol.PermissionOption{
				{Value: "accept-once", Label: "Yes", Scope: "node"},
				{Value: "accept-session", Label: "Yes, during this session", Scope: "path"},
				{Value: "reject", Label: "No", Scope: "node"},
			},
			CreatedAt: time.Unix(1, 0).UTC(),
		}},
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	result := protocol.RunResult{
		Approval: &approval,
		Session: protocol.SessionSnapshot{
			Meta: protocol.SessionMeta{SessionID: "sess_replace", State: protocol.SessionStateAwaitingApproval},
		},
	}
	var out bytes.Buffer
	if err := NewOutputWriter(&out, protocol.OutputFormatJSON).PrintResult(result); err != nil {
		t.Fatalf("PrintResult(json): %v", err)
	}
	raw := out.String()
	for _, want := range []string{
		`"operation": "replace"`,
		`"target_path": "README.md"`,
		`"old_content_hash": "hash_before"`,
		`"old_text": "typo"`,
		`"new_text": "type"`,
		`"scope": "path"`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("json output missing %s: %s", want, raw)
		}
	}
	if strings.Contains(raw, `"operation": "write"`) {
		t.Fatalf("replace permission must not serialize as write: %s", raw)
	}
}

func TestOutputWriterStreamJSONEmitsProtocolEventsOnly(t *testing.T) {
	t.Parallel()

	event := protocol.StreamEvent{
		Type:      protocol.EventApprovalRequired,
		SessionID: "sess_stream",
		Message:   "permission required",
		Payload: protocol.ApprovalRequest{
			CheckpointID:    "checkpoint_1",
			InterruptID:     "interrupt_1",
			ActiveRequestID: "req_1",
			Mode:            "tool",
			Requests: []protocol.PermissionRequest{{
				RequestID: "req_1",
				Tool:      string(protocol.NodeKindWorkspaceEdit),
				Operation: "write",
			}},
		},
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	var out bytes.Buffer
	if err := NewOutputWriter(&out, protocol.OutputFormatStreamJSON).Emit(event); err != nil {
		t.Fatalf("Emit(stream-json): %v", err)
	}
	line := strings.TrimSpace(out.String())
	if line == "" {
		t.Fatalf("expected stream-json line")
	}
	for _, forbidden := range []string{"InterruptInfo", "InterruptCtx", "AgenticMessage", "TypedAgentEvent"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("stream-json leaked Eino internal type %q: %s", forbidden, line)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("Unmarshal(stream event): %v\n%s", err, line)
	}
	if decoded["type"] != string(protocol.EventApprovalRequired) {
		t.Fatalf("unexpected stream event type: %+v", decoded)
	}
	payload, ok := decoded["payload"].(map[string]any)
	if !ok || payload["active_request_id"] != "req_1" {
		t.Fatalf("approval payload missing request id: %+v", decoded["payload"])
	}
}

func TestOutputWriterStreamJSONPreservesOpaquePayloadFields(t *testing.T) {
	t.Parallel()

	event := protocol.StreamEvent{
		Type:      protocol.EventResult,
		SessionID: "sess_payload",
		Message:   "result",
		Payload: map[string]any{
			"optional_status": "failed",
			"optional_code":   7,
			"optional_flag":   false,
		},
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	var out bytes.Buffer
	if err := NewOutputWriter(&out, protocol.OutputFormatStreamJSON).Emit(event); err != nil {
		t.Fatalf("Emit(stream-json): %v", err)
	}
	line := strings.TrimSpace(out.String())
	for _, forbidden := range []string{"InterruptInfo", "InterruptCtx", "AgenticMessage", "TypedAgentEvent", "Permission decision:"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("stream-json leaked internal text %q: %s", forbidden, line)
		}
	}
	var decoded struct {
		Type    protocol.StreamEventType `json:"type"`
		Payload map[string]any           `json:"payload"`
	}
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("Unmarshal(stream event): %v\n%s", err, line)
	}
	if decoded.Type != protocol.EventResult {
		t.Fatalf("unexpected stream event type: %+v", decoded)
	}
	for key, want := range map[string]any{
		"optional_status": "failed",
		"optional_code":   float64(7),
		"optional_flag":   false,
	} {
		if got := decoded.Payload[key]; got != want {
			t.Fatalf("payload[%s] = %#v, want %#v; payload=%+v", key, got, want, decoded.Payload)
		}
	}
}

func TestOutputWriterStreamJSONPreservesWorkspaceToolProjectionPayload(t *testing.T) {
	t.Parallel()

	event := protocol.StreamEvent{
		Type:      protocol.EventProgress,
		SessionID: "sess_workspace_tool",
		Message:   "Command exited 7",
		Payload: map[string]any{
			"subtype":            "workspace_tool",
			"tool":               "workspace_run_command",
			"tool_call_id":       "call_cmd",
			"tool_result_status": "failed",
			"command":            "printf stdout; printf stderr >&2; exit 7",
			"cwd":                "/tmp/workspace",
			"exit_code":          7,
			"stdout_truncated":   false,
			"stderr_truncated":   true,
		},
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	var out bytes.Buffer
	if err := NewOutputWriter(&out, protocol.OutputFormatStreamJSON).Emit(event); err != nil {
		t.Fatalf("Emit(stream-json): %v", err)
	}
	line := strings.TrimSpace(out.String())
	for _, forbidden := range []string{"InterruptInfo", "TypedAgentEvent", `"stdout":`, `"stderr":`} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("stream-json leaked forbidden text %q: %s", forbidden, line)
		}
	}
	var decoded struct {
		Type    protocol.StreamEventType `json:"type"`
		Message string                   `json:"message"`
		Payload map[string]any           `json:"payload"`
	}
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("Unmarshal(stream event): %v\n%s", err, line)
	}
	if decoded.Type != protocol.EventProgress || decoded.Message != "Command exited 7" {
		t.Fatalf("unexpected stream event: %+v", decoded)
	}
	for key, want := range map[string]any{
		"subtype":            "workspace_tool",
		"tool":               "workspace_run_command",
		"tool_call_id":       "call_cmd",
		"tool_result_status": "failed",
		"cwd":                "/tmp/workspace",
		"exit_code":          float64(7),
		"stderr_truncated":   true,
	} {
		if got := decoded.Payload[key]; got != want {
			t.Fatalf("payload[%s] = %#v, want %#v; payload=%+v", key, got, want, decoded.Payload)
		}
	}
}
