package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tuiui "github.com/zzqDeco/papersilm/internal/cli/tui"
	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestTranscriptHistoryEntriesRespectsMode(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	entries := []protocol.TranscriptEntry{
		{ID: "1", Type: protocol.TranscriptEntryUser, Body: "first prompt", InputMode: protocol.TranscriptInputPrompt, CreatedAt: now},
		{ID: "2", Type: protocol.TranscriptEntryCommand, Body: "/tasks", InputMode: protocol.TranscriptInputCommand, CreatedAt: now},
		{ID: "3", Type: protocol.TranscriptEntryUser, Body: "second prompt", InputMode: protocol.TranscriptInputPrompt, CreatedAt: now},
	}

	promptHistory := transcriptHistoryEntries(entries, protocol.TranscriptInputPrompt)
	if len(promptHistory) != 2 {
		t.Fatalf("expected 2 prompt history items, got %+v", promptHistory)
	}
	if promptHistory[0].Body != "second prompt" || promptHistory[1].Body != "first prompt" {
		t.Fatalf("expected reverse chronological prompt history, got %+v", promptHistory)
	}

	commandHistory := transcriptHistoryEntries(entries, protocol.TranscriptInputCommand)
	if len(commandHistory) != 1 || commandHistory[0].Body != "/tasks" {
		t.Fatalf("expected command history to only include /tasks, got %+v", commandHistory)
	}
}

func TestTranscriptEntryFromApprovalEventUsesDecisionSubtype(t *testing.T) {
	t.Parallel()

	entry, ok := transcriptEntryFromEvent(protocol.StreamEvent{
		Type:      protocol.EventApprovalRequired,
		SessionID: "sess_test",
		Message:   "approval required",
		CreatedAt: time.Now().UTC(),
	})
	if !ok {
		t.Fatalf("expected approval event to map into transcript")
	}
	if entry.Type != protocol.TranscriptEntryApproval {
		t.Fatalf("expected approval entry, got %q", entry.Type)
	}
	if entry.Subtype != transcriptSubtypeApprovalRequired {
		t.Fatalf("expected approval.required subtype, got %q", entry.Subtype)
	}
}

func TestAmbientEventsStayOutOfMainTimeline(t *testing.T) {
	t.Parallel()

	entry, ok := transcriptEntryFromEvent(protocol.StreamEvent{
		Type:      protocol.EventInit,
		SessionID: "sess_test",
		Message:   "session created",
		CreatedAt: time.Now().UTC(),
	})
	if !ok {
		t.Fatalf("expected init event to stay in transcript")
	}
	if entry.Visibility != protocol.TranscriptVisibilityAmbient || entry.Presentation != protocol.TranscriptPresentationHidden {
		t.Fatalf("expected ambient hidden init event, got visibility=%q presentation=%q", entry.Visibility, entry.Presentation)
	}

	model := newTestTUIModel()
	model.hydrateTranscript([]protocol.TranscriptEntry{entry})
	if model.messageStore.Len() != 1 {
		t.Fatalf("expected transcript to retain init event, got %+v", model.transcriptEntries())
	}
	if len(model.items) != 1 || model.items[0].Subtype != "welcome" {
		t.Fatalf("expected main timeline to show only welcome item, got %+v", model.items)
	}
	if containsString(model.renderTimelineContent(80), "session created") {
		t.Fatalf("did not expect session created in main timeline")
	}
	if !containsString(model.renderTranscriptContent(80), "session created") {
		t.Fatalf("expected transcript to show session created")
	}
}

func TestProgressEventsGroupIntoSingleActivityRow(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.items = nil
	model.messageViewport.Reset()

	model.appendTranscript(protocol.TranscriptEntry{
		ID:           "p1",
		SessionID:    "sess_test",
		Type:         protocol.TranscriptEntryProgress,
		Subtype:      string(protocol.EventProgress),
		Title:        "Progress",
		Body:         "node execution started",
		Visibility:   protocol.TranscriptVisibilityActivity,
		Presentation: protocol.TranscriptPresentationGrouped,
		CreatedAt:    time.Now().UTC(),
	}, false)
	model.appendTranscript(protocol.TranscriptEntry{
		ID:           "p2",
		SessionID:    "sess_test",
		Type:         protocol.TranscriptEntryProgress,
		Subtype:      string(protocol.EventProgress),
		Title:        "Progress",
		Body:         "node execution completed",
		Visibility:   protocol.TranscriptVisibilityActivity,
		Presentation: protocol.TranscriptPresentationGrouped,
		CreatedAt:    time.Now().UTC(),
	}, false)

	if model.messageStore.Len() != 2 {
		t.Fatalf("expected transcript to retain both progress entries, got %+v", model.transcriptEntries())
	}
	if len(model.items) != 1 {
		t.Fatalf("expected one grouped activity item, got %+v", model.items)
	}
	if model.items[0].Kind != tuiItemProgress || !containsString(model.items[0].Body, "2 updates") {
		t.Fatalf("expected grouped progress body, got %+v", model.items[0])
	}
}

func TestWorkspaceActivitySummarizesToolsInsteadOfUpdates(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.items = nil
	model.messageViewport.Reset()

	model.appendTranscript(protocol.TranscriptEntry{
		ID:           "p1",
		SessionID:    "sess_test",
		Type:         protocol.TranscriptEntryProgress,
		Subtype:      string(protocol.EventProgress),
		Title:        "Progress",
		Body:         "started · tool=workspace_search · node=search_readme",
		Visibility:   protocol.TranscriptVisibilityActivity,
		Presentation: protocol.TranscriptPresentationGrouped,
		CreatedAt:    time.Now().UTC(),
	}, false)
	model.appendTranscript(protocol.TranscriptEntry{
		ID:           "p2",
		SessionID:    "sess_test",
		Type:         protocol.TranscriptEntryProgress,
		Subtype:      string(protocol.EventProgress),
		Title:        "Progress",
		Body:         "started · tool=workspace_inspect · node=read_readme",
		Visibility:   protocol.TranscriptVisibilityActivity,
		Presentation: protocol.TranscriptPresentationGrouped,
		CreatedAt:    time.Now().UTC(),
	}, false)

	if len(model.items) != 1 {
		t.Fatalf("expected one grouped activity item, got %+v", model.items)
	}
	body := model.items[0].Body
	if !containsString(body, "Inspecting workspace") || !containsString(body, "1 search") || !containsString(body, "1 read") {
		t.Fatalf("expected workspace activity summary, got %q", body)
	}
	if containsString(body, "2 updates") {
		t.Fatalf("did not expect generic update count when tool stats exist, got %q", body)
	}
	if containsString(body, "tool=") || containsString(body, "node=") {
		t.Fatalf("did not expect low-level tool details in grouped activity, got %q", body)
	}
	rendered := model.renderTimelineItem(model.items[0], 80)
	if !containsString(rendered, "⏺ Inspecting workspace") {
		t.Fatalf("expected compact activity row, got %q", rendered)
	}
	if containsString(rendered, "Progress") || containsString(rendered, "activity.grouped") {
		t.Fatalf("did not expect activity to render as log header, got %q", rendered)
	}
}

func TestPlainAssistantMessageRendersWithoutLogHeader(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	rendered := model.renderTimelineItem(tuiTimelineItem{
		Kind:      tuiItemAssistant,
		Title:     "Assistant",
		Body:      "Here is the answer.",
		CreatedAt: time.Now().UTC(),
	}, 80)
	if containsString(rendered, "assistant ·") {
		t.Fatalf("did not expect assistant log header, got %q", rendered)
	}
	if !containsString(rendered, "Here is the answer.") {
		t.Fatalf("expected assistant body, got %q", rendered)
	}

	commandResult := model.renderTimelineItem(tuiTimelineItem{
		Kind:      tuiItemAssistant,
		Title:     "Plan",
		Body:      "Plan ready.",
		CreatedAt: time.Now().UTC(),
	}, 80)
	if containsString(commandResult, "plan ·") {
		t.Fatalf("did not expect command result to render as log header, got %q", commandResult)
	}
	if !containsString(commandResult, "Plan ready.") {
		t.Fatalf("expected command result body, got %q", commandResult)
	}
}

func TestOnlyPrimaryMessagesIncreaseUnread(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.autoScroll = false
	model.unread = 0
	model.appendTranscript(protocol.TranscriptEntry{
		ID:           "p1",
		SessionID:    "sess_test",
		Type:         protocol.TranscriptEntryProgress,
		Body:         "node execution started",
		Visibility:   protocol.TranscriptVisibilityActivity,
		Presentation: protocol.TranscriptPresentationGrouped,
		CreatedAt:    time.Now().UTC(),
	}, false)
	if model.unread != 0 {
		t.Fatalf("expected activity to avoid unread count, got %d", model.unread)
	}

	model.appendTranscript(protocol.TranscriptEntry{
		ID:        "a1",
		SessionID: "sess_test",
		Type:      protocol.TranscriptEntryAssistant,
		Title:     "Assistant",
		Body:      "done",
		CreatedAt: time.Now().UTC(),
	}, false)
	if model.unread != 1 {
		t.Fatalf("expected assistant message to increment unread, got %d", model.unread)
	}
}

func TestExecutionToTranscriptEntriesBuildsApprovalDecisionMessages(t *testing.T) {
	t.Parallel()

	before := protocol.SessionSnapshot{
		Meta: protocol.SessionMeta{
			SessionID:       "sess_test",
			State:           protocol.SessionStateAwaitingApproval,
			ApprovalPending: true,
		},
		TaskBoard: &protocol.TaskBoard{
			Tasks: []protocol.TaskCard{
				{TaskID: "task_1", Title: "Inspect README", Status: protocol.TaskStatusAwaitingApproval},
			},
		},
	}
	approved := before
	approved.Meta.State = protocol.SessionStateCompleted
	approved.Meta.ApprovalPending = false
	approved.TaskBoard = &protocol.TaskBoard{
		Tasks: []protocol.TaskCard{
			{TaskID: "task_1", Title: "Inspect README", Status: protocol.TaskStatusCompleted},
		},
	}
	entries := executionToTranscriptEntries("/task approve task_1", before, approved, "")
	if len(entries) == 0 {
		t.Fatalf("expected approval transcript entries")
	}
	if entries[0].Subtype != transcriptSubtypeApprovalApproved {
		t.Fatalf("expected approval.approved subtype, got %q", entries[0].Subtype)
	}
	if !strings.Contains(entries[0].Body, "Inspect README") {
		t.Fatalf("expected task title in approval body, got %q", entries[0].Body)
	}

	rejected := before
	rejected.Meta.State = protocol.SessionStatePlanned
	rejected.Meta.ApprovalPending = false
	rejected.TaskBoard = &protocol.TaskBoard{
		Tasks: []protocol.TaskCard{
			{TaskID: "task_1", Title: "Inspect README", Status: protocol.TaskStatusSkipped, Error: "skipped after manual rejection"},
		},
	}
	entries = executionToTranscriptEntries("/task reject task_1", before, rejected, "")
	if len(entries) == 0 {
		t.Fatalf("expected rejection transcript entries")
	}
	if entries[0].Subtype != transcriptSubtypeApprovalRejected {
		t.Fatalf("expected approval.rejected subtype, got %q", entries[0].Subtype)
	}
	if !strings.Contains(entries[0].Body, "Reason: skipped after manual rejection") {
		t.Fatalf("expected rejection reason in body, got %q", entries[0].Body)
	}
}

func TestExecutionToTranscriptEntriesBuildsPermissionDecisionMessages(t *testing.T) {
	t.Parallel()

	before := protocol.SessionSnapshot{
		Meta: protocol.SessionMeta{SessionID: "sess_test", State: protocol.SessionStateAwaitingApproval, ApprovalPending: true},
		Approval: &protocol.ApprovalRequest{
			ActiveRequestID: "req_1",
			Requests: []protocol.PermissionRequest{
				{
					RequestID:  "req_1",
					Tool:       string(protocol.NodeKindWorkspaceCommand),
					Title:      "Run command",
					Command:    "go test ./...",
					TargetPath: "",
				},
			},
		},
	}
	after := before
	after.Meta.State = protocol.SessionStateCompleted
	after.Meta.ApprovalPending = false
	after.Approval = nil

	entries := executionToTranscriptEntries("/permission reject node -- use unit tests only", before, after, "Permission decision: reject")
	if len(entries) == 0 {
		t.Fatalf("expected permission decision entry")
	}
	if entries[0].Subtype != transcriptSubtypeApprovalRejected {
		t.Fatalf("expected rejected subtype, got %q", entries[0].Subtype)
	}
	if !strings.Contains(entries[0].Body, "Run command") || !strings.Contains(entries[0].Body, "feedback: use unit tests only") {
		t.Fatalf("expected command and feedback in body, got %q", entries[0].Body)
	}
}

func TestExecutionToTranscriptEntriesSummarizesPendingApproval(t *testing.T) {
	t.Parallel()

	after := protocol.SessionSnapshot{
		Meta: protocol.SessionMeta{
			SessionID:          "sess_test",
			State:              protocol.SessionStateAwaitingApproval,
			ApprovalPending:    true,
			ActivePlanID:       "plan_123",
			ActiveCheckpointID: "checkpoint_123",
		},
		TaskBoard: &protocol.TaskBoard{
			PlanID: "plan_123",
			Tasks: []protocol.TaskCard{
				{TaskID: "task_1", Title: "Inspect README", Status: protocol.TaskStatusAwaitingApproval},
			},
		},
	}

	entries := executionToTranscriptEntries("hello workspace", protocol.SessionSnapshot{}, after, "Approval required\nPlan: plan_123\nTask Board: noisy details")
	if len(entries) != 1 {
		t.Fatalf("expected only pending approval entry, got %+v", entries)
	}
	if entries[0].Subtype != transcriptSubtypeApprovalRequired {
		t.Fatalf("expected approval.required, got %q", entries[0].Subtype)
	}
	if strings.Contains(entries[0].Body, "Task Board:") || strings.Contains(entries[0].Body, "\n") {
		t.Fatalf("expected compact approval summary, got %q", entries[0].Body)
	}
	if !strings.Contains(entries[0].Body, "Inspect README") || strings.Contains(entries[0].Body, "/approve") {
		t.Fatalf("expected actionable summary, got %q", entries[0].Body)
	}
}

func TestApprovalRequiredRendersDecisionOptions(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.snapshot.Meta.State = protocol.SessionStateAwaitingApproval
	model.snapshot.Meta.ApprovalPending = true
	model.snapshot.Meta.ActivePlanID = "plan_123"
	model.snapshot.TaskBoard = &protocol.TaskBoard{
		PlanID: "plan_123",
		Tasks: []protocol.TaskCard{
			{TaskID: "task_1", Title: "Inspect README", Status: protocol.TaskStatusAwaitingApproval},
		},
	}
	entry, ok := pendingApprovalTranscriptEntry(model.snapshot)
	if !ok {
		t.Fatalf("expected pending approval entry")
	}
	model.appendTranscript(entry, false)
	model.reflow()

	view := model.renderMainScreen()
	for _, want := range []string{"Review plan checkpoint", "❯ Yes", "Yes, during this session", "No", "Tab add feedback"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected approval decision option %q in view:\n%s", want, view)
		}
	}
}

func TestApprovalContextOwnsKeyboardButPreservesDraft(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.snapshot.Meta.State = protocol.SessionStateAwaitingApproval
	model.snapshot.Meta.ApprovalPending = true
	if !hasKeyContext(model.keyContexts(), tuiui.ContextConfirmation) {
		t.Fatalf("expected confirmation context")
	}
	gotModel, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	updated := gotModel.(*tuiModel)
	if updated.input.Value() != "" {
		t.Fatalf("expected approval context to keep chat input unchanged, got %q", updated.input.Value())
	}
	model = updated

	model.input.SetValue("keep typing")
	if !hasKeyContext(model.keyContexts(), tuiui.ContextConfirmation) {
		t.Fatalf("expected confirmation context to stay active while draft is preserved")
	}
	gotModel, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	updated = gotModel.(*tuiModel)
	if updated.input.Value() != "keep typing" {
		t.Fatalf("expected chat draft to be preserved while confirmation owns focus, got %q", updated.input.Value())
	}
	view := updated.renderMainScreen()
	if !strings.Contains(view, "Permission request active") {
		t.Fatalf("expected main status to explain active permission request, got:\n%s", view)
	}
}

func TestApprovalShortcutSelection(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.snapshot.Meta.State = protocol.SessionStateAwaitingApproval
	model.snapshot.Meta.ApprovalPending = true
	_, cmd := model.commitApprovalSelection("r")
	if cmd == nil {
		t.Fatalf("expected rejection command")
	}
	if model.approvalSelection != approvalOptionIndex(model.approvalOptions(), tuiPermissionReject) {
		t.Fatalf("expected r to select reject, got %d", model.approvalSelection)
	}
	if !model.busy {
		t.Fatalf("expected approval action to enter busy state")
	}
}

func TestStatusForSnapshotShowsAwaitingApproval(t *testing.T) {
	t.Parallel()

	status := statusForSnapshot(protocol.SessionSnapshot{
		Meta: protocol.SessionMeta{
			State:           protocol.SessionStateAwaitingApproval,
			ApprovalPending: true,
		},
	})
	if status != "Awaiting approval" {
		t.Fatalf("expected awaiting approval status, got %q", status)
	}

	if got := statusForSnapshot(protocol.SessionSnapshot{}); got != "" {
		t.Fatalf("expected idle snapshot to have no sticky status, got %q", got)
	}
}

func TestPendingApprovalCountDoesNotDoubleCountSessionGate(t *testing.T) {
	t.Parallel()

	count := pendingApprovalCount(protocol.SessionSnapshot{
		Meta: protocol.SessionMeta{ApprovalPending: true},
		TaskBoard: &protocol.TaskBoard{
			Tasks: []protocol.TaskCard{
				{TaskID: "task_1", Status: protocol.TaskStatusAwaitingApproval},
			},
		},
	})
	if count != 1 {
		t.Fatalf("expected one actionable approval, got %d", count)
	}
}

func TestSubmitInputClearsNavigationCommands(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		input      string
		wantScreen tuiScreen
		wantFocus  tuiFocus
		wantModal  tuiModalKind
		wantCmd    bool
	}{
		{name: "transcript", input: "/transcript", wantScreen: tuiScreenTranscript, wantFocus: tuiFocusTranscript},
		{name: "commands", input: "/commands", wantScreen: tuiScreenMain, wantFocus: tuiFocusModal, wantModal: tuiModalCommands, wantCmd: true},
		{name: "model", input: "/model", wantScreen: tuiScreenMain, wantFocus: tuiFocusModal, wantModal: tuiModalProviders, wantCmd: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			model := newTestTUIModel()
			model.input.SetValue(tc.input)
			model.suggestions = []tuiSuggestion{{Label: "one", Insert: "one"}}
			model.sel = 0
			model.historyState = tuiHistoryState{active: true, draft: "draft"}

			gotModel, cmd := model.submitInput()
			if (cmd != nil) != tc.wantCmd {
				t.Fatalf("cmd != nil = %v, want %v", cmd != nil, tc.wantCmd)
			}

			updated := gotModel.(*tuiModel)
			if updated.input.Value() != "" {
				t.Fatalf("expected %s to consume input, got %q", tc.input, updated.input.Value())
			}
			if updated.screen != tc.wantScreen || updated.focus != tc.wantFocus {
				t.Fatalf("expected screen=%q focus=%q, got screen=%q focus=%q", tc.wantScreen, tc.wantFocus, updated.screen, updated.focus)
			}
			if updated.modal.Kind != tc.wantModal {
				t.Fatalf("expected modal kind %q, got %q", tc.wantModal, updated.modal.Kind)
			}
			if updated.historyState.active {
				t.Fatalf("expected history state to reset")
			}
			if len(updated.suggestions) != 0 {
				t.Fatalf("expected suggestions to close, got %+v", updated.suggestions)
			}
		})
	}
}

func TestTranscriptSearchDoesNotOverwriteMainStatus(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.setMainStatus("Running task...")
	model.messageStore.Reset([]protocol.TranscriptEntry{
		{ID: "1", Type: protocol.TranscriptEntryAssistant, Body: "transformer summary"},
		{ID: "2", Type: protocol.TranscriptEntrySystem, Body: "session created"},
	})
	model.openTranscriptScreen(true)
	model.searchIn.SetValue("transformer")
	model.refreshTranscriptSearch()

	if model.mainStatus != "Running task..." {
		t.Fatalf("expected main status to remain unchanged, got %q", model.mainStatus)
	}
	if model.transcriptScreen.Status() != "1 matches" {
		t.Fatalf("expected transcript search status, got %q", model.transcriptScreen.Status())
	}

	model.closeTranscriptScreen()
	if model.mainStatus != "Running task..." {
		t.Fatalf("expected main status to survive transcript close, got %q", model.mainStatus)
	}
	if got := model.renderFooter(); !containsString(got, "Running task...") {
		t.Fatalf("expected main footer to show status after return, got %q", got)
	}
}

func TestTranscriptSearchFooterDoesNotLeakSearchStatus(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.setMainStatus("Running task...")
	model.messageStore.Reset([]protocol.TranscriptEntry{
		{ID: "1", Type: protocol.TranscriptEntryAssistant, Body: "transformer summary"},
	})
	model.openTranscriptScreen(true)
	model.searchIn.SetValue("transformer")
	model.refreshTranscriptSearch()

	footer := model.renderFooter()
	if containsString(footer, "Running task...") || containsString(footer, "matches") {
		t.Fatalf("expected transcript footer to stay clean, got %q", footer)
	}
}

func TestTranscriptHotkeyPreservesDraft(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.input.SetValue("draft prompt")

	gotModel, cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlO})
	if cmd != nil {
		t.Fatalf("expected no command for ctrl+o, got %v", cmd)
	}

	updated := gotModel.(*tuiModel)
	if updated.screen != tuiScreenTranscript {
		t.Fatalf("expected transcript screen, got %q", updated.screen)
	}
	updated.closeTranscriptScreen()
	if updated.input.Value() != "draft prompt" {
		t.Fatalf("expected draft to survive transcript round-trip, got %q", updated.input.Value())
	}
}

func TestCtrlRStartsPromptHistorySearchWithoutOpeningTranscript(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.input.SetValue("draft prompt")
	model.messageStore.Reset([]protocol.TranscriptEntry{
		{
			ID:        "hist_1",
			Type:      protocol.TranscriptEntryUser,
			Title:     "You",
			Body:      "summarize workspace",
			InputMode: protocol.TranscriptInputPrompt,
		},
	})

	gotModel, cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if cmd == nil {
		t.Fatalf("expected focus command for history search")
	}

	updated := gotModel.(*tuiModel)
	if updated.screen != tuiScreenMain {
		t.Fatalf("expected main screen, got %q", updated.screen)
	}
	if updated.focus != tuiFocusHistorySearch {
		t.Fatalf("expected history search focus, got %q", updated.focus)
	}
	if updated.input.Value() != "draft prompt" {
		t.Fatalf("expected draft to remain in input, got %q", updated.input.Value())
	}
	if len(updated.historyMatches) != 1 {
		t.Fatalf("expected one history match, got %+v", updated.historyMatches)
	}
}

func TestAltPOpensProviderPicker(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	gotModel, cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}, Alt: true})
	if cmd == nil {
		t.Fatalf("expected provider discovery command")
	}

	updated := gotModel.(*tuiModel)
	if updated.focus != tuiFocusModal {
		t.Fatalf("expected modal focus, got %q", updated.focus)
	}
	if updated.modal.Kind != tuiModalProviders {
		t.Fatalf("expected provider modal, got %q", updated.modal.Kind)
	}
}

func TestModalDrawerLimitsRowsOnShortTerminal(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.height = 18
	model.width = 88
	model.modal = tuiModalState{
		Kind:  tuiModalCommands,
		Title: "Command Palette",
	}
	for i := 0; i < 20; i++ {
		model.modal.Visible = append(model.modal.Visible, tuiChoice{
			Label:  fmt.Sprintf("/cmd-%02d", i),
			Detail: "detail",
		})
	}

	drawer := model.renderModalBox()
	if lines := strings.Split(drawer, "\n"); len(lines) > 12 {
		t.Fatalf("expected short terminal drawer to stay compact, got %d lines: %q", len(lines), drawer)
	}
}

func TestHistorySearchAcceptRestoresSelectedPrompt(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.input.SetValue("draft prompt")
	model.messageStore.Reset([]protocol.TranscriptEntry{
		{
			ID:        "hist_1",
			Type:      protocol.TranscriptEntryUser,
			Title:     "You",
			Body:      "summarize workspace",
			InputMode: protocol.TranscriptInputPrompt,
		},
	})
	model.openHistorySearch()
	model.historyIn.SetValue("workspace")
	model.refreshHistorySearch()

	gotModel, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := gotModel.(*tuiModel)
	if updated.focus != tuiFocusInput {
		t.Fatalf("expected input focus, got %q", updated.focus)
	}
	if updated.input.Value() != "summarize workspace" {
		t.Fatalf("expected selected history prompt, got %q", updated.input.Value())
	}
}

func TestTranscriptSlashSearchDoesNotUseMainHistorySearch(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.openTranscriptScreen(false)

	gotModel, cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if cmd == nil {
		t.Fatalf("expected focus command for transcript search")
	}

	updated := gotModel.(*tuiModel)
	if updated.focus != tuiFocusTranscriptSearch {
		t.Fatalf("expected transcript search focus, got %q", updated.focus)
	}
	if updated.screen != tuiScreenTranscript {
		t.Fatalf("expected transcript screen, got %q", updated.screen)
	}
}

func TestTranscriptSearchCommitKeepsHighlightForNextNavigation(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.messageStore.Reset([]protocol.TranscriptEntry{
		{ID: "1", Type: protocol.TranscriptEntryUser, Title: "You", Body: "first prompt"},
		{ID: "2", Type: protocol.TranscriptEntryAssistant, Title: "Assistant", Body: "workspace summary"},
	})
	model.openTranscriptScreen(false)
	model.openTranscriptSearch()
	model.searchIn.SetValue("workspace")
	model.refreshTranscriptSearch()

	gotModel, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := gotModel.(*tuiModel)
	if updated.focus != tuiFocusTranscript {
		t.Fatalf("expected transcript focus after committing search, got %q", updated.focus)
	}
	if updated.transcriptScreen.MatchCount() != 1 {
		t.Fatalf("expected search match to remain after commit, got %d", updated.transcriptScreen.MatchCount())
	}
	if got := updated.renderTranscriptContent(80); !containsString(got, "› ") {
		t.Fatalf("expected committed transcript search to keep highlight, got %q", got)
	}
}

func TestTranscriptScreenFreezesEntriesOnEntry(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.messageStore.Reset([]protocol.TranscriptEntry{
		{ID: "1", Type: protocol.TranscriptEntryUser, Body: "first", InputMode: protocol.TranscriptInputPrompt},
	})
	model.openTranscriptScreen(false)
	model.appendTranscript(protocol.TranscriptEntry{
		ID:        "2",
		SessionID: "sess_test",
		Type:      protocol.TranscriptEntryAssistant,
		Title:     "Assistant",
		Body:      "second",
	}, false)

	rendered := model.renderTranscriptContent(80)
	if !containsString(rendered, "first") {
		t.Fatalf("expected frozen transcript to include first entry, got %q", rendered)
	}
	if containsString(rendered, "second") {
		t.Fatalf("did not expect frozen transcript to include later entry, got %q", rendered)
	}

	model.closeTranscriptScreen()
	if len(model.transcriptEntries()) != 2 {
		t.Fatalf("expected live transcript after close")
	}
}

func TestSuggestionsDoNotReduceTimelineHeight(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	baseHeight := model.timeline.Height

	model.suggestions = []tuiSuggestion{
		{Label: "one", Detail: "detail"},
		{Label: "two", Detail: "detail"},
		{Label: "three", Detail: "detail"},
	}
	model.reflow()

	if model.timeline.Height != baseHeight {
		t.Fatalf("expected timeline height to remain %d, got %d", baseHeight, model.timeline.Height)
	}
	view := model.renderMainScreen()
	if !containsString(view, "one") || !containsString(view, "two") {
		t.Fatalf("expected overlay suggestions in main screen, got %q", view)
	}
}

func TestWelcomeIsLowNoiseSingleLine(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	rendered := model.renderTimelineContent(80)
	lines := strings.Split(rendered, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one low-noise welcome line, got %d lines: %q", len(lines), rendered)
	}
	if containsString(rendered, "papersilm ·") {
		t.Fatalf("did not expect welcome to duplicate header wordmark, got %q", rendered)
	}
	if containsString(rendered, "Workspace") {
		t.Fatalf("did not expect welcome to repeat workspace metadata, got %q", rendered)
	}
	if !containsString(rendered, "Ask about the current workspace") {
		t.Fatalf("expected low-noise workspace hint, got %q", rendered)
	}
}

func TestUnreadRendersAsPillNotFooterStatus(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.autoScroll = false
	model.unread = 2
	view := model.renderMainScreen()
	if !containsString(view, "2 new messages") {
		t.Fatalf("expected new messages pill in main screen, got %q", view)
	}
	footer := model.renderFooter()
	if containsString(footer, "2 new") {
		t.Fatalf("did not expect unread count in footer meta, got %q", footer)
	}
}

func TestDetachedScrollShowsJumpPillAndStickyPrompt(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.appendTranscript(protocol.TranscriptEntry{
		ID:        "u1",
		SessionID: "sess_test",
		Type:      protocol.TranscriptEntryUser,
		Body:      "summarize the current workspace",
		CreatedAt: time.Now().UTC(),
	}, false)
	model.autoScroll = false
	model.unread = 0
	model.reflow()

	view := model.renderMainScreen()
	if !containsString(view, "Jump to bottom") {
		t.Fatalf("expected detached scroll pill, got %q", view)
	}
	if !containsString(view, "› summarize the current workspace") {
		t.Fatalf("expected sticky prompt header, got %q", view)
	}
}

func TestPaneOverlayPositionIgnoresSuggestions(t *testing.T) {
	t.Parallel()

	base := newTestTUIModel()
	base.openPane("Workspace", "details")
	base.reflow()
	baseView := base.renderMainScreen()
	baseLine := lineIndexContaining(baseView, "Workspace")
	if baseLine < 0 {
		t.Fatalf("expected pane title in base view, got %q", baseView)
	}

	withSuggestions := newTestTUIModel()
	withSuggestions.openPane("Workspace", "details")
	withSuggestions.suggestions = []tuiSuggestion{
		{Label: "/help", Detail: "Show slash commands"},
		{Label: "/model", Detail: "Open provider/model picker"},
	}
	withSuggestions.reflow()
	view := withSuggestions.renderMainScreen()
	gotLine := lineIndexContaining(view, "Workspace")
	if gotLine != baseLine {
		t.Fatalf("expected pane line to stay at %d with suggestions, got %d: %q", baseLine, gotLine, view)
	}
}

func TestTaskPanePlainOutputDoesNotUseMarkdownRenderer(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.paneBody = "Task Board: plan_1\n- workspace_task [awaiting_approval] hello | actions=Inspect, Approve, Reject"
	rendered := model.renderPaneBody(80)
	if strings.Contains(rendered, "\x1b[") {
		t.Fatalf("expected plain task pane without ANSI markdown noise, got %q", rendered)
	}
	if !containsString(rendered, "workspace_task") {
		t.Fatalf("expected task pane content, got %q", rendered)
	}
}

func TestEscClosesSuggestionBeforePane(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.openPane("Workspace", "details")
	model.suggestions = []tuiSuggestion{{Label: "/help", Detail: "Show slash commands"}}
	model.sel = 0

	gotModel, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := gotModel.(*tuiModel)
	if len(updated.suggestions) != 0 {
		t.Fatalf("expected Esc to close suggestions first, got %+v", updated.suggestions)
	}
	if !updated.paneVisible {
		t.Fatalf("expected pane to remain visible after first Esc")
	}

	gotModel, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated = gotModel.(*tuiModel)
	if updated.paneVisible {
		t.Fatalf("expected second Esc to close pane")
	}
}

func TestIdleInputDoesNotShowRecipeSuggestions(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.input.SetValue("")
	model.refreshSuggestions()

	if len(model.suggestions) != 0 {
		t.Fatalf("expected idle input to hide suggestions, got %+v", model.suggestions)
	}
}

func TestEmptyPromptPlaceholderStaysSingleLine(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.width = 42
	model.input.SetValue("")
	model.reflow()

	rendered := model.renderInput()
	if !containsString(rendered, "Ask about workspace") {
		t.Fatalf("expected compact placeholder, got %q", rendered)
	}
	if strings.Count(rendered, "›") != 1 {
		t.Fatalf("expected one prompt marker in placeholder, got %q", rendered)
	}
}

func TestShortPromptInputStaysSingleLine(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.input.SetValue("/")
	model.reflow()

	rendered := model.renderInput()
	if strings.Count(rendered, "›") != 1 {
		t.Fatalf("expected one prompt marker for short input, got %q", rendered)
	}
	if strings.Count(rendered, "\n") != 0 {
		t.Fatalf("expected single prompt row without form divider, got %q", rendered)
	}
}

func TestPaneDoesNotReduceTimelineHeight(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	baseHeight := model.timeline.Height

	model.openPane("Workspace", "details")
	model.reflow()

	if model.timeline.Height != baseHeight {
		t.Fatalf("expected pane overlay to keep timeline height %d, got %d", baseHeight, model.timeline.Height)
	}
	view := model.renderMainScreen()
	if !containsString(view, "Workspace") || !containsString(view, "details") {
		t.Fatalf("expected pane overlay in main screen, got %q", view)
	}
}

func TestModalOverlayPreservesBaseScreen(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	base := model.renderMainScreen()
	model.modal = tuiModalState{
		Kind:    tuiModalCommands,
		Title:   "Command Palette",
		Message: "Filter commands.",
	}

	view := model.renderModalOver(base)
	if !containsString(view, "Command Palette") {
		t.Fatalf("expected modal title, got %q", view)
	}
	if !containsString(view, "papersilm") {
		t.Fatalf("expected base screen to remain behind modal, got %q", view)
	}
}

func TestModalRendersAsBottomDrawer(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.width = 88
	model.height = 24
	model.modal = tuiModalState{
		Kind:    tuiModalCommands,
		Title:   "Command Palette",
		Message: "Filter commands.",
		Visible: []tuiChoice{
			{Label: "/help", Detail: "Show commands"},
			{Label: "/model", Detail: "Open provider/model picker"},
		},
	}

	view := model.renderModalOver(model.renderMainScreen())
	lines := strings.Split(view, "\n")
	drawerLine := -1
	for i, line := range lines {
		if containsString(line, "───") {
			drawerLine = i
			break
		}
	}
	if drawerLine < 0 {
		t.Fatalf("expected bottom drawer divider, got %q", view)
	}
	if drawerLine < model.height/2 {
		t.Fatalf("expected drawer near bottom, line=%d view=%q", drawerLine, view)
	}
	if containsString(view, "┌") || containsString(view, "└") {
		t.Fatalf("did not expect centered box borders in drawer modal: %q", view)
	}
}

func TestSuggestionsUsePromptOverlayRows(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.suggestions = []tuiSuggestion{
		{Label: "/help", Detail: "Show slash commands"},
		{Label: "/model", Detail: "Open provider/model picker"},
	}
	model.sel = 0

	rendered := model.renderSuggestions()
	if !containsString(rendered, "+ /help") {
		t.Fatalf("expected selected suggestion marker, got %q", rendered)
	}
	if !containsString(rendered, "+ /help – Show slash commands") {
		t.Fatalf("expected compact Claude-style suggestion row, got %q", rendered)
	}
	if containsString(rendered, "┌") || containsString(rendered, "└") {
		t.Fatalf("did not expect boxed suggestion overlay, got %q", rendered)
	}
}

func TestThemeCommandIsUINavigationOnlyInTUI(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.runtime.cfg.BaseDir = t.TempDir()
	model.input.SetValue("/theme light")

	gotModel, cmd := model.submitInput()
	if cmd != nil {
		t.Fatalf("expected /theme to be handled synchronously")
	}
	updated := gotModel.(*tuiModel)
	if updated.input.Value() != "" {
		t.Fatalf("expected /theme to consume input, got %q", updated.input.Value())
	}
	if updated.messageStore.Len() != 0 {
		t.Fatalf("expected /theme to stay out of transcript, got %+v", updated.transcriptEntries())
	}
	if updated.styles.theme != config.ThemeLight {
		t.Fatalf("expected light theme, got %q", updated.styles.theme)
	}
	loaded, err := config.Load(config.ConfigPath(updated.runtime.cfg.BaseDir))
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if loaded.Theme != config.ThemeLight {
		t.Fatalf("expected persisted light theme, got %q", loaded.Theme)
	}
}

func TestStartupEventDrainPreventsHydrateReplay(t *testing.T) {
	t.Parallel()

	sink := newTUIEventSink(4)
	runtime := &tuiRuntimeManager{sink: sink}
	if err := sink.Emit(protocol.StreamEvent{Type: protocol.EventInit, Message: "session created"}); err != nil {
		t.Fatalf("emit startup event: %v", err)
	}

	runtime.drainPendingStartupEvents()

	select {
	case event := <-sink.ch:
		t.Fatalf("expected startup event queue to drain, got %+v", event)
	default:
	}
}

func TestCompactFooterStaysWithinThreeLines(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.height = 22
	model.unread = 3
	model.setMainStatus("Running task...")
	model.reflow()

	lines := strings.Split(model.renderFooter(), "\n")
	if len(lines) > 3 {
		t.Fatalf("expected compact footer to use at most 3 lines, got %d: %q", len(lines), model.renderFooter())
	}
}

func TestHeaderStaysSingleLine(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.width = 80
	model.workspaceName = "a-very-long-workspace-name-that-should-not-wrap"
	model.snapshot.Meta.SessionID = "sess_very_long_identifier_that_should_truncate"
	model.snapshot.Meta.Model = "gpt-5.4-long-model-name"
	model.reflow()

	header := model.renderHeader()
	if lines := strings.Split(header, "\n"); len(lines) != 1 {
		t.Fatalf("expected header to stay on one line, got %d lines: %q", len(lines), header)
	}
	if got := lipgloss.Width(header); got > model.width-2 {
		t.Fatalf("expected header width <= %d, got %d: %q", model.width-2, got, header)
	}
}

func TestHeaderDropsWorkspaceOnNarrowWidth(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.width = 58
	model.workspaceName = "papersilm"
	model.reflow()

	header := model.renderHeader()
	if strings.Contains(header, "papersilm · papersilm") {
		t.Fatalf("expected narrow header to avoid duplicated app/workspace label, got %q", header)
	}
}

func TestFooterMetaStaysSingleLineWithLongWorkspace(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.width = 80
	model.workspaceName = ""
	model.workspaceDisplayPath = "/private/var/folders/really/long/generated/path/that/previously/wrapped/papersilm-workspace"
	model.snapshot.Meta.Model = "gpt-5.4-long-model-name"
	model.reflow()

	footer := model.renderFooter()
	lines := strings.Split(footer, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected footer to render compact single-line meta, got %d lines: %q", len(lines), footer)
	}
	if got := lipgloss.Width(lines[0]); got > model.width-2 {
		t.Fatalf("expected footer meta width <= %d, got %d: %q", model.width-2, got, lines[0])
	}
	if containsString(lines[0], "/private/var/folders") || containsString(lines[0], "gpt-5.4-long-model-name") {
		t.Fatalf("expected footer to drop low-priority metadata, got %q", lines[0])
	}
}

func TestFooterDropsLowPriorityMetadataOnNarrowWidth(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.width = 58
	model.snapshot.Sources = []protocol.PaperRef{{PaperID: "source_1"}}
	model.workspaceName = "papersilm-workspace"
	model.reflow()

	meta := strings.Split(model.renderFooter(), "\n")[0]
	if containsString(meta, "sources") || containsString(meta, "papersilm-workspace") || containsString(meta, "dark") {
		t.Fatalf("expected narrow footer to drop low-priority metadata, got %q", meta)
	}
	if containsString(meta, "confirm") {
		t.Fatalf("did not expect default confirm mode to add footer noise, got %q", meta)
	}
}

func TestHintsCanBeHiddenWithoutRemovingFooterMeta(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	visible := model.renderFooter()
	if !containsString(visible, "? for shortcuts") {
		t.Fatalf("expected compact footer shortcut hint, got %q", visible)
	}
	if containsString(visible, "Enter send") || containsString(visible, "Ctrl+K") {
		t.Fatalf("expected default footer hint to stay collapsed, got %q", visible)
	}

	model.setHintsVisible(false)
	hidden := model.renderFooter()
	if containsString(hidden, "? for shortcuts") {
		t.Fatalf("expected hints line to disappear, got %q", hidden)
	}
	if containsString(hidden, "confirm") {
		t.Fatalf("did not expect default confirm mode to add footer noise, got %q", hidden)
	}
}

func TestFooterHintsSuppressWhileTyping(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.input.SetValue("draft prompt")

	footer := model.renderFooter()
	if containsString(footer, "? for shortcuts") {
		t.Fatalf("expected shortcuts to be suppressed while typing, got %q", footer)
	}
	if strings.TrimSpace(footer) != "" {
		t.Fatalf("expected default footer to stay quiet while typing, got %q", footer)
	}
}

func TestHintsToggleKeysAreRecognized(t *testing.T) {
	t.Parallel()

	if !isHintsToggleKey("ctrl+/") {
		t.Fatalf("expected ctrl+/ to toggle hints")
	}
	if !isHintsToggleKey("ctrl+_") {
		t.Fatalf("expected ctrl+_ to toggle hints")
	}
	if isHintsToggleKey("ctrl+k") {
		t.Fatalf("did not expect ctrl+k to toggle hints")
	}
}

func newTestTUIModel() *tuiModel {
	runtime := &tuiRuntimeManager{cfg: config.Default()}
	model := newTUIModel(context.Background(), runtime, protocol.SessionSnapshot{
		Meta: protocol.SessionMeta{
			SessionID:       "sess_test",
			PermissionMode:  protocol.PermissionModeConfirm,
			Language:        "zh",
			Style:           "distill",
			ProviderProfile: config.DefaultProviderProfile,
			Model:           "gpt-test",
		},
	})
	model.width = 100
	model.height = 30
	model.ready = true
	model.reflow()
	return model
}

func containsString(text, want string) bool {
	return len(text) > 0 && len(want) > 0 && strings.Contains(text, want)
}

func hasKeyContext(contexts []tuiui.KeyContext, want tuiui.KeyContext) bool {
	for _, context := range contexts {
		if context == want {
			return true
		}
	}
	return false
}

func lineIndexContaining(text, want string) int {
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, want) {
			return i
		}
	}
	return -1
}
