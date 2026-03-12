package protocol

import "time"

type StreamEventType string

const (
	EventInit             StreamEventType = "init"
	EventSessionLoaded    StreamEventType = "session_loaded"
	EventSourceAttached   StreamEventType = "source_attached"
	EventAnalysis         StreamEventType = "analysis"
	EventPlan             StreamEventType = "plan"
	EventApprovalRequired StreamEventType = "approval_required"
	EventProgress         StreamEventType = "progress"
	EventAssistant        StreamEventType = "assistant"
	EventArtifactWritten  StreamEventType = "artifact_written"
	EventResult           StreamEventType = "result"
	EventError            StreamEventType = "error"
)

type StreamEvent struct {
	Type      StreamEventType `json:"type"`
	SessionID string          `json:"session_id,omitempty"`
	Message   string          `json:"message,omitempty"`
	Payload   interface{}     `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

