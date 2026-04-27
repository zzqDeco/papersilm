package protocol

import "time"

type TranscriptEntryType string

const (
	TranscriptEntryUser      TranscriptEntryType = "user"
	TranscriptEntryCommand   TranscriptEntryType = "command"
	TranscriptEntryAssistant TranscriptEntryType = "assistant"
	TranscriptEntrySystem    TranscriptEntryType = "system"
	TranscriptEntryProgress  TranscriptEntryType = "progress"
	TranscriptEntryApproval  TranscriptEntryType = "approval"
	TranscriptEntryError     TranscriptEntryType = "error"
	TranscriptEntryDivider   TranscriptEntryType = "divider"
)

type TranscriptInputMode string

const (
	TranscriptInputPrompt  TranscriptInputMode = "prompt"
	TranscriptInputCommand TranscriptInputMode = "command"
	TranscriptInputShell   TranscriptInputMode = "shell"
)

type TranscriptRenderState string

const (
	TranscriptRenderDefault   TranscriptRenderState = "default"
	TranscriptRenderCollapsed TranscriptRenderState = "collapsed"
)

type TranscriptVisibility string

const (
	TranscriptVisibilityPrimary  TranscriptVisibility = "primary"
	TranscriptVisibilityActivity TranscriptVisibility = "activity"
	TranscriptVisibilityDecision TranscriptVisibility = "decision"
	TranscriptVisibilityAmbient  TranscriptVisibility = "ambient"
	TranscriptVisibilityDebug    TranscriptVisibility = "debug"
)

type TranscriptPresentation string

const (
	TranscriptPresentationBlock   TranscriptPresentation = "block"
	TranscriptPresentationRow     TranscriptPresentation = "row"
	TranscriptPresentationGrouped TranscriptPresentation = "grouped"
	TranscriptPresentationHidden  TranscriptPresentation = "hidden"
	TranscriptPresentationPane    TranscriptPresentation = "pane"
)

type TranscriptEntry struct {
	ID           string                 `json:"id"`
	SessionID    string                 `json:"session_id,omitempty"`
	Type         TranscriptEntryType    `json:"type"`
	Subtype      string                 `json:"subtype,omitempty"`
	Title        string                 `json:"title,omitempty"`
	Body         string                 `json:"body,omitempty"`
	InputMode    TranscriptInputMode    `json:"input_mode,omitempty"`
	RenderState  TranscriptRenderState  `json:"render_state,omitempty"`
	Visibility   TranscriptVisibility   `json:"visibility,omitempty"`
	Presentation TranscriptPresentation `json:"presentation,omitempty"`
	SourceRef    string                 `json:"source_ref,omitempty"`
	Markdown     bool                   `json:"markdown,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}
