package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func plainTimelineRenderer() TimelineRenderer {
	style := lipgloss.NewStyle()
	return TimelineRenderer{
		Styles: TimelineStyles{
			Body:           style,
			UserShell:      style,
			UserLabel:      style,
			AssistantLabel: style,
			ApprovalShell:  style,
			ApprovalLabel:  style,
			SuccessShell:   style,
			SuccessLabel:   style,
			RejectionShell: style,
			RejectionLabel: style,
			ErrorShell:     style,
			ErrorLabel:     style,
			FooterMuted:    style,
			ProgressLine:   style,
			SystemLine:     style,
		},
	}
}

func TestRenderTimelineAssistantWithoutLogHeader(t *testing.T) {
	t.Parallel()

	rendered := RenderTimelineItem(TimelineItem{
		Kind:      TimelineItemAssistant,
		Title:     "Assistant",
		Body:      "answer",
		CreatedAt: time.Now(),
	}, 80, plainTimelineRenderer())
	if strings.Contains(rendered, "assistant ·") || !strings.Contains(rendered, "answer") {
		t.Fatalf("unexpected assistant rendering: %q", rendered)
	}
}

func TestRenderTimelineActivityIsCompact(t *testing.T) {
	t.Parallel()

	rendered := RenderTimelineItem(TimelineItem{
		Kind:    TimelineItemProgress,
		Subtype: "activity.grouped",
		Body:    "Inspecting workspace · 1 read",
	}, 80, plainTimelineRenderer())
	if !strings.Contains(rendered, "• Inspecting workspace") {
		t.Fatalf("expected compact activity row, got %q", rendered)
	}
}

func TestRenderTimelineRejectedCompact(t *testing.T) {
	t.Parallel()

	rendered := RenderTimelineItem(TimelineItem{
		Kind:    TimelineItemApproval,
		Subtype: TimelineSubtypeApprovalRejected,
		Title:   "Rejected",
		Body:    "User rejected tool use",
		Compact: true,
	}, 80, plainTimelineRenderer())
	if !strings.Contains(rendered, "Rejected") || strings.Contains(rendered, "\n") {
		t.Fatalf("expected compact rejection line, got %q", rendered)
	}
}
