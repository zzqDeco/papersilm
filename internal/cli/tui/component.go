package tui

import tea "github.com/charmbracelet/bubbletea"

type ComponentEventKind string

const (
	ComponentEventNone   ComponentEventKind = ""
	ComponentEventChange ComponentEventKind = "change"
	ComponentEventSubmit ComponentEventKind = "submit"
)

type ComponentEvent struct {
	Kind  ComponentEventKind
	Value string
}

type ComponentUpdate struct {
	Cmd   tea.Cmd
	Event ComponentEvent
}
