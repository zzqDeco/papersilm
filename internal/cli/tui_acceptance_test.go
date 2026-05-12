package cli

import (
	"testing"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestCCTUIAcceptanceMainScreenStaysContentFirst(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.items = nil
	model.messageViewport.Reset()

	model.appendTranscript(protocol.TranscriptEntry{
		ID:           "ambient",
		SessionID:    "sess_test",
		Type:         protocol.TranscriptEntrySystem,
		Title:        "Session",
		Body:         "session created",
		Visibility:   protocol.TranscriptVisibilityAmbient,
		Presentation: protocol.TranscriptPresentationHidden,
		CreatedAt:    time.Now().UTC(),
	}, false)
	model.appendTranscript(protocol.TranscriptEntry{
		ID:        "user",
		SessionID: "sess_test",
		Type:      protocol.TranscriptEntryUser,
		Title:     "You",
		Body:      "summarize this workspace",
		CreatedAt: time.Now().UTC(),
	}, false)
	model.appendTranscript(protocol.TranscriptEntry{
		ID:           "progress_start",
		SessionID:    "sess_test",
		Type:         protocol.TranscriptEntryProgress,
		Title:        "Progress",
		Body:         "node execution started",
		Visibility:   protocol.TranscriptVisibilityActivity,
		Presentation: protocol.TranscriptPresentationGrouped,
		CreatedAt:    time.Now().UTC(),
	}, false)
	model.appendTranscript(protocol.TranscriptEntry{
		ID:           "progress_done",
		SessionID:    "sess_test",
		Type:         protocol.TranscriptEntryProgress,
		Title:        "Progress",
		Body:         "node execution completed",
		Visibility:   protocol.TranscriptVisibilityActivity,
		Presentation: protocol.TranscriptPresentationGrouped,
		CreatedAt:    time.Now().UTC(),
	}, false)
	model.appendTranscript(protocol.TranscriptEntry{
		ID:        "assistant",
		SessionID: "sess_test",
		Type:      protocol.TranscriptEntryAssistant,
		Title:     "Assistant",
		Body:      "Workspace summary is ready.",
		CreatedAt: time.Now().UTC(),
	}, false)
	model.reflow()

	view := model.renderMainScreen()
	for _, forbidden := range []string{"session created", "node execution completed", "Assistant ·", "assistant ·", "You ·", "you ·"} {
		if containsString(view, forbidden) {
			t.Fatalf("expected CC-style main screen to hide %q, got:\n%s", forbidden, view)
		}
	}
	for _, required := range []string{"summarize this workspace", "Running plan", "Workspace summary is ready."} {
		if !containsString(view, required) {
			t.Fatalf("expected CC-style main screen to show %q, got:\n%s", required, view)
		}
	}
}

func TestCCTUIAcceptancePromptOverlayKeepsDraftVisible(t *testing.T) {
	t.Parallel()

	model := newTestTUIModel()
	model.setPromptValue("draft-visible-input-row")
	model.suggestions = []tuiSuggestion{
		{Label: "/model", Detail: "Open provider/model picker", Insert: "/model"},
		{Label: "/commands", Detail: "Open command palette", Insert: "/commands"},
	}
	model.sel = 0
	model.reflow()

	view := model.renderMainScreen()
	if !containsString(view, "draft-visible-input-row") {
		t.Fatalf("expected prompt draft to remain visible with suggestions, got:\n%s", view)
	}
	if !containsString(view, "/model") {
		t.Fatalf("expected suggestion overlay to render, got:\n%s", view)
	}
	if containsString(view, "prompt ·") || containsString(view, "── prompt") {
		t.Fatalf("expected prompt to avoid form-like chrome, got:\n%s", view)
	}
}
