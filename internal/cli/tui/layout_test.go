package tui

import (
	"strings"
	"testing"
)

func TestRenderBottomDrawerUsesSingleDivider(t *testing.T) {
	t.Parallel()

	rendered := RenderBottomDrawer(Drawer{
		Width:   32,
		Title:   "Command Palette",
		Message: "Filter commands",
		Rows: []ListRow{
			{Label: "/help", Detail: "Show slash commands", Selected: true},
		},
	})

	if !strings.Contains(rendered, strings.Repeat("▔", 32)) {
		t.Fatalf("expected single divider, got %q", rendered)
	}
	if strings.ContainsAny(rendered, "┌┐└┘") {
		t.Fatalf("expected drawer without box corners, got %q", rendered)
	}
	if !strings.Contains(rendered, "+ /help") {
		t.Fatalf("expected selected row marker, got %q", rendered)
	}
}

func TestOverlayBottomKeepsBaseHeight(t *testing.T) {
	t.Parallel()

	base := strings.Join([]string{"a", "b", "c", "d"}, "\n")
	block := strings.Join([]string{"x", "y"}, "\n")

	overlaid := OverlayBottom(base, block, 8)
	if got, want := len(strings.Split(overlaid, "\n")), len(strings.Split(base, "\n")); got != want {
		t.Fatalf("expected overlay to keep base height %d, got %d: %q", want, got, overlaid)
	}
	if !strings.Contains(overlaid, "x") || !strings.Contains(overlaid, "y") {
		t.Fatalf("expected block content in overlay, got %q", overlaid)
	}
}

func TestOverlayBottomWithPeekKeepsTranscriptPeek(t *testing.T) {
	t.Parallel()

	base := strings.Join([]string{"peek1", "peek2", "old", "input"}, "\n")
	block := strings.Join([]string{"drawer", "row1", "row2", "row3"}, "\n")

	overlaid := OverlayBottomWithPeek(base, block, 16, 2)
	lines := strings.Split(overlaid, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected overlay to keep base height, got %d: %q", len(lines), overlaid)
	}
	if !strings.Contains(lines[0], "peek1") || !strings.Contains(lines[1], "peek2") {
		t.Fatalf("expected first two transcript rows to remain visible, got %q", overlaid)
	}
	if !strings.Contains(overlaid, "drawer") || strings.Contains(overlaid, "row3") {
		t.Fatalf("expected clipped bottom drawer, got %q", overlaid)
	}
}

func TestRenderFullscreenLayoutSlots(t *testing.T) {
	t.Parallel()

	rendered := RenderFullscreenLayout(FullscreenLayout{
		Width:         40,
		Header:        "header",
		StickyHeader:  "sticky prompt",
		Scrollable:    strings.Join([]string{"m1", "m2", "m3", "m4"}, "\n"),
		Bottom:        strings.Join([]string{"input", "footer"}, "\n"),
		Pane:          strings.Join([]string{"pane", "details"}, "\n"),
		PromptOverlay: "suggestion",
		ScrollPill:    "jump",
	})

	for _, want := range []string{"header", "sticky prompt", "pane", "suggestion", "jump", "input", "footer"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in fullscreen layout, got %q", want, rendered)
		}
	}
	if got := len(strings.Split(rendered, "\n")); got != 8 {
		t.Fatalf("expected layout to keep base height, got %d lines: %q", got, rendered)
	}
}
