package cli

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type tuiScreen string

const (
	tuiScreenMain       tuiScreen = "main"
	tuiScreenTranscript tuiScreen = "transcript"
)

type tuiFocus string

const (
	tuiFocusInput            tuiFocus = "input"
	tuiFocusSuggestion       tuiFocus = "suggestion"
	tuiFocusPane             tuiFocus = "pane"
	tuiFocusModal            tuiFocus = "modal"
	tuiFocusTranscript       tuiFocus = "transcript"
	tuiFocusTranscriptSearch tuiFocus = "transcript_search"
	tuiFocusHistorySearch    tuiFocus = "history_search"
)

type tuiHistoryState struct {
	active bool
	index  int
	draft  string
	mode   protocol.TranscriptInputMode
}

const (
	transcriptSubtypeApprovalRequired = "approval.required"
	transcriptSubtypeApprovalApproved = "approval.approved"
	transcriptSubtypeApprovalRejected = "approval.rejected"
)

var transcriptCounter uint64

func newTranscriptEntry(
	sessionID string,
	entryType protocol.TranscriptEntryType,
	title, body string,
	opts ...func(*protocol.TranscriptEntry),
) protocol.TranscriptEntry {
	now := time.Now().UTC()
	entry := protocol.TranscriptEntry{
		ID:          fmt.Sprintf("msg_%d_%d", now.UnixNano(), atomic.AddUint64(&transcriptCounter, 1)),
		SessionID:   sessionID,
		Type:        entryType,
		Title:       strings.TrimSpace(title),
		Body:        strings.TrimSpace(body),
		CreatedAt:   now,
		RenderState: protocol.TranscriptRenderDefault,
	}
	for _, opt := range opts {
		opt(&entry)
	}
	return entry
}

func withTranscriptMarkdown(markdown bool) func(*protocol.TranscriptEntry) {
	return func(entry *protocol.TranscriptEntry) {
		entry.Markdown = markdown
	}
}

func withTranscriptInputMode(mode protocol.TranscriptInputMode) func(*protocol.TranscriptEntry) {
	return func(entry *protocol.TranscriptEntry) {
		entry.InputMode = mode
	}
}

func withTranscriptSubtype(value string) func(*protocol.TranscriptEntry) {
	return func(entry *protocol.TranscriptEntry) {
		entry.Subtype = strings.TrimSpace(value)
	}
}

func withTranscriptVisibility(visibility protocol.TranscriptVisibility, presentation protocol.TranscriptPresentation) func(*protocol.TranscriptEntry) {
	return func(entry *protocol.TranscriptEntry) {
		entry.Visibility = visibility
		entry.Presentation = presentation
	}
}

func transcriptInputModeForValue(value string) protocol.TranscriptInputMode {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "/") {
		return protocol.TranscriptInputCommand
	}
	if strings.HasPrefix(trimmed, "!") {
		return protocol.TranscriptInputShell
	}
	return protocol.TranscriptInputPrompt
}

func transcriptEntriesFromLegacyEvents(events []protocol.StreamEvent) []protocol.TranscriptEntry {
	entries := make([]protocol.TranscriptEntry, 0, len(events))
	for _, event := range events {
		entry, ok := transcriptEntryFromEvent(event)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func transcriptEntryFromEvent(event protocol.StreamEvent) (protocol.TranscriptEntry, bool) {
	body := strings.TrimSpace(event.Message)
	switch event.Type {
	case protocol.EventProgress:
		visibility, presentation := transcriptEventDisplay(event.Type)
		if progress := progressSummary(event.Payload); progress != "" {
			body = progress
		}
		if body == "" {
			body = "progress"
		}
		return protocol.TranscriptEntry{
			ID:           fmt.Sprintf("event_%s_%d", event.Type, event.CreatedAt.UnixNano()),
			SessionID:    event.SessionID,
			Type:         protocol.TranscriptEntryProgress,
			Subtype:      string(event.Type),
			Title:        "Progress",
			Body:         body,
			CreatedAt:    event.CreatedAt,
			Visibility:   visibility,
			Presentation: presentation,
		}, true
	case protocol.EventApprovalRequired:
		if summary := approvalSummary(event.Payload); summary != "" {
			body = summary
		}
		if body == "" {
			body = "approval required"
		}
		return protocol.TranscriptEntry{
			ID:           fmt.Sprintf("event_%s_%d", event.Type, event.CreatedAt.UnixNano()),
			SessionID:    event.SessionID,
			Type:         protocol.TranscriptEntryApproval,
			Subtype:      transcriptSubtypeApprovalRequired,
			Title:        "Approval Required",
			Body:         body,
			CreatedAt:    event.CreatedAt,
			Visibility:   protocol.TranscriptVisibilityDecision,
			Presentation: protocol.TranscriptPresentationBlock,
		}, true
	case protocol.EventError:
		if body == "" {
			body = "error"
		}
		return protocol.TranscriptEntry{
			ID:           fmt.Sprintf("event_%s_%d", event.Type, event.CreatedAt.UnixNano()),
			SessionID:    event.SessionID,
			Type:         protocol.TranscriptEntryError,
			Subtype:      string(event.Type),
			Title:        "Error",
			Body:         body,
			CreatedAt:    event.CreatedAt,
			Visibility:   protocol.TranscriptVisibilityDecision,
			Presentation: protocol.TranscriptPresentationBlock,
		}, true
	case protocol.EventAssistant:
		return protocol.TranscriptEntry{
			ID:           fmt.Sprintf("event_%s_%d", event.Type, event.CreatedAt.UnixNano()),
			SessionID:    event.SessionID,
			Type:         protocol.TranscriptEntryAssistant,
			Subtype:      string(event.Type),
			Title:        "Assistant",
			Body:         body,
			Markdown:     looksLikeMarkdown(body),
			CreatedAt:    event.CreatedAt,
			Visibility:   protocol.TranscriptVisibilityPrimary,
			Presentation: protocol.TranscriptPresentationBlock,
		}, true
	case protocol.EventResult:
		return protocol.TranscriptEntry{}, false
	default:
		visibility, presentation := transcriptEventDisplay(event.Type)
		if detail := payloadSummary(event.Payload); detail != "" && detail != body {
			if body != "" {
				body += " · "
			}
			body += detail
		}
		if body == "" {
			body = string(event.Type)
		}
		return protocol.TranscriptEntry{
			ID:           fmt.Sprintf("event_%s_%d", event.Type, event.CreatedAt.UnixNano()),
			SessionID:    event.SessionID,
			Type:         protocol.TranscriptEntrySystem,
			Subtype:      string(event.Type),
			Title:        string(event.Type),
			Body:         body,
			CreatedAt:    event.CreatedAt,
			Visibility:   visibility,
			Presentation: presentation,
		}, true
	}
}

func transcriptEventDisplay(eventType protocol.StreamEventType) (protocol.TranscriptVisibility, protocol.TranscriptPresentation) {
	switch eventType {
	case protocol.EventInit, protocol.EventSessionLoaded:
		return protocol.TranscriptVisibilityAmbient, protocol.TranscriptPresentationHidden
	case protocol.EventSourceAttached, protocol.EventAnalysis, protocol.EventPlan, protocol.EventProgress, protocol.EventArtifactWritten:
		return protocol.TranscriptVisibilityActivity, protocol.TranscriptPresentationGrouped
	default:
		return protocol.TranscriptVisibilityAmbient, protocol.TranscriptPresentationHidden
	}
}

func transcriptHistoryEntries(entries []protocol.TranscriptEntry, mode protocol.TranscriptInputMode) []protocol.TranscriptEntry {
	out := make([]protocol.TranscriptEntry, 0)
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Type != protocol.TranscriptEntryUser && entry.Type != protocol.TranscriptEntryCommand {
			continue
		}
		if entry.InputMode != "" && entry.InputMode != mode {
			continue
		}
		if strings.TrimSpace(entry.Body) == "" {
			continue
		}
		out = append(out, entry)
	}
	return out
}
