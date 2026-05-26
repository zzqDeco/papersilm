package input

import (
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type Source string

const (
	SourceCLI        Source = "cli"
	SourceTUI        Source = "tui"
	SourceQueue      Source = "queue"
	SourcePermission Source = "permission"
	SourceSystem     Source = "system"
)

type Kind string

const (
	KindUserMessage        Kind = "user_message"
	KindSlashCommand       Kind = "slash_command"
	KindRunPlanned         Kind = "run_planned"
	KindTaskAction         Kind = "task_action"
	KindPermissionDecision Kind = "permission_decision"
	KindStop               Kind = "stop"
	KindPreempt            Kind = "preempt"
	KindResume             Kind = "resume"
)

type Priority int

const (
	PriorityNormal Priority = iota
	PriorityHigh
	PriorityCritical
)

type Item struct {
	ID        string
	SessionID string
	Source    Source
	Kind      Kind
	Text      string
	Payload   any
	Priority  Priority
	CauseID   string
	CreatedAt time.Time
}

type RunPlannedPayload struct {
	Language string
	Style    string
}

type TaskActionPayload struct {
	Action   string
	TaskID   string
	Language string
	Style    string
	Approved bool
	Comment  string
}

func FromClientRequest(req protocol.ClientRequest) Item {
	return Item{
		SessionID: req.SessionID,
		Source:    SourceCLI,
		Kind:      KindUserMessage,
		Text:      req.Task,
		Payload:   req,
		Priority:  PriorityNormal,
	}
}

func FromRunPlanned(sessionID, lang, style string) Item {
	return Item{
		SessionID: sessionID,
		Source:    SourceCLI,
		Kind:      KindRunPlanned,
		Payload: RunPlannedPayload{
			Language: lang,
			Style:    style,
		},
		Priority: PriorityHigh,
	}
}

func FromTaskAction(sessionID string, payload TaskActionPayload) Item {
	return Item{
		SessionID: sessionID,
		Source:    SourceCLI,
		Kind:      KindTaskAction,
		Payload:   payload,
		Priority:  PriorityHigh,
	}
}

func FromSlashCommand(sessionID, line string, priority Priority) Item {
	return Item{
		SessionID: sessionID,
		Source:    SourceCLI,
		Kind:      KindSlashCommand,
		Text:      line,
		Priority:  priority,
	}
}

func FromPermissionDecision(sessionID string, decision protocol.PermissionDecision) Item {
	return Item{
		SessionID: sessionID,
		Source:    SourcePermission,
		Kind:      KindPermissionDecision,
		Payload:   decision,
		Priority:  PriorityCritical,
	}
}
