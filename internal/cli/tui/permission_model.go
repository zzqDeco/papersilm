package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type PermissionEventKind string

const (
	PermissionEventNone     PermissionEventKind = ""
	PermissionEventApprove  PermissionEventKind = "approve"
	PermissionEventReject   PermissionEventKind = "reject"
	PermissionEventFeedback PermissionEventKind = "feedback"
	PermissionEventDetails  PermissionEventKind = "details"
)

type PermissionEvent struct {
	Kind PermissionEventKind
	Key  string
}

type PermissionModel struct {
	State PermissionState
}

func NewPermissionModel() PermissionModel {
	return PermissionModel{}
}

func (m *PermissionModel) Sync(active bool, request protocol.PermissionRequest) {
	m.State.Sync(active, request)
}

func (m PermissionModel) Active() bool {
	return m.State.Active
}

func (m PermissionModel) CapturesText() bool {
	return m.State.Active && m.State.FeedbackMode != ""
}

func (m *PermissionModel) CancelFeedback() {
	m.State.CancelFeedback()
}

func (m *PermissionModel) UpdateKey(msg tea.KeyMsg) PermissionEvent {
	if !m.State.Active {
		return PermissionEvent{}
	}
	if m.State.FeedbackMode != "" {
		switch msg.Type {
		case tea.KeyRunes:
			m.State.AppendText(string(msg.Runes))
			return PermissionEvent{Kind: PermissionEventFeedback, Key: msg.String()}
		case tea.KeySpace:
			m.State.AppendText(" ")
			return PermissionEvent{Kind: PermissionEventFeedback, Key: msg.String()}
		case tea.KeyBackspace, tea.KeyDelete:
			m.State.Backspace()
			return PermissionEvent{Kind: PermissionEventFeedback, Key: msg.String()}
		case tea.KeyCtrlJ:
			m.State.Newline()
			return PermissionEvent{Kind: PermissionEventFeedback, Key: msg.String()}
		}
	}
	switch msg.String() {
	case "up", "left":
		m.State.MoveSelection(-1)
	case "down", "right", "space":
		m.State.MoveSelection(1)
	case "tab":
		m.State.ToggleFeedback()
		return PermissionEvent{Kind: PermissionEventFeedback, Key: msg.String()}
	case "shift+tab":
		m.State.CycleScope("accept")
	case "ctrl+e", "i":
		return PermissionEvent{Kind: PermissionEventDetails, Key: msg.String()}
	case "a", "y", "enter":
		return PermissionEvent{Kind: PermissionEventApprove, Key: msg.String()}
	case "r", "n", "esc":
		return PermissionEvent{Kind: PermissionEventReject, Key: msg.String()}
	}
	return PermissionEvent{}
}
