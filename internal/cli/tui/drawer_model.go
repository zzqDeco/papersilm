package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type DrawerChoice struct {
	Label    string
	Value    string
	Detail   string
	Disabled bool
}

type DrawerModel struct {
	Kind      OverlayKind
	Title     string
	Message   string
	Hint      string
	Loading   bool
	Filter    textinput.Model
	All       []DrawerChoice
	Visible   []DrawerChoice
	Selection int
}

func NewDrawerModel(input textinput.Model) DrawerModel {
	return DrawerModel{Filter: input}
}

func (m DrawerModel) Open() bool {
	return m.Kind != OverlayNone
}

func (m *DrawerModel) Close() {
	m.Kind = OverlayNone
	m.Title = ""
	m.Message = ""
	m.Hint = ""
	m.Loading = false
	m.All = nil
	m.Visible = nil
	m.Selection = 0
	m.Filter.SetValue("")
	m.Filter.Blur()
}

func (m *DrawerModel) SetChoices(choices []DrawerChoice) {
	m.All = append(m.All[:0], choices...)
	m.Refresh()
}

func (m *DrawerModel) Refresh() {
	query := strings.TrimSpace(m.Filter.Value())
	if query == "" {
		m.Visible = append(m.Visible[:0], m.All...)
	} else {
		m.Visible = m.Visible[:0]
		lowerQuery := strings.ToLower(query)
		for _, choice := range m.All {
			haystack := strings.ToLower(strings.Join([]string{choice.Label, choice.Value, choice.Detail}, " "))
			if strings.Contains(haystack, lowerQuery) {
				m.Visible = append(m.Visible, choice)
			}
		}
	}
	if len(m.Visible) == 0 {
		m.Selection = 0
		return
	}
	m.Selection = clamp(m.Selection, 0, len(m.Visible)-1)
}

func (m *DrawerModel) Move(delta int) {
	if len(m.Visible) == 0 {
		m.Selection = 0
		return
	}
	m.Selection = clamp(m.Selection+delta, 0, len(m.Visible)-1)
}

func (m DrawerModel) Selected() (DrawerChoice, bool) {
	if len(m.Visible) == 0 {
		return DrawerChoice{}, false
	}
	return m.Visible[clamp(m.Selection, 0, len(m.Visible)-1)], true
}

func (m *DrawerModel) Update(msg tea.Msg) (DrawerModel, tea.Cmd) {
	var cmd tea.Cmd
	m.Filter, cmd = m.Filter.Update(msg)
	m.Refresh()
	return *m, cmd
}
