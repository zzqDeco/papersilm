package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderFooterChromeKeepsMetaSingleLine(t *testing.T) {
	t.Parallel()

	rendered := RenderFooterChrome(FooterChrome{
		Width:       72,
		MetaLeft:    "confirm · running",
		MetaRight:   "provider/model",
		Hints:       "Enter send · Ctrl+K commands",
		ShowHints:   true,
		FooterStyle: lipgloss.NewStyle(),
		LeftStyle:   lipgloss.NewStyle(),
		RightStyle:  lipgloss.NewStyle(),
		HintStyle:   lipgloss.NewStyle(),
	})
	lines := strings.Split(rendered, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected footer meta and hint to share one line, got %d lines: %q", len(lines), rendered)
	}
	if got := lipgloss.Width(lines[0]); got > 72 {
		t.Fatalf("expected meta width <= 48, got %d: %q", got, lines[0])
	}
	if !strings.Contains(rendered, "Enter send") {
		t.Fatalf("expected inline hint, got %q", rendered)
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
