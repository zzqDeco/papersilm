package tui

import (
	"fmt"
	"strings"
	"testing"
)

func TestMessageViewportCachesAndReplaces(t *testing.T) {
	t.Parallel()

	var viewport MessageViewport
	renderCalls := 0
	content := viewport.Content(80, 2, func(index int, width int) string {
		renderCalls++
		return fmt.Sprintf("%d/%d", index, width)
	})
	if content != "0/80\n\n1/80" {
		t.Fatalf("unexpected content: %q", content)
	}
	again := viewport.Content(80, 2, func(index int, width int) string {
		renderCalls++
		return "rerendered"
	})
	if again != content || renderCalls != 2 {
		t.Fatalf("expected cached content, got content=%q calls=%d", again, renderCalls)
	}

	replaced, ok := viewport.ReplaceLast(80, 2, "last")
	if !ok || replaced != "0/80\n\nlast" {
		t.Fatalf("expected replace last, ok=%v content=%q", ok, replaced)
	}
	appended, ok := viewport.Append(80, 2, "third")
	if !ok || !strings.Contains(appended, "third") {
		t.Fatalf("expected append, ok=%v content=%q", ok, appended)
	}
}
