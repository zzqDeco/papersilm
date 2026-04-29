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

func TestMessageViewportCachesDuplicateKeysByOccurrence(t *testing.T) {
	t.Parallel()

	var viewport MessageViewport
	calls := 0
	content := viewport.ContentByKey(80, []string{"activity.grouped", "answer", "activity.grouped"}, func(index int, width int) string {
		calls++
		return []string{"first activity", "answer", "second activity"}[index]
	})
	if content != "first activity\n\nanswer\n\nsecond activity" || calls != 3 {
		t.Fatalf("unexpected first render content=%q calls=%d", content, calls)
	}

	again := viewport.ContentByKey(80, []string{"activity.grouped", "answer", "activity.grouped"}, func(index int, width int) string {
		calls++
		return "rerendered"
	})
	if again != content || calls != 3 {
		t.Fatalf("expected duplicate-key cache to preserve each occurrence, content=%q calls=%d", again, calls)
	}
}

func TestMessageViewportAnchorSurvivesReflow(t *testing.T) {
	t.Parallel()

	var viewport MessageViewport
	viewport.ContentByKey(20, []string{"a", "b", "c"}, func(index int, width int) string {
		return []string{"one", "two\nline", "three"}[index]
	})
	anchor, ok := viewport.AnchorAt(2)
	if !ok || anchor.Key != "b" || anchor.Delta != 0 {
		t.Fatalf("unexpected anchor: %+v ok=%v", anchor, ok)
	}

	viewport.ContentByKey(10, []string{"a", "b", "c"}, func(index int, width int) string {
		return []string{"one\nwrap", "two\nline\nwrap", "three"}[index]
	})
	offset, ok := viewport.OffsetForAnchor(anchor)
	if !ok || offset != 3 {
		t.Fatalf("expected anchor to resolve after reflow, offset=%d ok=%v", offset, ok)
	}
}

func TestMessageViewportAnchorDistinguishesDuplicateKeys(t *testing.T) {
	t.Parallel()

	var viewport MessageViewport
	viewport.ContentByKey(20, []string{"activity.grouped", "answer", "activity.grouped"}, func(index int, width int) string {
		return []string{"first\nactivity", "answer", "second\nactivity"}[index]
	})
	anchor, ok := viewport.AnchorAt(5)
	if !ok || anchor.Key != "activity.grouped" || anchor.Occurrence != 1 || anchor.Delta != 0 {
		t.Fatalf("unexpected duplicate-key anchor: %+v ok=%v", anchor, ok)
	}

	viewport.ContentByKey(10, []string{"activity.grouped", "answer", "activity.grouped"}, func(index int, width int) string {
		return []string{"first\nactivity\nwrap", "answer", "second\nactivity\nwrap"}[index]
	})
	offset, ok := viewport.OffsetForAnchor(anchor)
	if !ok || offset != 6 {
		t.Fatalf("expected duplicate-key anchor to resolve to second activity, offset=%d ok=%v", offset, ok)
	}
}
