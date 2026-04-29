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

type PermissionDialog struct {
	Width        int
	Title        string
	Subtitle     string
	Question     string
	Summary      string
	Preview      string
	Rows         []ListRow
	Feedback     string
	FeedbackMode string
	Hint         string
	DividerStyle lipgloss.Style
	TitleStyle   lipgloss.Style
	MutedStyle   lipgloss.Style
	BodyStyle    lipgloss.Style
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

func RenderPermissionDialog(dialog PermissionDialog) string {
	width := max(20, dialog.Width)
	bodyWidth := max(12, width-4)
	bodyStyle := dialog.BodyStyle
	lines := []string{
		dialog.DividerStyle.Render(strings.Repeat("─", width)),
	}
	if title := strings.TrimSpace(dialog.Title); title != "" {
		if subtitle := strings.TrimSpace(dialog.Subtitle); subtitle != "" {
			title = title + dialog.MutedStyle.Render(" · "+truncateRight(subtitle, max(8, bodyWidth-lipgloss.Width(title)-3)))
		}
		lines = append(lines, "  "+dialog.TitleStyle.Render(truncateRight(title, bodyWidth)))
	}
	if question := strings.TrimSpace(dialog.Question); question != "" {
		lines = append(lines, "  "+bodyStyle.Render(truncateRight(question, bodyWidth)))
	}
	if summary := strings.TrimSpace(dialog.Summary); summary != "" {
		lines = append(lines, "  "+dialog.MutedStyle.Render(truncateRight(summary, bodyWidth)))
	}
	if preview := strings.TrimSpace(dialog.Preview); preview != "" {
		previewLines := strings.Split(preview, "\n")
		limit := min(len(previewLines), 6)
		for i := 0; i < limit; i++ {
			line := truncateRight(strings.TrimRight(previewLines[i], "\r"), bodyWidth-2)
			lines = append(lines, "  "+dialog.MutedStyle.Render("│ "+line))
		}
		if len(previewLines) > limit {
			lines = append(lines, "  "+dialog.MutedStyle.Render("│ …"))
		}
	}
	if len(dialog.Rows) > 0 {
		lines = append(lines, RenderListRows(dialog.Rows, bodyWidth)...)
	}
	if mode := strings.TrimSpace(dialog.FeedbackMode); mode != "" {
		label := "Tell papersilm what to do next"
		if mode == "reject" {
			label = "Tell papersilm what to do differently"
		}
		value := strings.TrimRight(dialog.Feedback, "\n")
		if value == "" {
			value = " "
		}
		lines = append(lines, "  "+dialog.MutedStyle.Render(label))
		for _, line := range strings.Split(value, "\n") {
			lines = append(lines, "  "+bodyStyle.Render("› "+truncateRight(line, bodyWidth-2)))
		}
	}
	if hint := strings.TrimSpace(dialog.Hint); hint != "" {
		lines = append(lines, "  "+dialog.MutedStyle.Render(truncateRight(hint, bodyWidth)))
	}
	return strings.Join(lines, "\n")
}
