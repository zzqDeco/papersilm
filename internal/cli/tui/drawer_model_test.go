package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDrawerModelFiltersAndSelects(t *testing.T) {
	t.Parallel()

	model := NewDrawerModel(textinput.New())
	model.SetChoices([]DrawerChoice{
		{Label: "/help", Value: "/help", Detail: "Show help"},
		{Label: "/model", Value: "/model", Detail: "Pick model"},
	})
	model.Filter.SetValue("model")
	model.Refresh()

	if len(model.Visible) != 1 || model.Visible[0].Value != "/model" {
		t.Fatalf("expected filtered model choice, got %+v", model.Visible)
	}
	selected, ok := model.Selected()
	if !ok || selected.Value != "/model" {
		t.Fatalf("expected selected model choice, got %+v ok=%v", selected, ok)
	}
}

func TestDrawerModelUpdateOwnsFilterInput(t *testing.T) {
	t.Parallel()

	model := NewDrawerModel(textinput.New())
	model.SetChoices([]DrawerChoice{{Label: "/model", Value: "/model"}})
	model.Filter.Focus()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if updated.Filter.Value() != "m" {
		t.Fatalf("expected filter input to update, got %q", updated.Filter.Value())
	}
}
