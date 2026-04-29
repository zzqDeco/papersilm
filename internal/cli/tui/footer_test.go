package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderFooterChromeKeepsMetaSingleLine(t *testing.T) {
	t.Parallel()

	rendered := RenderFooterChrome(FooterChrome{
		Width:       48,
		MetaLeft:    "confirm · running · long status that should truncate",
		MetaRight:   "provider/model · workspace · dark",
		Hints:       "Enter send · Ctrl+K commands",
		ShowHints:   true,
		FooterStyle: lipgloss.NewStyle(),
		LeftStyle:   lipgloss.NewStyle(),
		RightStyle:  lipgloss.NewStyle(),
		HintStyle:   lipgloss.NewStyle(),
	})
	lines := strings.Split(rendered, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected meta + hints, got %d lines: %q", len(lines), rendered)
	}
	if got := lipgloss.Width(lines[0]); got > 48 {
		t.Fatalf("expected meta width <= 48, got %d: %q", got, lines[0])
	}
}

func TestRenderFooterChromeSearchSuppressesHints(t *testing.T) {
	t.Parallel()

	rendered := RenderFooterChrome(FooterChrome{
		Width:       60,
		MetaLeft:    "confirm",
		SearchLine:  "search prompts: workspace",
		Hints:       "Enter send",
		ShowHints:   true,
		FooterStyle: lipgloss.NewStyle(),
		LeftStyle:   lipgloss.NewStyle(),
		RightStyle:  lipgloss.NewStyle(),
		HintStyle:   lipgloss.NewStyle(),
	})
	if strings.Contains(rendered, "Enter send") {
		t.Fatalf("expected search footer to suppress hints, got %q", rendered)
	}
	if !strings.Contains(rendered, "search prompts") {
		t.Fatalf("expected search line, got %q", rendered)
	}
}
