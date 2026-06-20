package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/pipeline"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/internal/tools"
	"github.com/zzqDeco/papersilm/pkg/core"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type tuiServiceRuntimeSmokeHarness struct {
	t         *testing.T
	model     *tuiModel
	runtime   *tuiRuntimeManager
	store     *storage.Store
	workspace string
	sink      *tuiEventSink
}

func newTUIServiceRuntimeSmokeHarness(t *testing.T, mode protocol.PermissionMode) *tuiServiceRuntimeSmokeHarness {
	t.Helper()
	ctx := context.Background()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# service smoke\n\nservice-smoke-marker\nhello typo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md): %v", err)
	}

	cfg := config.Default()
	cfg.BaseDir = filepath.Join(workspace, ".papersilm")
	cfg.PermissionMode = mode
	sink := newTUIEventSink(128)
	store := storage.New(cfg.BaseDir)
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	registry := tools.New(pipeline.New(cfg))
	svc := core.New(cfg, store, registry, sink)
	meta, err := svc.NewSession(mode, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	snapshot, err := store.Snapshot(meta.SessionID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	runtime := &tuiRuntimeManager{
		ctx:   ctx,
		cfg:   cfg,
		store: store,
		svc:   svc,
		ops:   serviceTUIRuntimeOps{svc: svc},
		sink:  sink,
	}
	runtime.drainPendingStartupEvents()
	model := newTUIModel(ctx, runtime, snapshot)
	model.width = 100
	model.height = 30
	model.ready = true
	model.reflow()
	return &tuiServiceRuntimeSmokeHarness{t: t, model: model, runtime: runtime, store: store, workspace: workspace, sink: sink}
}

func (h *tuiServiceRuntimeSmokeHarness) submitPrompt(prompt string) tea.Cmd {
	h.t.Helper()
	h.model.setPromptValue(prompt)
	updated, cmd := h.model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	h.model = updated.(*tuiModel)
	return cmd
}

func (h *tuiServiceRuntimeSmokeHarness) key(msg tea.KeyMsg) tea.Cmd {
	h.t.Helper()
	updated, cmd := h.model.handleKey(msg)
	h.model = updated.(*tuiModel)
	return cmd
}

func (h *tuiServiceRuntimeSmokeHarness) dispatch(msg tea.Msg) {
	h.t.Helper()
	if msg == nil {
		return
	}
	updated, _ := h.model.Update(msg)
	h.model = updated.(*tuiModel)
}

func (h *tuiServiceRuntimeSmokeHarness) drainEvents() {
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

func (h *tuiServiceRuntimeSmokeHarness) finish(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		h.t.Fatalf("expected command")
	}
	msg := cmd()
	h.drainEvents()
	h.dispatch(msg)
	h.drainEvents()
}

func TestTUIServiceRuntimeSmokePromptPersistsEventsAndTranscript(t *testing.T) {
	t.Parallel()

	h := newTUIServiceRuntimeSmokeHarness(t, protocol.PermissionModeAuto)
	cmd := h.submitPrompt("search `service-smoke-marker`")
	if !h.model.busy {
		t.Fatalf("expected service-backed prompt to enter busy state")
	}
	if got := strings.TrimSpace(h.model.input.Value()); got != "" {
		t.Fatalf("expected prompt input to clear after submit, got %q", got)
	}

	h.finish(cmd)
	if h.model.busy {
		t.Fatalf("expected service-backed prompt to clear busy state")
	}
	if h.model.snapshot.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("expected completed session, got %s status=%q screen:\n%s", h.model.snapshot.Meta.State, h.model.mainStatus, h.model.renderMainScreen())
	}

	view := h.model.renderMainScreen()
	for _, want := range []string{"search `service-smoke-marker`", "Search results", "README.md", "service-smoke-marker", "· Inspecting workspace"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in main screen:\n%s", want, view)
		}
	}
	for _, bad := range []string{"tool=", "node=", "agent_event", "activity.grouped", "Permission decision:"} {
		if strings.Contains(view, bad) {
			t.Fatalf("did not expect low-level runtime text %q in main screen:\n%s", bad, view)
		}
	}

	events, err := h.store.LoadRecentEvents(h.model.snapshot.Meta.SessionID, 20)
	if err != nil {
		t.Fatalf("LoadRecentEvents: %v", err)
	}
	for _, want := range []protocol.StreamEventType{protocol.EventPlan, protocol.EventProgress, protocol.EventResult} {
		if !serviceSmokeEventsContainType(events, want) {
			t.Fatalf("expected stored event %s, got %+v", want, events)
		}
	}
	if !serviceSmokeEventsContainProgress(events, protocol.PlanProgressStarted) || !serviceSmokeEventsContainProgress(events, protocol.PlanProgressCompleted) {
		t.Fatalf("expected started and completed progress events, got %+v", events)
	}

	transcript, err := h.store.LoadTranscript(h.model.snapshot.Meta.SessionID)
	if err != nil {
		t.Fatalf("LoadTranscript: %v", err)
	}
	for _, want := range []string{"search `service-smoke-marker`", "tool=workspace_search", "Search results", "service-smoke-marker"} {
		if !serviceSmokeTranscriptContains(transcript, want) {
			t.Fatalf("expected transcript to contain %q, got %+v", want, transcript)
		}
	}
}

func TestTUIServiceRuntimeSmokeCommandPermissionResumeShowsResult(t *testing.T) {
	t.Parallel()

	h := newTUIServiceRuntimeSmokeHarness(t, protocol.PermissionModeConfirm)
	h.finish(h.submitPrompt("run command `printf %s service-shell`"))

	if !h.model.approvalPanelActive() || h.model.snapshot.Approval == nil {
		t.Fatalf("expected command permission panel, got approval=%+v", h.model.snapshot.Approval)
	}
	request := h.model.snapshot.Approval.Requests[0]
	if request.Tool != string(protocol.NodeKindWorkspaceCommand) || request.Command != "printf %s service-shell" {
		t.Fatalf("unexpected command permission request: %+v", request)
	}
	view := h.model.renderMainScreen()
	for _, want := range []string{"Run command", "printf %s service-shell", "Enter select", "› Ask about workspace or papers"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in permission screen:\n%s", want, view)
		}
	}

	h.model.setPromptValue("draft after command approval")
	h.finish(h.key(tea.KeyMsg{Type: tea.KeyEnter}))
	if h.model.approvalPanelActive() {
		t.Fatalf("expected approval panel to clear after command resume")
	}
	if got := h.model.input.Value(); got != "draft after command approval" {
		t.Fatalf("expected prompt draft to survive command approval, got %q", got)
	}
	view = h.model.renderMainScreen()
	for _, want := range []string{"✓ Approved", "Command: printf %s service-shell", "command exited 0", "service-shell", "draft after command approval"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q after command approval:\n%s", want, view)
		}
	}
	for _, bad := range []string{"Permission decision: accept-once", "tool=", "node=", "Error"} {
		if strings.Contains(view, bad) {
			t.Fatalf("did not expect %q after command approval:\n%s", bad, view)
		}
	}

	transcript, err := h.store.LoadTranscript(h.model.snapshot.Meta.SessionID)
	if err != nil {
		t.Fatalf("LoadTranscript: %v", err)
	}
	for _, want := range []string{"Approval Required", "✓ Approved", "command exited 0", "service-shell"} {
		if !serviceSmokeTranscriptContains(transcript, want) {
			t.Fatalf("expected transcript to contain %q, got %+v", want, transcript)
		}
	}
}

func TestTUIServiceRuntimeSmokeEditPermissionPreviewAndApply(t *testing.T) {
	t.Parallel()

	h := newTUIServiceRuntimeSmokeHarness(t, protocol.PermissionModeConfirm)
	readmePath := filepath.Join(h.workspace, "README.md")
	h.finish(h.submitPrompt("update `README.md` replace `typo` with `type`"))

	if !h.model.approvalPanelActive() || h.model.snapshot.Approval == nil {
		t.Fatalf("expected edit permission panel, got approval=%+v", h.model.snapshot.Approval)
	}
	request := h.model.snapshot.Approval.Requests[0]
	if request.Tool != string(protocol.NodeKindWorkspaceEdit) || request.TargetPath != "README.md" || request.Preview.Kind != "diff" {
		t.Fatalf("unexpected edit permission request: %+v", request)
	}
	before, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("ReadFile(before): %v", err)
	}
	if !strings.Contains(string(before), "hello typo") {
		t.Fatalf("expected edit not to apply before approval, got %q", string(before))
	}
	view := h.model.renderMainScreen()
	for _, want := range []string{"Edit file", "README.md", "-hello typo", "+hello type", "Enter select"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in edit permission screen:\n%s", want, view)
		}
	}

	h.finish(h.key(tea.KeyMsg{Type: tea.KeyEnter}))
	after, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("ReadFile(after): %v", err)
	}
	if !strings.Contains(string(after), "hello type") || strings.Contains(string(after), "hello typo") {
		t.Fatalf("expected approved edit to apply, got %q", string(after))
	}
	view = h.model.renderMainScreen()
	for _, want := range []string{"✓ Approved", "README.md", "Replace text in README.md"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q after edit approval:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Permission decision: accept-once") || strings.Contains(view, "Error") {
		t.Fatalf("did not expect internal decision text or error after edit approval:\n%s", view)
	}
}

func serviceSmokeEventsContainType(events []protocol.StreamEvent, want protocol.StreamEventType) bool {
	for _, event := range events {
		if event.Type == want {
			return true
		}
	}
	return false
}

func serviceSmokeEventsContainProgress(events []protocol.StreamEvent, want protocol.PlanProgressStatus) bool {
	for _, event := range events {
		if event.Type != protocol.EventProgress {
			continue
		}
		if progress, ok := event.Payload.(protocol.PlanProgress); ok && progress.Status == want {
			return true
		}
		if payload, ok := event.Payload.(map[string]interface{}); ok && payload["status"] == string(want) {
			return true
		}
	}
	return false
}

func serviceSmokeTranscriptContains(entries []protocol.TranscriptEntry, needle string) bool {
	for _, entry := range entries {
		if strings.Contains(entry.Title, needle) || strings.Contains(entry.Body, needle) {
			return true
		}
	}
	return false
}
