package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type FooterChrome struct {
	Width       int
	MetaLeft    string
	MetaRight   string
	SearchLine  string
	Hints       string
	ShowHints   bool
	FooterStyle lipgloss.Style
	LeftStyle   lipgloss.Style
	RightStyle  lipgloss.Style
	HintStyle   lipgloss.Style
}

func RenderFooterChrome(footer FooterChrome) string {
	width := max(20, footer.Width)
	metaLine := RenderSplitLine(width, footer.MetaLeft, footer.MetaRight, footer.LeftStyle, footer.RightStyle)
	lines := []string{footer.FooterStyle.Width(width).Render(metaLine)}
	if strings.TrimSpace(footer.SearchLine) != "" {
		lines = append(lines, footer.FooterStyle.Width(width).Render(truncateRight(footer.SearchLine, width)))
		return strings.Join(lines, "\n")
	}
	if footer.ShowHints && strings.TrimSpace(footer.Hints) != "" {
		lines = append(lines, footer.FooterStyle.Width(width).Render(footer.HintStyle.Render(truncateRight(footer.Hints, width))))
	}
	return strings.Join(lines, "\n")
}

func RenderSplitLine(width int, left, right string, leftStyle, rightStyle lipgloss.Style) string {
	width = max(0, width)
	if width == 0 {
		return ""
	}
	rightBudget := 0
	if right != "" && width >= 36 {
		rightBudget = clamp(width/2, 12, minInt(44, width-12))
	}
	leftBudget := width
	if rightBudget > 0 {
		leftBudget = max(0, width-rightBudget-1)
	}
	left = truncateRight(left, leftBudget)
	right = truncateRight(right, rightBudget)
	if right == "" {
		return leftStyle.Render(truncateRight(left, width))
	}
	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	return leftStyle.Render(left) + strings.Repeat(" ", gap) + rightStyle.Render(right)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
