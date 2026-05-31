package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
)

func TestMessageListModelOwnsViewportCacheAndUnread(t *testing.T) {
	t.Parallel()

	model := NewMessageListModel(viewport.New(20, 3), MessageViewport{})
	model.SetContentByKeyVersion(20, []string{"a", "b"}, []string{"1", "1"}, func(index int, width int) string {
		return []string{"one", "two"}[index]
	})
	if model.Viewport().TotalLineCount() == 0 {
		t.Fatalf("expected viewport content")
	}
	model.SetAutoScroll(false)
	model.IncrementUnread()
	if model.Unread() != 1 {
		t.Fatalf("expected unread increment, got %d", model.Unread())
	}
	model.GotoBottom()
	if !model.AutoScroll() || model.Unread() != 0 {
		t.Fatalf("expected goto bottom to reset unread, auto=%v unread=%d", model.AutoScroll(), model.Unread())
	}
}
