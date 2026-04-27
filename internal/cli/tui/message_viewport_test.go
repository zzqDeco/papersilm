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

func TestMessageViewportCachesByStableKey(t *testing.T) {
	t.Parallel()

	var viewport MessageViewport
	calls := 0
	content := viewport.ContentByKey(80, []string{"a", "b"}, func(index int, width int) string {
		calls++
		return fmt.Sprintf("%d/%d", index, width)
	})
	if content != "0/80\n\n1/80" || calls != 2 {
		t.Fatalf("unexpected first render content=%q calls=%d", content, calls)
	}

	reordered := viewport.ContentByKey(80, []string{"b", "a"}, func(index int, width int) string {
		calls++
		return "rerendered"
	})
	if reordered != "1/80\n\n0/80" || calls != 2 {
		t.Fatalf("expected keyed cache reuse on reorder, content=%q calls=%d", reordered, calls)
	}

	replaced, ok := viewport.ReplaceLastByKey(80, "a", "last")
	if !ok || replaced != "1/80\n\nlast" {
		t.Fatalf("expected keyed replace, ok=%v content=%q", ok, replaced)
	}
}
