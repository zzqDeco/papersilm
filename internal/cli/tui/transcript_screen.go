package tui

import (
	"fmt"
	"strings"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type TranscriptScreen struct {
	active     bool
	frozenLen  int
	matches    []int
	selection  int
	status     string
	searchOpen bool
}

func (s *TranscriptScreen) Open(totalEntries int) {
	if !s.active {
		s.active = true
		s.frozenLen = clamp(totalEntries, 0, totalEntries)
	}
}

func (s *TranscriptScreen) Close() {
	s.active = false
	s.frozenLen = 0
	s.ClearSearch()
}

func (s *TranscriptScreen) Entries(entries []protocol.TranscriptEntry) []protocol.TranscriptEntry {
	if !s.active {
		return append([]protocol.TranscriptEntry(nil), entries...)
	}
	limit := clamp(s.frozenLen, 0, len(entries))
	return append([]protocol.TranscriptEntry(nil), entries[:limit]...)
}

func (s *TranscriptScreen) OpenSearch() {
	s.searchOpen = true
	s.matches = nil
	s.selection = 0
	s.status = ""
}

func (s *TranscriptScreen) CloseSearch(clear bool) {
	s.searchOpen = false
	if clear {
		s.ClearSearch()
	}
}

func (s *TranscriptScreen) ClearSearch() {
	s.searchOpen = false
	s.matches = nil
	s.selection = 0
	s.status = ""
}

func (s *TranscriptScreen) RefreshSearch(entries []protocol.TranscriptEntry, query string) {
	s.matches = transcriptScreenSearchMatches(entries, query)
	if len(s.matches) == 0 {
		s.selection = 0
		if strings.TrimSpace(query) == "" {
			s.status = ""
		} else {
			s.status = "No matches"
		}
		return
	}
	s.selection = clamp(s.selection, 0, len(s.matches)-1)
	s.status = fmt.Sprintf("%d matches", len(s.matches))
}

func transcriptScreenSearchMatches(entries []protocol.TranscriptEntry, query string) []int {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return nil
	}
	matches := make([]int, 0)
	for idx, entry := range entries {
		haystack := strings.ToLower(strings.Join([]string{
			string(entry.Type),
			entry.Subtype,
			entry.Title,
			entry.Body,
			entry.SourceRef,
		}, " "))
		if strings.Contains(haystack, query) {
			matches = append(matches, idx)
		}
	}
	return matches
}

func (s *TranscriptScreen) MoveSearch(delta int) bool {
	if len(s.matches) == 0 {
		return false
	}
	next := s.selection + delta
	if next < 0 {
		next = len(s.matches) - 1
	}
	if next >= len(s.matches) {
		next = 0
	}
	s.selection = next
	s.status = fmt.Sprintf("%d matches", len(s.matches))
	return true
}

func (s *TranscriptScreen) SelectedEntryIndex() (int, bool) {
	if len(s.matches) == 0 {
		return 0, false
	}
	return s.matches[s.selection], true
}

func (s *TranscriptScreen) IsSelected(index int) bool {
	selected, ok := s.SelectedEntryIndex()
	return ok && selected == index
}

func (s *TranscriptScreen) Status() string {
	return s.status
}

func (s *TranscriptScreen) MatchCount() int {
	return len(s.matches)
}

func (s *TranscriptScreen) MatchPosition() (int, int) {
	if len(s.matches) == 0 {
		return 0, 0
	}
	return s.selection + 1, len(s.matches)
}
