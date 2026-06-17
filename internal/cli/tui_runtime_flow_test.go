package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type fakeTUIRuntimeOps struct {
	executeFn func(context.Context, protocol.SessionSnapshot, string) (protocol.SessionSnapshot, string, error)
	decideFn  func(context.Context, protocol.SessionSnapshot, protocol.PermissionDecision) (protocol.SessionSnapshot, string, error)

	prompts   []string
	decisions []protocol.PermissionDecision
}

func (f *fakeTUIRuntimeOps) ExecutePrompt(ctx context.Context, snapshot protocol.SessionSnapshot, prompt string) (protocol.SessionSnapshot, string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.executeFn == nil {
		return snapshot, "", nil
	}
	return f.executeFn(ctx, snapshot, prompt)
}

func (f *fakeTUIRuntimeOps) DecidePermission(ctx context.Context, snapshot protocol.SessionSnapshot, decision protocol.PermissionDecision) (protocol.SessionSnapshot, string, error) {
	f.decisions = append(f.decisions, decision)
	if f.decideFn == nil {
		return snapshot, "Permission decision: " + decision.Value, nil
	}
	return f.decideFn(ctx, snapshot, decision)
}

type tuiRuntimeFlowHarness struct {
	t     *testing.T
	model *tuiModel
	sink  *tuiEventSink
	ops   *fakeTUIRuntimeOps
}

func newTUIRuntimeFlowHarness(t *testing.T) *tuiRuntimeFlowHarness {
	t.Helper()
	model := newTestTUIModel()
	sink := newTUIEventSink(64)
	ops := &fakeTUIRuntimeOps{}
	model.runtime = &tuiRuntimeManager{
		ctx:  context.Background(),
		cfg:  config.Default(),
		ops:  ops,
		sink: sink,
	}
	model.snapshot.Meta.SessionID = "sess_runtime_flow"
	model.snapshot.Meta.PermissionMode = protocol.PermissionModeConfirm
	model.reflow()
	return &tuiRuntimeFlowHarness{t: t, model: model, sink: sink, ops: ops}
}

func (h *tuiRuntimeFlowHarness) submitPrompt(prompt string) tea.Cmd {
	h.t.Helper()
	h.model.setPromptValue(prompt)
	updated, cmd := h.model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	h.model = updated.(*tuiModel)
	return cmd
}

func (h *tuiRuntimeFlowHarness) key(msg tea.KeyMsg) tea.Cmd {
	h.t.Helper()
	updated, cmd := h.model.handleKey(msg)
	h.model = updated.(*tuiModel)
	return cmd
}

func (h *tuiRuntimeFlowHarness) dispatch(msg tea.Msg) {
	h.t.Helper()
	if msg == nil {
		return
	}
	updated, _ := h.model.Update(msg)
	h.model = updated.(*tuiModel)
}

func (h *tuiRuntimeFlowHarness) runCmd(cmd tea.Cmd) tea.Msg {
	h.t.Helper()
	if cmd == nil {
		h.t.Fatalf("expected command")
	}
	return cmd()
}

func (h *tuiRuntimeFlowHarness) drainEvents() {
	h.t.Helper()
	for {
		select {
		case event := <-h.sink.ch:
			h.dispatch(tuiEventMsg{Event: event})
		default:
			return
		}
	}
}

func (h *tuiRuntimeFlowHarness) finish(cmd tea.Cmd) {
	h.t.Helper()
	msg := h.runCmd(cmd)
	h.drainEvents()
	h.dispatch(msg)
}

func (h *tuiRuntimeFlowHarness) emit(event protocol.StreamEvent) {
	h.t.Helper()
	if event.SessionID == "" {
		event.SessionID = h.model.snapshot.Meta.SessionID
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if err := h.sink.Emit(event); err != nil {
		h.t.Fatalf("emit event: %v", err)
	}
}

func TestTUIRuntimeFlowPromptProjectsEvents(t *testing.T) {
	t.Parallel()

	h := newTUIRuntimeFlowHarness(t)
	h.ops.executeFn = func(ctx context.Context, snapshot protocol.SessionSnapshot, prompt string) (protocol.SessionSnapshot, string, error) {
		h.emit(protocol.StreamEvent{
			Type:    protocol.EventProgress,
			Message: "started",
			Payload: protocol.PlanProgress{
				Status:  protocol.PlanProgressStarted,
				Tool:    string(protocol.NodeKindWorkspaceSearch),
				NodeID:  "search_readme",
				Message: "node execution started",
			},
		})
		h.emit(protocol.StreamEvent{
			Type:    protocol.EventAssistant,
			Message: "Workspace summary is ready.",
		})
		after := snapshot
		after.Meta.State = protocol.SessionStateCompleted
		return after, "Runtime completed.", nil
	}

	cmd := h.submitPrompt("summarize workspace")
	if !h.model.busy {
		t.Fatalf("expected prompt submit to enter busy state")
	}
	if got := strings.TrimSpace(h.model.input.Value()); got != "" {
		t.Fatalf("expected prompt input to clear after submit, got %q", got)
	}
	if len(h.ops.prompts) != 0 {
		t.Fatalf("did not expect runtime call before command executes")
	}

	h.finish(cmd)
	if h.model.busy {
		t.Fatalf("expected runtime completion to clear busy state")
	}
	if len(h.ops.prompts) != 1 || h.ops.prompts[0] != "summarize workspace" {
		t.Fatalf("unexpected prompts: %+v", h.ops.prompts)
	}

	view := h.model.renderMainScreen()
	for _, want := range []string{"summarize workspace", "Workspace summary is ready.", "· Inspecting workspace", "Runtime completed."} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in main screen:\n%s", want, view)
		}
	}
	for _, bad := range []string{"tool=", "node=", "agent_event", "activity.grouped"} {
		if strings.Contains(view, bad) {
			t.Fatalf("did not expect low-level runtime text %q in main screen:\n%s", bad, view)
		}
	}
}

func TestTUIRuntimeFlowPromptPreservesDraftDuringPermission(t *testing.T) {
	t.Parallel()

	h := newTUIRuntimeFlowHarness(t)
	h.ops.executeFn = func(ctx context.Context, snapshot protocol.SessionSnapshot, prompt string) (protocol.SessionSnapshot, string, error) {
		after := runtimeFlowPendingApprovalSnapshot(snapshot)
		h.emit(protocol.StreamEvent{
			Type:    protocol.EventApprovalRequired,
			Message: "permission required",
			Payload: *after.Approval,
		})
		return after, "Approval required.", nil
	}

	h.finish(h.submitPrompt("prepare edit"))
	h.model.setPromptValue("draft after permission")
	view := h.model.renderMainScreen()
	for _, want := range []string{"Edit README.md", "Do you want to make this edit?", "Enter select", "› draft after permission"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in permission screen:\n%s", want, view)
		}
	}
	if timeline := h.model.renderTimelineContent(80); strings.Contains(timeline, "Approval Required") || strings.Contains(timeline, "Edit README.md") {
		t.Fatalf("expected pending approval to stay out of main timeline:\n%s", timeline)
	}
	transcript := h.model.renderTranscriptContent(100)
	if !strings.Contains(transcript, "Approval Required") || !strings.Contains(transcript, "Edit README.md") {
		t.Fatalf("expected transcript to retain approval event:\n%s", transcript)
	}
}

func TestTUIRuntimeFlowApprovePermissionResumes(t *testing.T) {
	t.Parallel()

	h := newTUIRuntimeFlowHarness(t)
	h.model.snapshot = runtimeFlowPendingApprovalSnapshot(h.model.snapshot)
	h.model.setPromptValue("draft after permission")
	h.ops.decideFn = func(ctx context.Context, snapshot protocol.SessionSnapshot, decision protocol.PermissionDecision) (protocol.SessionSnapshot, string, error) {
		after := runtimeFlowClearedApprovalSnapshot(snapshot)
		return after, "Permission decision: " + decision.Value, nil
	}

	cmd := h.key(tea.KeyMsg{Type: tea.KeyEnter})
	if !h.model.busy {
		t.Fatalf("expected permission decision to enter busy state")
	}
	h.finish(cmd)

	if len(h.ops.decisions) != 1 {
		t.Fatalf("expected one permission decision, got %+v", h.ops.decisions)
	}
	decision := h.ops.decisions[0]
	if decision.RequestID != "req_edit" || decision.Value != tuiPermissionAcceptOnce {
		t.Fatalf("unexpected permission decision: %+v", decision)
	}
	if h.model.approvalPanelActive() {
		t.Fatalf("expected approval panel to clear after resume")
	}
	if got := h.model.input.Value(); got != "draft after permission" {
		t.Fatalf("expected prompt draft to survive approval, got %q", got)
	}

	view := h.model.renderMainScreen()
	for _, want := range []string{"✓ Approved", "README.md", "draft after permission"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q after approve:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Permission decision: accept-once") || strings.Contains(view, "Edit README.md") {
		t.Fatalf("did not expect internal decision text or pending panel after approve:\n%s", view)
	}
}

func TestTUIRuntimeFlowRejectPermissionWithFeedback(t *testing.T) {
	t.Parallel()

	h := newTUIRuntimeFlowHarness(t)
	h.model.snapshot = runtimeFlowPendingApprovalSnapshot(h.model.snapshot)
	h.ops.decideFn = func(ctx context.Context, snapshot protocol.SessionSnapshot, decision protocol.PermissionDecision) (protocol.SessionSnapshot, string, error) {
		after := runtimeFlowClearedApprovalSnapshot(snapshot)
		after.Meta.State = protocol.SessionStatePlanned
		return after, "Permission decision: " + decision.Value, nil
	}

	h.key(tea.KeyMsg{Type: tea.KeyDown})
	h.key(tea.KeyMsg{Type: tea.KeyDown})
	h.key(tea.KeyMsg{Type: tea.KeyTab})
	h.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("use tests only")})
	cmd := h.key(tea.KeyMsg{Type: tea.KeyEnter})
	h.finish(cmd)

	if len(h.ops.decisions) != 1 {
		t.Fatalf("expected one permission decision, got %+v", h.ops.decisions)
	}
	decision := h.ops.decisions[0]
	if decision.Value != tuiPermissionReject || decision.Feedback != "use tests only" {
		t.Fatalf("unexpected reject decision: %+v", decision)
	}
	view := h.model.renderMainScreen()
	for _, want := range []string{"Tool use rejected", "Feedback: use tests only"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q after reject:\n%s", want, view)
		}
	}
	for _, bad := range []string{"Error", "Permission decision: reject", "Edit README.md"} {
		if strings.Contains(view, bad) {
			t.Fatalf("did not expect %q after reject:\n%s", bad, view)
		}
	}
}

func TestTUIRuntimeFlowRuntimeErrorDoesNotClearPendingApproval(t *testing.T) {
	t.Parallel()

	h := newTUIRuntimeFlowHarness(t)
	h.model.snapshot = runtimeFlowPendingApprovalSnapshot(h.model.snapshot)
	h.ops.decideFn = func(ctx context.Context, snapshot protocol.SessionSnapshot, decision protocol.PermissionDecision) (protocol.SessionSnapshot, string, error) {
		return snapshot, "", errors.New("resume failed")
	}

	cmd := h.key(tea.KeyMsg{Type: tea.KeyEnter})
	h.finish(cmd)

	if len(h.ops.decisions) != 1 {
		t.Fatalf("expected one permission decision, got %+v", h.ops.decisions)
	}
	if !h.model.approvalPanelActive() {
		t.Fatalf("expected pending approval to remain after runtime error")
	}
	view := h.model.renderMainScreen()
	for _, want := range []string{"resume failed", "Edit README.md"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q after runtime error:\n%s", want, view)
		}
	}
	if strings.Contains(view, "✓ Approved") {
		t.Fatalf("did not expect successful approval decision after runtime error:\n%s", view)
	}
}

func runtimeFlowPendingApprovalSnapshot(snapshot protocol.SessionSnapshot) protocol.SessionSnapshot {
	after := snapshot
	after.Meta.State = protocol.SessionStateAwaitingApproval
	after.Meta.ApprovalPending = true
	after.Approval = &protocol.ApprovalRequest{
		ActiveRequestID: "req_edit",
		Requests: []protocol.PermissionRequest{
			{
				RequestID:  "req_edit",
				SessionID:  after.Meta.SessionID,
				Tool:       string(protocol.NodeKindWorkspaceEdit),
				Operation:  "write",
				Title:      "Edit README.md",
				Subtitle:   "README.md",
				Question:   "Do you want to make this edit?",
				Summary:    "Prepared workspace edit preview",
				TargetPath: "README.md",
				Preview: protocol.PermissionPreview{
					Kind: "diff",
					Diff: "--- README.md\n+++ README.md\n-old line\n+new line",
				},
				Options: []protocol.PermissionOption{
					{Value: tuiPermissionAcceptOnce, Label: "Yes", Scope: "node", Feedback: tuiPermissionFeedbackAccept},
					{Value: tuiPermissionAcceptSession, Label: "Yes, during this session", Scope: "path", Feedback: tuiPermissionFeedbackAccept},
					{Value: tuiPermissionReject, Label: "No", Scope: "node", Feedback: tuiPermissionFeedbackReject},
				},
				CreatedAt: time.Now().UTC(),
			},
		},
	}
	return after
}

func runtimeFlowClearedApprovalSnapshot(snapshot protocol.SessionSnapshot) protocol.SessionSnapshot {
	after := snapshot
	after.Meta.State = protocol.SessionStateCompleted
	after.Meta.ApprovalPending = false
	after.Approval = nil
	return after
}
