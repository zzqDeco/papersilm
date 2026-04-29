package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type DecisionPanel struct {
	Width        int
	Title        string
	Summary      string
	Hint         string
	Rows         []ListRow
	DividerStyle lipgloss.Style
	TitleStyle   lipgloss.Style
	MutedStyle   lipgloss.Style
}

func RenderDecisionPanel(panel DecisionPanel) string {
	width := max(20, panel.Width)
	bodyWidth := max(12, width-4)
	lines := []string{
		panel.DividerStyle.Render(strings.Repeat("─", width)),
	}
	title := strings.TrimSpace(panel.Title)
	if title != "" {
		lines = append(lines, "  "+panel.TitleStyle.Render(title))
	}
	if summary := strings.TrimSpace(panel.Summary); summary != "" {
		lines = append(lines, "  "+panel.MutedStyle.Render(truncateRight(summary, bodyWidth)))
	}
	if len(panel.Rows) > 0 {
		lines = append(lines, RenderListRows(panel.Rows, bodyWidth)...)
	}
	if hint := strings.TrimSpace(panel.Hint); hint != "" {
		lines = append(lines, "  "+panel.MutedStyle.Render(truncateRight(hint, bodyWidth)))
	}
	return strings.Join(lines, "\n")
}
