package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type PromptChrome struct {
	Width        int
	Label        string
	Body         string
	LabelStyle   lipgloss.Style
	DividerStyle lipgloss.Style
	BodyStyle    lipgloss.Style
}

func RenderPromptChrome(prompt PromptChrome) string {
	width := max(20, prompt.Width)
	body := prompt.BodyStyle.Width(width).Render(prompt.Body)
	label := strings.TrimSpace(prompt.Label)
	if strings.Contains(strings.ToLower(label), "approval") {
		return lipgloss.JoinVertical(lipgloss.Left, prompt.LabelStyle.Render(label), body)
	}
	return body
}
