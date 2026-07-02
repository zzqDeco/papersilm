package events

import (
	"strings"
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

	mapped := FromAgentEvent("session_1", "turn_1", "checkpoint_1", event)
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
	if approval.CheckpointID != "checkpoint_1" {
		t.Fatalf("expected checkpoint from runner, got %q", approval.CheckpointID)
	}
	if approval.Requests[0].InterruptID != "interrupt_1" {
		t.Fatalf("expected request interrupt target to be preserved, got %+v", approval.Requests[0])
	}
}

func TestFromAgentEventMapsStreamingMessageOutput(t *testing.T) {
	t.Parallel()

	event := &adk.TypedAgentEvent[*schema.AgenticMessage]{
		AgentName: "papersilm",
		Output: &adk.TypedAgentOutput[*schema.AgenticMessage]{
			MessageOutput: &adk.TypedMessageVariant[*schema.AgenticMessage]{
				IsStreaming: true,
				MessageStream: schema.StreamReaderFromArray([]*schema.AgenticMessage{
					agenticTextMessage("hello "),
					agenticTextMessage("world"),
				}),
			},
		},
	}

	mapped := FromAgentEvent("session_1", "turn_1", "checkpoint_1", event)
	if len(mapped) != 1 || mapped[0].Type != protocol.EventAssistant {
		t.Fatalf("expected one assistant event, got %+v", mapped)
	}
	if !strings.Contains(mapped[0].Message, "hello") || !strings.Contains(mapped[0].Message, "world") {
		t.Fatalf("expected concatenated streaming text, got %q", mapped[0].Message)
	}
}

func TestFromAgentEventProjectsWorkspaceCommandToolResult(t *testing.T) {
	t.Parallel()

	event := agenticToolResultEvent("workspace_run_command", "call_cmd", `{
		"status":"failed",
		"summary":"command output",
		"command":"printf stdout; printf stderr >&2; exit 7",
		"cwd":"/tmp/workspace",
		"exit_code":7,
		"stdout":"stdout",
		"stderr":"stderr",
		"stdout_truncated":false,
		"stderr_truncated":true
	}`)

	mapped := FromAgentEvent("session_1", "turn_1", "checkpoint_1", event)
	if len(mapped) != 1 || mapped[0].Type != protocol.EventProgress {
		t.Fatalf("expected one progress projection, got %+v", mapped)
	}
	if mapped[0].Message != "Command exited 7" {
		t.Fatalf("unexpected projection message: %q", mapped[0].Message)
	}
	payload, ok := mapped[0].Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", mapped[0].Payload)
	}
	for key, want := range map[string]any{
		"subtype":            "workspace_tool",
		"tool":               "workspace_run_command",
		"tool_call_id":       "call_cmd",
		"tool_result_status": "failed",
		"command":            "printf stdout; printf stderr >&2; exit 7",
		"cwd":                "/tmp/workspace",
		"summary":            "Command exited 7",
		"stderr_truncated":   true,
	} {
		if got := payload[key]; got != want {
			t.Fatalf("payload[%s] = %#v, want %#v; payload=%+v", key, got, want, payload)
		}
	}
	if got, ok := payload["exit_code"].(float64); !ok || got != 7 {
		t.Fatalf("payload exit_code = %#v, want 7", payload["exit_code"])
	}
	if _, leaked := payload["stdout"]; leaked {
		t.Fatalf("projection payload must not include full stdout: %+v", payload)
	}
	if strings.Contains(mapped[0].Message, "stdout") || strings.Contains(mapped[0].Message, "stderr") {
		t.Fatalf("projection message leaked command output: %q", mapped[0].Message)
	}
}

func TestFromAgentEventProjectsWorkspaceEditResults(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		tool    string
		body    string
		message string
	}{
		{
			name:    "replace success",
			tool:    "workspace_replace_text",
			body:    `{"status":"completed","summary":"replace typo","target_path":"README.md","changed":true}`,
			message: "Edited README.md",
		},
		{
			name:    "write success",
			tool:    "workspace_write_file",
			body:    `{"status":"completed","summary":"write note","target_path":"notes.md","changed":true}`,
			message: "Wrote notes.md",
		},
		{
			name:    "conflict",
			tool:    "workspace_replace_text",
			body:    `{"status":"conflict","summary":"replace typo","target_path":"README.md","changed":false,"conflict":true}`,
			message: "Edit conflict in README.md",
		},
		{
			name:    "rejected",
			tool:    "workspace_write_file",
			body:    `{"status":"rejected","summary":"not this way","target_path":"README.md"}`,
			message: "Tool use rejected",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mapped := FromAgentEvent("session_1", "turn_1", "checkpoint_1", agenticToolResultEvent(tc.tool, "call_edit", tc.body))
			if len(mapped) != 1 || mapped[0].Type != protocol.EventProgress {
				t.Fatalf("expected progress projection, got %+v", mapped)
			}
			if mapped[0].Message != tc.message {
				t.Fatalf("message = %q, want %q", mapped[0].Message, tc.message)
			}
		})
	}
}

func TestFromAgentEventProjectsWorkspaceSearchListAndRead(t *testing.T) {
	t.Parallel()

	cases := []struct {
		tool    string
		body    string
		message string
	}{
		{
			tool:    "workspace_search",
			body:    `[{"path":"README.md","line":1,"snippet":"marker"},{"path":"README.md","line":2,"snippet":"marker"}]`,
			message: "Search found 2 matches",
		},
		{
			tool:    "workspace_search",
			body:    `[]`,
			message: "Search found no matches",
		},
		{
			tool:    "workspace_list_files",
			body:    `[{"path":"README.md"},{"path":"go.mod"}]`,
			message: "Listed 2 files",
		},
		{
			tool:    "workspace_read_file",
			body:    `{"status":"completed","summary":"README.md","output":"# Very long file content"}`,
			message: "Read README.md",
		},
	}

	for _, tc := range cases {
		t.Run(tc.tool+"/"+tc.message, func(t *testing.T) {
			mapped := FromAgentEvent("session_1", "turn_1", "checkpoint_1", agenticToolResultEvent(tc.tool, "call_tool", tc.body))
			if len(mapped) != 1 || mapped[0].Message != tc.message {
				t.Fatalf("unexpected projection: %+v", mapped)
			}
			if strings.Contains(mapped[0].Message, "Very long file content") {
				t.Fatalf("read projection leaked file content: %q", mapped[0].Message)
			}
		})
	}
}

func TestFromAgentEventKeepsNonWorkspaceToolResultAsAssistant(t *testing.T) {
	t.Parallel()

	mapped := FromAgentEvent("session_1", "turn_1", "checkpoint_1", agenticToolResultEvent("paper_digest", "call_paper", "digest complete"))
	if len(mapped) != 1 || mapped[0].Type != protocol.EventAssistant {
		t.Fatalf("expected non-workspace tool result to stay assistant text, got %+v", mapped)
	}
	if mapped[0].Message != "digest complete" {
		t.Fatalf("unexpected assistant message: %q", mapped[0].Message)
	}
}

func agenticTextMessage(text string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: text}),
		},
	}
}

func agenticToolResultEvent(name, callID, text string) *adk.TypedAgentEvent[*schema.AgenticMessage] {
	return &adk.TypedAgentEvent[*schema.AgenticMessage]{
		AgentName: "papersilm",
		Output: &adk.TypedAgentOutput[*schema.AgenticMessage]{
			MessageOutput: &adk.TypedMessageVariant[*schema.AgenticMessage]{
				Message: &schema.AgenticMessage{
					Role: schema.AgenticRoleTypeUser,
					ContentBlocks: []*schema.ContentBlock{
						schema.NewContentBlock(&schema.FunctionToolResult{
							Name:   name,
							CallID: callID,
							Content: []*schema.FunctionToolResultContentBlock{{
								Type: schema.FunctionToolResultContentBlockTypeText,
								Text: &schema.UserInputText{Text: text},
							}},
						}),
					},
				},
			},
		},
	}
}
