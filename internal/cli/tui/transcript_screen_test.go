package tui

import (
	"testing"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestTranscriptScreenFreezesEntries(t *testing.T) {
	t.Parallel()

	var screen TranscriptScreen
	entries := []protocol.TranscriptEntry{{ID: "1", Body: "first"}}
	screen.Open(len(entries))
	entries = append(entries, protocol.TranscriptEntry{ID: "2", Body: "second"})

	frozen := screen.Entries(entries)
	if len(frozen) != 1 || frozen[0].Body != "first" {
		t.Fatalf("expected frozen entries, got %+v", frozen)
	}
	screen.Close()
	if live := screen.Entries(entries); len(live) != 2 {
		t.Fatalf("expected live entries after close, got %+v", live)
	}
}

func TestTranscriptScreenSearchState(t *testing.T) {
	t.Parallel()

	var screen TranscriptScreen
	entries := []protocol.TranscriptEntry{
		{ID: "1", Body: "alpha"},
		{ID: "2", Body: "beta alpha"},
	}
	screen.Open(len(entries))
	screen.OpenSearch()
	screen.RefreshSearch(entries, "alpha")
	if screen.Status() != "2 matches" || screen.MatchCount() != 2 {
		t.Fatalf("unexpected search status=%q count=%d", screen.Status(), screen.MatchCount())
	}
	if idx, ok := screen.SelectedEntryIndex(); !ok || idx != 0 {
		t.Fatalf("expected first match selected, idx=%d ok=%v", idx, ok)
	}
	if !screen.MoveSearch(1) {
		t.Fatalf("expected search move")
	}
	if idx, ok := screen.SelectedEntryIndex(); !ok || idx != 1 {
		t.Fatalf("expected second match selected, idx=%d ok=%v", idx, ok)
	}
	screen.CloseSearch(true)
	if screen.Status() != "" || screen.MatchCount() != 0 {
		t.Fatalf("expected cleared search state")
	}
}
