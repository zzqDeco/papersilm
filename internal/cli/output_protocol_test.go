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
