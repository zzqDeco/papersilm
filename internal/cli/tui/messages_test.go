package tui

import (
	"testing"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestMessageStoreCopiesEntries(t *testing.T) {
	t.Parallel()

	entries := []protocol.TranscriptEntry{{ID: "1", Body: "first"}}
	store := NewMessageStore(entries)
	entries[0].Body = "mutated"

	got := store.Entries()
	if got[0].Body != "first" {
		t.Fatalf("expected defensive copy, got %+v", got)
	}
	got[0].Body = "changed"
	if store.Entries()[0].Body != "first" {
		t.Fatalf("expected entries copy to avoid external mutation")
	}
}

func TestMessageStoreFrozenAndHistory(t *testing.T) {
	t.Parallel()

	store := NewMessageStore([]protocol.TranscriptEntry{
		{ID: "1", Type: protocol.TranscriptEntryUser, Body: "first", InputMode: protocol.TranscriptInputPrompt},
		{ID: "2", Type: protocol.TranscriptEntryCommand, Body: "/help", InputMode: protocol.TranscriptInputCommand},
		{ID: "3", Type: protocol.TranscriptEntryAssistant, Body: "answer"},
		{ID: "4", Type: protocol.TranscriptEntryUser, Body: "second", InputMode: protocol.TranscriptInputPrompt},
	})

	if got := store.Frozen(2); len(got) != 2 || got[1].Body != "/help" {
		t.Fatalf("unexpected frozen entries: %+v", got)
	}
	history := store.History(protocol.TranscriptInputPrompt)
	if len(history) != 2 || history[0].Body != "second" || history[1].Body != "first" {
		t.Fatalf("unexpected prompt history: %+v", history)
	}
	if latest := store.LatestInput(); latest != "second" {
		t.Fatalf("expected latest input, got %q", latest)
	}
}
