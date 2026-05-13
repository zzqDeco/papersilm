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
	Width               int
	Title               string
	Subtitle            string
	Question            string
	Summary             string
	Preview             string
	PreviewKind         string
	Rows                []ListRow
	Feedback            string
	FeedbackMode        string
	FeedbackLabel       string
	FeedbackPlaceholder string
	Hint                string
	DividerStyle        lipgloss.Style
	TitleStyle          lipgloss.Style
	MutedStyle          lipgloss.Style
	BodyStyle           lipgloss.Style
	PreviewAdd          lipgloss.Style
	PreviewDelete       lipgloss.Style
	PreviewCommand      lipgloss.Style
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
		lines = append(lines, renderPermissionPreview(dialog, bodyWidth)...)
	}
	if len(dialog.Rows) > 0 {
		lines = append(lines, RenderCompactListRows(dialog.Rows, bodyWidth)...)
	}
	if mode := strings.TrimSpace(dialog.FeedbackMode); mode != "" {
		lines = append(lines, renderPermissionFeedback(dialog, bodyWidth)...)
	}
	if hint := strings.TrimSpace(dialog.Hint); hint != "" {
		lines = append(lines, "  "+dialog.MutedStyle.Render(truncateRight(hint, bodyWidth)))
	}
	return strings.Join(lines, "\n")
}

func renderPermissionFeedback(dialog PermissionDialog, bodyWidth int) []string {
	label := permissionFeedbackLabel(dialog)
	placeholder := strings.TrimSpace(dialog.FeedbackPlaceholder)
	if placeholder == "" {
		placeholder = "Type feedback, then Enter"
	}
	value := strings.TrimRight(dialog.Feedback, "\n")
	lines := []string{
		"  " + dialog.MutedStyle.Render("│ "+truncateRight(label, bodyWidth-2)),
	}
	if strings.TrimSpace(value) == "" {
		lines = append(lines, "  "+dialog.MutedStyle.Render("│ › "+truncateRight(placeholder, bodyWidth-4)))
		return lines
	}
	valueLines := strings.Split(value, "\n")
	limit := min(len(valueLines), 4)
	for i := 0; i < limit; i++ {
		lines = append(lines, "  "+dialog.BodyStyle.Render("│ › "+truncateRight(valueLines[i], bodyWidth-4)))
	}
	if len(valueLines) > limit {
		lines = append(lines, "  "+dialog.MutedStyle.Render("│ …"))
	}
	return lines
}

func permissionFeedbackLabel(dialog PermissionDialog) string {
	label := strings.TrimSpace(dialog.FeedbackLabel)
	if label != "" {
		return label
	}
	if strings.TrimSpace(dialog.FeedbackMode) == "reject" {
		return "Tell papersilm what to do differently"
	}
	return "Tell papersilm what to do next"
}

func renderPermissionPreview(dialog PermissionDialog, bodyWidth int) []string {
	previewLines := strings.Split(strings.TrimSpace(dialog.Preview), "\n")
	limit := min(len(previewLines), 6)
	lines := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		raw := strings.TrimRight(previewLines[i], "\r")
		line := truncateRight(raw, bodyWidth-2)
		lines = append(lines, "  "+renderPermissionPreviewLine(dialog, line))
	}
	if len(previewLines) > limit {
		lines = append(lines, "  "+dialog.MutedStyle.Render("│ …"))
	}
	return lines
}

func renderPermissionPreviewLine(dialog PermissionDialog, line string) string {
	kind := strings.TrimSpace(dialog.PreviewKind)
	switch {
	case kind == "diff" && strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
		return dialog.PreviewAdd.Render("│ " + line)
	case kind == "diff" && strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
		return dialog.PreviewDelete.Render("│ " + line)
	case kind == "command" && strings.HasPrefix(strings.TrimSpace(line), "$ "):
		return dialog.PreviewCommand.Render("│ " + line)
	default:
		return dialog.MutedStyle.Render("│ " + line)
	}
}
