package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type PromptModel struct {
	input      textarea.Model
	controller PromptController
}

func NewPromptModel(input textarea.Model, controller PromptController) PromptModel {
	controller.SetValue(input.Value())
	return PromptModel{input: input, controller: controller}
}

func (m PromptModel) Init() tea.Cmd {
	return m.input.Focus()
}

func (m PromptModel) Input() textarea.Model {
	return m.input
}

func (m PromptModel) Controller() PromptController {
	return m.controller
}

func (m *PromptModel) SetInput(input textarea.Model) {
	m.input = input
	m.controller.SetValue(input.Value())
}

func (m *PromptModel) SetController(controller PromptController) {
	m.controller = controller
	m.controller.SetValue(m.input.Value())
}

func (m *PromptModel) SetValue(value string) {
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.controller.SetValue(value)
}

func (m PromptModel) Value() string {
	return m.input.Value()
}

func (m PromptModel) Mode() PromptMode {
	return m.controller.Mode()
}

func (m PromptModel) View() string {
	return m.input.View()
}

func (m *PromptModel) Focus() tea.Cmd {
	return m.input.Focus()
}

func (m *PromptModel) Blur() {
	m.input.Blur()
}

func (m *PromptModel) SetWidth(width int) {
	m.input.SetWidth(width)
}

func (m *PromptModel) SetHeight(height int) {
	m.input.SetHeight(height)
}

func (m PromptModel) LineCount() int {
	return m.input.LineCount()
}

func (m PromptModel) AtFirstLine() bool {
	if strings.TrimSpace(m.input.Value()) == "" {
		return true
	}
	return m.input.Line() == 0 && m.input.LineInfo().RowOffset == 0
}

func (m PromptModel) AtLastLine() bool {
	return m.input.Line() >= max(0, m.input.LineCount()-1)
}

func (m *PromptModel) SetHistory(entries []PromptHistoryEntry) {
	m.controller.SetHistory(entries)
}

func (m *PromptModel) HistoryPrev() bool {
	if !m.controller.HistoryPrev() {
		return false
	}
	m.input.SetValue(m.controller.Value())
	m.input.CursorEnd()
	return true
}

func (m *PromptModel) HistoryNext() bool {
	if !m.controller.HistoryNext() {
		return false
	}
	m.input.SetValue(m.controller.Value())
	m.input.CursorEnd()
	return true
}

func (m *PromptModel) CancelHistory() {
	m.controller.CancelHistory()
	m.input.SetValue(m.controller.Value())
	m.input.CursorEnd()
}

func (m *PromptModel) Update(msg tea.Msg) (PromptModel, ComponentUpdate) {
	var cmd tea.Cmd
	focusCmd := m.input.Focus()
	m.input, cmd = m.input.Update(msg)
	m.controller.SetValue(m.input.Value())
	return *m, ComponentUpdate{
		Cmd: tea.Batch(focusCmd, cmd),
		Event: ComponentEvent{
			Kind:  ComponentEventChange,
			Value: m.input.Value(),
		},
	}
}
