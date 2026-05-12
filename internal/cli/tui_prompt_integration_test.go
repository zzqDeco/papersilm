package cli

import (
	"testing"

	"github.com/zzqDeco/papersilm/internal/cli/tui"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestProgrammaticPromptMutationsSyncController(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.setPromptValue("draft")

	model.applySuggestion(tuiSuggestion{Label: "/model", Insert: "/model"})
	if got := model.promptController.Value(); got != "/model" {
		t.Fatalf("expected suggestion to sync prompt controller, got %q", got)
	}
	if got := model.promptController.Mode(); got != tui.PromptModeCommand {
		t.Fatalf("expected suggestion to switch prompt mode, got %q", got)
	}

	model.modal = tuiModalState{
		Kind:      tuiModalCommands,
		Visible:   []tuiChoice{{Label: "/tasks", Value: "/tasks"}},
		Selection: 0,
	}
	if _, _ = model.commitModalSelection(); model.promptController.Value() != "/tasks" {
		t.Fatalf("expected command palette selection to sync prompt controller, got %q", model.promptController.Value())
	}

	model.historyMatches = []protocol.TranscriptEntry{{
		ID:        "hist_1",
		Type:      protocol.TranscriptEntryUser,
		Body:      "summarize workspace",
		InputMode: protocol.TranscriptInputPrompt,
	}}
	model.historySelection = 0
	model.historyDraft = "draft"
	model.focus = tuiFocusHistorySearch
	_ = model.acceptHistorySearch(false)
	if got := model.promptController.Value(); got != "summarize workspace" {
		t.Fatalf("expected accepted history prompt to sync prompt controller, got %q", got)
	}
	if got := model.promptController.Mode(); got != tui.PromptModePrompt {
		t.Fatalf("expected accepted history prompt mode, got %q", got)
	}
}
