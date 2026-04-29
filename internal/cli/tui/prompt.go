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
	label := strings.TrimSpace(prompt.Label)
	if label == "" {
		label = "prompt"
	}
	label = " " + label + " "
	dividerWidth := max(1, width-lipgloss.Width(label))
	divider := prompt.LabelStyle.Render(label) + prompt.DividerStyle.Render(strings.Repeat("─", dividerWidth))
	return lipgloss.JoinVertical(
		lipgloss.Left,
		divider,
		prompt.BodyStyle.Width(width).Render(prompt.Body),
	)
}
