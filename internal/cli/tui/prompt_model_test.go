package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPromptModelOwnsTextareaAndController(t *testing.T) {
	t.Parallel()

	input := textarea.New()
	model := NewPromptModel(input, NewPromptController())

	updated, result := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if result.Event.Kind != ComponentEventChange {
		t.Fatalf("expected change event, got %+v", result.Event)
	}
	if updated.Value() != "h" {
		t.Fatalf("expected textarea value, got %q", updated.Value())
	}
	controller := updated.Controller()
	if controller.Value() != "h" {
		t.Fatalf("expected controller sync, got %q", controller.Value())
	}

	updated.SetValue("/model")
	if updated.Mode() != PromptModeCommand {
		t.Fatalf("expected command mode, got %q", updated.Mode())
	}
}

func TestPromptModelHistoryRestoresDraft(t *testing.T) {
	t.Parallel()

	input := textarea.New()
	model := NewPromptModel(input, NewPromptController())
	model.SetValue("draft")
	model.SetHistory([]PromptHistoryEntry{
		{Value: "latest", Mode: PromptModePrompt},
		{Value: "/help", Mode: PromptModeCommand},
	})

	if !model.HistoryPrev() || model.Value() != "latest" {
		t.Fatalf("expected prompt history, got %q", model.Value())
	}
	if !model.HistoryNext() || model.Value() != "draft" {
		t.Fatalf("expected draft restore, got %q", model.Value())
	}
}

func TestPromptModelAcceptHistoryEditPreservesEditedRecall(t *testing.T) {
	t.Parallel()

	input := textarea.New()
	model := NewPromptModel(input, NewPromptController())
	model.SetValue("draft")
	model.SetHistory([]PromptHistoryEntry{{Value: "history", Mode: PromptModePrompt}})
	if !model.HistoryPrev() {
		t.Fatal("expected history recall")
	}
	model.SetValue("history edited")
	model.AcceptHistoryEdit()
	model.CancelHistory()
	if model.Value() != "history edited" {
		t.Fatalf("expected edited history to stay, got %q", model.Value())
	}
}

func TestPromptModelResetValueClearsHistoryNavigation(t *testing.T) {
	t.Parallel()

	input := textarea.New()
	model := NewPromptModel(input, NewPromptController())
	model.SetValue("draft")
	model.SetHistory([]PromptHistoryEntry{{Value: "history", Mode: PromptModePrompt}})
	if !model.HistoryPrev() {
		t.Fatal("expected history recall")
	}
	model.ResetValue("")
	model.CancelHistory()
	if model.Value() != "" {
		t.Fatalf("expected reset prompt to stay empty, got %q", model.Value())
	}
}
