package events

import (
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestFromAgentEventMapsInterruptToProtocolApproval(t *testing.T) {
	t.Parallel()

	event := &adk.TypedAgentEvent[*schema.AgenticMessage]{
		Action: &adk.AgentAction{
			Interrupted: &adk.InterruptInfo{
				InterruptContexts: []*adk.InterruptCtx{{
					ID: "interrupt_1",
					Info: protocol.PermissionRequest{
						RequestID: "req_1",
						NodeID:    "node_1",
						Tool:      string(protocol.NodeKindWorkspaceEdit),
						Title:     "Edit file",
						Question:  "Allow edit?",
					},
				}},
			},
		},
	}

	mapped := FromAgentEvent("session_1", "turn_1", event)
	if len(mapped) != 1 || mapped[0].Type != protocol.EventApprovalRequired {
		t.Fatalf("expected one approval event, got %+v", mapped)
	}
	approval, ok := mapped[0].Payload.(protocol.ApprovalRequest)
	if !ok {
		t.Fatalf("expected protocol approval payload, got %T", mapped[0].Payload)
	}
	if approval.ActiveRequestID != "req_1" || approval.InterruptID != "interrupt_1" || len(approval.Requests) != 1 {
		t.Fatalf("unexpected approval payload: %+v", approval)
	}
	if approval.Requests[0].InterruptID != "interrupt_1" {
		t.Fatalf("expected request interrupt target to be preserved, got %+v", approval.Requests[0])
	}
}
