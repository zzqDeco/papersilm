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
		t.Fatalf("expected approval state and body, got %q", rendered)
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected approval state + body, got %d lines: %q", len(lines), rendered)
	}
	if strings.Contains(lines[0], "──") {
		t.Fatalf("did not expect prompt to render a form divider, got %q", rendered)
	}
	if got := lipgloss.Width(rendered); got > 80 {
		t.Fatalf("expected compact prompt chrome, got width %d: %q", got, rendered)
	}
}

func TestRenderPromptChromeIsSingleLineForNormalInput(t *testing.T) {
	t.Parallel()

	rendered := RenderPromptChrome(PromptChrome{
		Width:        40,
		Label:        "prompt",
		Body:         "› draft text",
		LabelStyle:   lipgloss.NewStyle(),
		DividerStyle: lipgloss.NewStyle(),
		BodyStyle:    lipgloss.NewStyle(),
	})
	if !strings.Contains(rendered, "› draft text") || strings.Contains(rendered, "prompt") || strings.Contains(rendered, "──") {
		t.Fatalf("expected normal prompt to avoid label chrome, got %q", rendered)
	}
	if strings.Contains(rendered, "\n") {
		t.Fatalf("expected normal prompt to stay on one line, got %q", rendered)
	}
}
