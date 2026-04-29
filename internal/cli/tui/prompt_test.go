package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderPromptChromeKeepsInputVisible(t *testing.T) {
	t.Parallel()

	rendered := RenderPromptChrome(PromptChrome{
		Width:        40,
		Label:        "prompt · approval pending",
		Body:         "› draft text",
		LabelStyle:   lipgloss.NewStyle(),
		DividerStyle: lipgloss.NewStyle(),
		BodyStyle:    lipgloss.NewStyle(),
	})
	if !strings.Contains(rendered, "approval pending") || !strings.Contains(rendered, "draft text") {
		t.Fatalf("expected prompt label and body, got %q", rendered)
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected divider + body, got %d lines: %q", len(lines), rendered)
	}
	if got := lipgloss.Width(lines[0]); got > 40 {
		t.Fatalf("expected divider width <= 40, got %d: %q", got, lines[0])
	}
}
