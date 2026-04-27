package tui

import "github.com/zzqDeco/papersilm/pkg/protocol"

type MessageStore struct {
	entries []protocol.TranscriptEntry
}

func NewMessageStore(entries []protocol.TranscriptEntry) MessageStore {
	var store MessageStore
	store.Reset(entries)
	return store
}

func (s *MessageStore) Reset(entries []protocol.TranscriptEntry) {
	s.entries = append(s.entries[:0], entries...)
}

func (s *MessageStore) Append(entry protocol.TranscriptEntry) {
	s.entries = append(s.entries, entry)
}

func (s *MessageStore) Len() int {
	return len(s.entries)
}

func (s *MessageStore) Entries() []protocol.TranscriptEntry {
	return append([]protocol.TranscriptEntry(nil), s.entries...)
}

func (s *MessageStore) Frozen(limit int) []protocol.TranscriptEntry {
	if limit < 0 {
		limit = 0
	}
	if limit > len(s.entries) {
		limit = len(s.entries)
	}
	return append([]protocol.TranscriptEntry(nil), s.entries[:limit]...)
}

func (s *MessageStore) LatestInput() string {
	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if entry.Type != protocol.TranscriptEntryUser && entry.Type != protocol.TranscriptEntryCommand {
			continue
		}
		if entry.Body != "" {
			return entry.Body
		}
	}
	return ""
}

func (s *MessageStore) History(mode protocol.TranscriptInputMode) []protocol.TranscriptEntry {
	out := make([]protocol.TranscriptEntry, 0)
	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if entry.Type != protocol.TranscriptEntryUser && entry.Type != protocol.TranscriptEntryCommand {
			continue
		}
		if entry.InputMode != "" && entry.InputMode != mode {
			continue
		}
		if entry.Body == "" {
			continue
		}
		out = append(out, entry)
	}
	return out
}
