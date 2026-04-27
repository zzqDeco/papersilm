package tui

import (
	"testing"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestMessagePipelineHidesAmbientAndKeepsPrimary(t *testing.T) {
	t.Parallel()

	pipeline := NewMessagePipeline()
	if _, ok := pipeline.Project(protocol.TranscriptEntry{
		ID:   "session",
		Type: protocol.TranscriptEntrySystem,
		Body: "session loaded",
	}); ok {
		t.Fatalf("expected ambient system event to stay out of main timeline")
	}

	msg, ok := pipeline.Project(protocol.TranscriptEntry{
		ID:        "assistant",
		Type:      protocol.TranscriptEntryAssistant,
		Body:      "answer",
		CreatedAt: time.Now(),
	})
	if !ok || msg.Type != UIMessageAssistant || msg.Visibility != protocol.TranscriptVisibilityPrimary {
		t.Fatalf("unexpected assistant projection: %+v ok=%v", msg, ok)
	}
}

func TestMessagePipelineGroupsActivity(t *testing.T) {
	t.Parallel()

	pipeline := NewMessagePipeline()
	first, ok := pipeline.Project(protocol.TranscriptEntry{
		ID:           "read",
		Type:         protocol.TranscriptEntryProgress,
		Body:         "read README.md",
		Visibility:   protocol.TranscriptVisibilityActivity,
		Presentation: protocol.TranscriptPresentationGrouped,
	})
	if !ok || first.ID != "activity.grouped" || first.Type != UIMessageActivity {
		t.Fatalf("expected grouped activity, got %+v ok=%v", first, ok)
	}
	second, ok := pipeline.Project(protocol.TranscriptEntry{
		ID:           "search",
		Type:         protocol.TranscriptEntryProgress,
		Body:         "search TODO",
		Visibility:   protocol.TranscriptVisibilityActivity,
		Presentation: protocol.TranscriptPresentationGrouped,
	})
	if !ok || second.ID != "activity.grouped" || second.Body == first.Body {
		t.Fatalf("expected activity update, got first=%+v second=%+v ok=%v", first, second, ok)
	}
}
