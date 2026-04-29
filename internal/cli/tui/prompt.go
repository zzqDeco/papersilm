package tui

import "github.com/charmbracelet/lipgloss"

type PromptChrome struct {
	Width        int
	Label        string
	Body         string
	LabelStyle   lipgloss.Style
	DividerStyle lipgloss.Style
	BodyStyle    lipgloss.Style
}

func RenderPromptChrome(prompt PromptChrome) string {
	return prompt.BodyStyle.Render(prompt.Body)
}
