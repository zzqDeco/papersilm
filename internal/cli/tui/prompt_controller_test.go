package tui

import "testing"

func TestPromptControllerHistoryRestoresDraftAndFiltersMode(t *testing.T) {
	t.Parallel()

	prompt := NewPromptController()
	prompt.SetHistory([]PromptHistoryEntry{
		{Value: "latest prompt", Mode: PromptModePrompt},
		{Value: "/help", Mode: PromptModeCommand},
		{Value: "older prompt", Mode: PromptModePrompt},
	})
	prompt.SetValue("draft")

	if !prompt.HistoryPrev() || prompt.Value() != "latest prompt" {
		t.Fatalf("expected latest prompt history, got %q", prompt.Value())
	}
	if !prompt.HistoryPrev() || prompt.Value() != "older prompt" {
		t.Fatalf("expected filtered older prompt history, got %q", prompt.Value())
	}
	if !prompt.HistoryNext() || prompt.Value() != "latest prompt" {
		t.Fatalf("expected next history, got %q", prompt.Value())
	}
	if !prompt.HistoryNext() || prompt.Value() != "draft" {
		t.Fatalf("expected draft restore, got %q", prompt.Value())
	}
}

func TestDetectPromptMode(t *testing.T) {
	t.Parallel()

	if got := DetectPromptMode("/model"); got != PromptModeCommand {
		t.Fatalf("expected command mode, got %q", got)
	}
	if got := DetectPromptMode("!ls"); got != PromptModeShell {
		t.Fatalf("expected shell mode, got %q", got)
	}
	if got := DetectPromptMode("summarize"); got != PromptModePrompt {
		t.Fatalf("expected prompt mode, got %q", got)
	}
}
