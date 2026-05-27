package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/pipeline"
	"github.com/zzqDeco/papersilm/internal/runtime/agenttool"
	"github.com/zzqDeco/papersilm/internal/runtime/prepare"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/internal/tools"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type nativeE2ESink struct {
	events []protocol.StreamEvent
}

func (s *nativeE2ESink) Emit(event protocol.StreamEvent) error {
	s.events = append(s.events, event)
	return nil
}

type scriptedAgenticModel struct {
	mu        sync.Mutex
	toolNames []string
}

var nativeE2ESessionSeq atomic.Uint64

func (m *scriptedAgenticModel) Generate(_ context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.AgenticMessage, error) {
	m.recordTools(opts...)
	if result, ok := latestToolResult(input); ok {
		return assistantText("tool result: " + result), nil
	}
	user := strings.ToLower(latestUserInput(input))
	switch {
	case strings.Contains(user, "read"):
		return toolCall("workspace_read_file", `{"path":"README.md"}`), nil
	case strings.Contains(user, "write"):
		content := "eino native note\n"
		if strings.Contains(user, "again") {
			content = "eino native note again\n"
		}
		args, _ := json.Marshal(map[string]string{
			"path":    "notes.md",
			"content": content,
			"summary": "eino write smoke",
		})
		return toolCall("workspace_write_file", string(args)), nil
	case strings.Contains(user, "shell"):
		return toolCall("workspace_run_command", `{"command":"printf %s e2e-shell","summary":"eino shell smoke"}`), nil
	default:
		return assistantText("direct response: " + latestUserInput(input)), nil
	}
}

func (m *scriptedAgenticModel) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	reader, writer := schema.Pipe[*schema.AgenticMessage](1)
	go func() {
		defer writer.Close()
		msg, err := m.Generate(ctx, input, opts...)
		writer.Send(msg, err)
	}()
	return reader, nil
}

func (m *scriptedAgenticModel) recordTools(opts ...einomodel.Option) {
	common := einomodel.GetCommonOptions(nil, opts...)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, info := range common.Tools {
		if info != nil {
			m.toolNames = append(m.toolNames, info.Name)
		}
	}
}

func (m *scriptedAgenticModel) sawTool(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, got := range m.toolNames {
		if got == name {
			return true
		}
	}
	return false
}

func TestEinoNativeAssistantUsesWorkspaceToolsAndPersistsEvents(t *testing.T) {
	rt, store, model, sink := newNativeE2ERuntime(t)
	sessionID := newNativeE2ESession(t, store, protocol.PermissionModeAuto)
	if err := os.WriteFile(filepath.Join(store.WorkspaceRoot(), "README.md"), []byte("workspace e2e content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md): %v", err)
	}
	if err := store.RefreshWorkspaceState(); err != nil {
		t.Fatalf("RefreshWorkspaceState: %v", err)
	}

	result, err := rt.runEinoAssistant(context.Background(), sessionID, "read README.md", protocol.PermissionModeAuto, "turn_read")
	if err != nil {
		t.Fatalf("runEinoAssistant(read): %v", err)
	}
	if !strings.Contains(result.Response, "workspace e2e content") {
		t.Fatalf("expected tool result in response, got %q", result.Response)
	}
	if !model.sawTool("workspace_read_file") {
		t.Fatalf("expected ChatModelAgent to receive workspace_read_file tool")
	}
	if len(sink.events) == 0 {
		t.Fatalf("expected visible stream events to be emitted")
	}
	events, err := store.LoadRecentEvents(sessionID, 20)
	if err != nil {
		t.Fatalf("LoadRecentEvents: %v", err)
	}
	if !streamEventsContain(events, protocol.EventAssistant, "workspace e2e content") {
		t.Fatalf("expected assistant event with tool output, got %+v", events)
	}
	rawAgentEvents, err := os.ReadFile(filepath.Join(store.SessionDir(sessionID), "agent_events.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile(agent_events): %v", err)
	}
	if !strings.Contains(string(rawAgentEvents), `"turn_id":"turn_read"`) {
		t.Fatalf("expected raw agent event log with turn id, got %s", string(rawAgentEvents))
	}
}

func TestEinoNativeToolPermissionAcceptAndReject(t *testing.T) {
	rt, store, _, _ := newNativeE2ERuntime(t)
	sessionID := newNativeE2ESession(t, store, protocol.PermissionModeConfirm)

	pending, err := rt.runEinoAssistant(context.Background(), sessionID, "write notes", protocol.PermissionModeConfirm, "turn_write")
	if err != nil {
		t.Fatalf("runEinoAssistant(write): %v", err)
	}
	if pending.Approval == nil || len(pending.Approval.Requests) != 1 {
		t.Fatalf("expected tool-scoped write approval, got %+v", pending.Approval)
	}
	if _, err := os.Stat(filepath.Join(store.WorkspaceRoot(), "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("write should not happen before approval, stat err=%v", err)
	}

	accepted, err := rt.DecidePermission(context.Background(), sessionID, protocol.PermissionDecision{
		RequestID: pending.Approval.ActiveRequestID,
		Value:     agenttool.PermissionAcceptOnce,
	}, "turn_write_resume")
	if err != nil {
		t.Fatalf("DecidePermission(accept-once): %v", err)
	}
	if accepted.Session.Meta.State != protocol.SessionStateCompleted || accepted.Session.Approval != nil {
		t.Fatalf("expected completed session with cleared approval, got %+v", accepted.Session)
	}
	content, err := os.ReadFile(filepath.Join(store.WorkspaceRoot(), "notes.md"))
	if err != nil {
		t.Fatalf("ReadFile(notes.md): %v", err)
	}
	if string(content) != "eino native note\n" {
		t.Fatalf("expected approved write content, got %q", string(content))
	}

	rejectSessionID := newNativeE2ESession(t, store, protocol.PermissionModeConfirm)
	rejectPending, err := rt.runEinoAssistant(context.Background(), rejectSessionID, "write notes", protocol.PermissionModeConfirm, "turn_reject")
	if err != nil {
		t.Fatalf("runEinoAssistant(reject write): %v", err)
	}
	rejected, err := rt.DecidePermission(context.Background(), rejectSessionID, protocol.PermissionDecision{
		RequestID: rejectPending.Approval.ActiveRequestID,
		Value:     agenttool.PermissionReject,
		Feedback:  "do not write yet",
	}, "turn_reject_resume")
	if err != nil {
		t.Fatalf("DecidePermission(reject): %v", err)
	}
	if !strings.Contains(rejected.Response, "rejected") || !strings.Contains(rejected.Response, "do not write yet") {
		t.Fatalf("expected rejection feedback in response, got %q", rejected.Response)
	}
}

func TestEinoNativeAcceptSessionRuleAllowsMatchingToolWithoutInterrupt(t *testing.T) {
	rt, store, _, _ := newNativeE2ERuntime(t)
	sessionID := newNativeE2ESession(t, store, protocol.PermissionModeConfirm)

	pending, err := rt.runEinoAssistant(context.Background(), sessionID, "write notes", protocol.PermissionModeConfirm, "turn_write_once")
	if err != nil {
		t.Fatalf("runEinoAssistant(write once): %v", err)
	}
	if pending.Approval == nil {
		t.Fatalf("expected initial write approval")
	}
	if _, err := rt.DecidePermission(context.Background(), sessionID, protocol.PermissionDecision{
		RequestID: pending.Approval.ActiveRequestID,
		Value:     agenttool.PermissionAcceptSession,
		Scope:     agenttool.PermissionScopePath,
	}, "turn_write_once_resume"); err != nil {
		t.Fatalf("DecidePermission(accept-session): %v", err)
	}

	allowed, err := rt.runEinoAssistant(context.Background(), sessionID, "write notes again", protocol.PermissionModeConfirm, "turn_write_again")
	if err != nil {
		t.Fatalf("runEinoAssistant(write again): %v", err)
	}
	if allowed.Approval != nil || allowed.Session.Approval != nil {
		t.Fatalf("expected session rule to avoid a second interrupt, got result=%+v snapshot=%+v", allowed.Approval, allowed.Session.Approval)
	}
	content, err := os.ReadFile(filepath.Join(store.WorkspaceRoot(), "notes.md"))
	if err != nil {
		t.Fatalf("ReadFile(notes.md): %v", err)
	}
	if string(content) != "eino native note again\n" {
		t.Fatalf("expected second write to apply through session rule, got %q", string(content))
	}
}

func newNativeE2ERuntime(t *testing.T) (*Runtime, *storage.Store, *scriptedAgenticModel, *nativeE2ESink) {
	t.Helper()
	cfg := config.Default()
	root := t.TempDir()
	cfg.BaseDir = filepath.Join(root, ".papersilm")
	store := storage.New(cfg.BaseDir)
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	registry := tools.New(pipeline.New(cfg))
	sink := &nativeE2ESink{}
	rt := New(cfg, store, registry, sink)
	model := &scriptedAgenticModel{}
	rt.prepareAgent = func(ctx context.Context, req prepare.Request) (prepare.PreparedAgent, error) {
		workspaceTools, err := agenttool.BuildWorkspaceTools(agenttool.WorkspaceToolsConfig{
			Store:          req.Store,
			Registry:       req.Registry,
			SessionID:      req.SessionID,
			PermissionMode: req.PermissionMode,
		})
		if err != nil {
			return prepare.PreparedAgent{}, err
		}
		agent, err := adk.NewTypedChatModelAgent(ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
			Name:        "papersilm_e2e",
			Description: "E2E test agent",
			Instruction: "Use workspace tools when asked.",
			Model:       model,
			ToolsConfig: adk.ToolsConfig{
				ToolsNodeConfig:    compose.ToolsNodeConfig{Tools: workspaceTools},
				EmitInternalEvents: true,
			},
			MaxIterations: 4,
		})
		if err != nil {
			return prepare.PreparedAgent{}, err
		}
		return prepare.PreparedAgent{Agent: agent, Instruction: "test"}, nil
	}
	return rt, store, model, sink
}

func newNativeE2ESession(t *testing.T, store *storage.Store, mode protocol.PermissionMode) string {
	t.Helper()
	now := time.Now().UTC()
	sessionID := fmt.Sprintf("sess_e2e_%d_%d", now.UnixNano(), nativeE2ESessionSeq.Add(1))
	meta := protocol.SessionMeta{
		SessionID:      sessionID,
		State:          protocol.SessionStateIdle,
		PermissionMode: mode,
		WorkspaceRoot:  store.WorkspaceRoot(),
		WorkspaceID:    protocol.DefaultWorkspaceID,
		Language:       "zh",
		Style:          "distill",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := store.CreateSession(meta); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return sessionID
}

func assistantText(text string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: text}),
		},
	}
}

func toolCall(name, args string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.FunctionToolCall{
				CallID:    "call_" + name,
				Name:      name,
				Arguments: args,
			}),
		},
	}
}

func latestUserInput(messages []*schema.AgenticMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] == nil || messages[i].Role != schema.AgenticRoleTypeUser {
			continue
		}
		return agenticMessageText(messages[i])
	}
	return ""
}

func latestToolResult(messages []*schema.AgenticMessage) (string, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] == nil {
			continue
		}
		for _, block := range messages[i].ContentBlocks {
			if block == nil || block.FunctionToolResult == nil {
				continue
			}
			var parts []string
			for _, content := range block.FunctionToolResult.Content {
				if content != nil && content.Text != nil {
					parts = append(parts, content.Text.Text)
				}
			}
			return strings.Join(parts, "\n"), true
		}
	}
	return "", false
}

func agenticMessageText(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, block := range msg.ContentBlocks {
		if block == nil {
			continue
		}
		switch {
		case block.UserInputText != nil:
			parts = append(parts, block.UserInputText.Text)
		case block.AssistantGenText != nil:
			parts = append(parts, block.AssistantGenText.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func streamEventsContain(events []protocol.StreamEvent, eventType protocol.StreamEventType, needle string) bool {
	for _, event := range events {
		if event.Type == eventType && strings.Contains(event.Message, needle) {
			return true
		}
	}
	return false
}
