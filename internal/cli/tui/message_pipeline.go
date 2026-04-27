package tui

import (
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type UIMessageType string

const (
	UIMessageUser      UIMessageType = "user"
	UIMessageAssistant UIMessageType = "assistant"
	UIMessageSystem    UIMessageType = "system"
	UIMessageActivity  UIMessageType = "activity"
	UIMessageDecision  UIMessageType = "decision"
	UIMessageError     UIMessageType = "error"
)

type UIMessage struct {
	ID           string
	Type         UIMessageType
	Subtype      string
	TurnID       string
	RunID        string
	CreatedAt    time.Time
	Visibility   protocol.TranscriptVisibility
	Presentation protocol.TranscriptPresentation
	RenderState  protocol.TranscriptRenderState
	Title        string
	Body         string
	Markdown     bool
	SourceRef    string
}

type MessagePipeline struct {
	activity UIMessage
	hasGroup bool
}

func NewMessagePipeline() MessagePipeline {
	return MessagePipeline{}
}

func (p *MessagePipeline) Project(entry protocol.TranscriptEntry) (UIMessage, bool) {
	visibility := entry.Visibility
	presentation := entry.Presentation
	if visibility == "" {
		visibility = defaultVisibility(entry.Type)
	}
	if presentation == "" {
		presentation = defaultPresentation(visibility)
	}
	if presentation == protocol.TranscriptPresentationHidden ||
		visibility == protocol.TranscriptVisibilityAmbient ||
		visibility == protocol.TranscriptVisibilityDebug {
		return UIMessage{}, false
	}
	msg := UIMessage{
		ID:           first(entry.ID, entry.Subtype, string(entry.Type)),
		Type:         uiMessageType(entry.Type, visibility),
		Subtype:      entry.Subtype,
		CreatedAt:    entry.CreatedAt,
		Visibility:   visibility,
		Presentation: presentation,
		RenderState:  entry.RenderState,
		Title:        entry.Title,
		Body:         entry.Body,
		Markdown:     entry.Markdown,
		SourceRef:    entry.SourceRef,
	}
	if visibility == protocol.TranscriptVisibilityActivity || presentation == protocol.TranscriptPresentationGrouped {
		return p.projectActivity(msg), true
	}
	p.hasGroup = false
	p.activity = UIMessage{}
	return msg, true
}

func (p *MessagePipeline) projectActivity(msg UIMessage) UIMessage {
	body := strings.TrimSpace(msg.Body)
	if body == "" {
		body = strings.TrimSpace(msg.Title)
	}
	if body == "" {
		body = "Working"
	}
	if !p.hasGroup {
		p.activity = msg
		p.activity.ID = "activity.grouped"
		p.activity.Type = UIMessageActivity
		p.activity.Title = "Activity"
		p.activity.Body = body
		p.activity.Presentation = protocol.TranscriptPresentationGrouped
		p.hasGroup = true
		return p.activity
	}
	p.activity.Body = compactActivity(p.activity.Body, body)
	return p.activity
}

func defaultVisibility(entryType protocol.TranscriptEntryType) protocol.TranscriptVisibility {
	switch entryType {
	case protocol.TranscriptEntryUser, protocol.TranscriptEntryCommand, protocol.TranscriptEntryAssistant, protocol.TranscriptEntryDivider:
		return protocol.TranscriptVisibilityPrimary
	case protocol.TranscriptEntryApproval, protocol.TranscriptEntryError:
		return protocol.TranscriptVisibilityDecision
	case protocol.TranscriptEntryProgress:
		return protocol.TranscriptVisibilityActivity
	default:
		return protocol.TranscriptVisibilityAmbient
	}
}

func defaultPresentation(visibility protocol.TranscriptVisibility) protocol.TranscriptPresentation {
	switch visibility {
	case protocol.TranscriptVisibilityActivity:
		return protocol.TranscriptPresentationGrouped
	case protocol.TranscriptVisibilityAmbient, protocol.TranscriptVisibilityDebug:
		return protocol.TranscriptPresentationHidden
	default:
		return protocol.TranscriptPresentationBlock
	}
}

func uiMessageType(entryType protocol.TranscriptEntryType, visibility protocol.TranscriptVisibility) UIMessageType {
	switch {
	case visibility == protocol.TranscriptVisibilityDecision && entryType == protocol.TranscriptEntryError:
		return UIMessageError
	case visibility == protocol.TranscriptVisibilityDecision:
		return UIMessageDecision
	case visibility == protocol.TranscriptVisibilityActivity:
		return UIMessageActivity
	case entryType == protocol.TranscriptEntryUser || entryType == protocol.TranscriptEntryCommand:
		return UIMessageUser
	case entryType == protocol.TranscriptEntryAssistant:
		return UIMessageAssistant
	default:
		return UIMessageSystem
	}
}

func compactActivity(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" || strings.Contains(current, next) {
		return current
	}
	return current + " · " + next
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "message"
}
