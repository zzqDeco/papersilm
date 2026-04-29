package tui

import (
	"strings"
	"testing"
)

func TestOverlayManagerSeparatesPromptAndDrawer(t *testing.T) {
	t.Parallel()

	var manager OverlayManager
	manager.SetPrompt(PromptOverlay{Kind: OverlaySuggestions, Rows: []ListRow{{Label: "/help"}}})
	manager.SetDrawer(DrawerOverlay{Kind: OverlayPalette, Title: "Commands"})

	if prompt, ok := manager.Prompt(); !ok || prompt.Kind != OverlaySuggestions {
		t.Fatalf("expected prompt overlay, got %+v ok=%v", prompt, ok)
	}
	if drawer, ok := manager.Drawer(); !ok || drawer.Kind != OverlayPalette {
		t.Fatalf("expected drawer overlay, got %+v ok=%v", drawer, ok)
	}

	manager.ClearPrompt()
	if _, ok := manager.Prompt(); ok {
		t.Fatalf("expected prompt overlay to clear")
	}
	if _, ok := manager.Drawer(); !ok {
		t.Fatalf("expected drawer overlay to remain after clearing prompt")
	}
}

func TestRenderPromptOverlay(t *testing.T) {
	t.Parallel()

	rendered := RenderPromptOverlay(PromptOverlay{
		Kind: OverlaySuggestions,
		Rows: []ListRow{
			{Label: "/model", Detail: "Pick model", Selected: true},
		},
	}, 40)
	if rendered == "" || !containsAll(rendered, "+ /model", "Pick model") {
		t.Fatalf("unexpected prompt overlay: %q", rendered)
	}
}

func containsAll(value string, wants ...string) bool {
	for _, want := range wants {
		if !strings.Contains(value, want) {
			return false
		}
	}
	return true
}
