package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestPermissionModelCapturesFeedbackBeforeShortcuts(t *testing.T) {
	t.Parallel()

	model := NewPermissionModel()
	model.Sync(true, protocol.PermissionRequest{
		RequestID: "req",
		Title:     "Run command?",
		Options: []protocol.PermissionOption{
			{Value: "reject", Label: "No", Feedback: "reject"},
		},
	})
	model.State.FeedbackMode = "reject"

	event := model.UpdateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if event.Kind != PermissionEventFeedback {
		t.Fatalf("expected feedback event, got %+v", event)
	}
	if model.State.Feedback != "n" {
		t.Fatalf("expected feedback text, got %q", model.State.Feedback)
	}
}

func TestPermissionModelRoutesConfirmationKeys(t *testing.T) {
	t.Parallel()

	model := NewPermissionModel()
	model.Sync(true, protocol.PermissionRequest{
		RequestID: "req",
		Title:     "Run command?",
		Options: []protocol.PermissionOption{
			{Value: "accept-once", Label: "Yes"},
			{Value: "reject", Label: "No"},
		},
	})

	if event := model.UpdateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}); event.Kind != PermissionEventApprove {
		t.Fatalf("expected approve event, got %+v", event)
	}
	if event := model.UpdateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}); event.Kind != PermissionEventReject {
		t.Fatalf("expected reject event, got %+v", event)
	}
}
