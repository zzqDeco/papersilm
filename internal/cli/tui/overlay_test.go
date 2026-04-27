package tui

import "testing"

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
