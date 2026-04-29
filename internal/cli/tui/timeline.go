package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type TimelineKind string

const (
	TimelineItemUser      TimelineKind = "user"
	TimelineItemAssistant TimelineKind = "assistant"
	TimelineItemSystem    TimelineKind = "system"
	TimelineItemProgress  TimelineKind = "progress"
	TimelineItemApproval  TimelineKind = "approval"
	TimelineItemError     TimelineKind = "error"
)

const (
	TimelineSubtypeApprovalApproved = "approval.approved"
	TimelineSubtypeApprovalRejected = "approval.rejected"
	TimelineSubtypeApprovalRequired = "approval.required"
)

type TimelineItem struct {
	ID              string
	Kind            TimelineKind
	Subtype         string
	Title           string
	Body            string
	Markdown        bool
	Compact         bool
	CreatedAt       time.Time
	Workspace       string
	DecisionOptions string
}

type TimelineStyles struct {
	Body           lipgloss.Style
	UserShell      lipgloss.Style
	UserLabel      lipgloss.Style
	AssistantLabel lipgloss.Style
	ApprovalShell  lipgloss.Style
	ApprovalLabel  lipgloss.Style
	SuccessShell   lipgloss.Style
	SuccessLabel   lipgloss.Style
	RejectionShell lipgloss.Style
	RejectionLabel lipgloss.Style
	ErrorShell     lipgloss.Style
	ErrorLabel     lipgloss.Style
	FooterMuted    lipgloss.Style
	ProgressLine   lipgloss.Style
	SystemLine     lipgloss.Style
}

type TimelineRenderer struct {
	Styles   TimelineStyles
	Markdown func(markdown string, width int) string
}

func RenderTimelineItem(item TimelineItem, width int, renderer TimelineRenderer) string {
	bodyWidth := max(10, width-4)
	timestamp := item.CreatedAt.Local().Format("15:04")
	styles := renderer.Styles
	switch item.Kind {
	case TimelineItemUser:
		body := styles.Body.Width(bodyWidth).Render(item.Body)
		label := "you"
		if text := strings.TrimSpace(item.Title); text != "" {
			label = strings.ToLower(text)
		}
		header := renderTimelineTimestamp(styles.UserLabel, styles.FooterMuted, label, timestamp)
		return styles.UserShell.Width(width).Render(header + "\n" + body)
	case TimelineItemAssistant:
		body := item.Body
		if item.Markdown && renderer.Markdown != nil {
			body = renderer.Markdown(item.Body, bodyWidth)
		} else {
			body = styles.Body.Width(bodyWidth).Render(item.Body)
		}
		if !shouldRenderTimelineAssistantHeader(item.Title) {
			return body
		}
		header := renderTimelineTimestamp(styles.AssistantLabel, styles.FooterMuted, strings.ToLower(item.Title), timestamp)
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	case TimelineItemApproval:
		return renderTimelineDecision(item, width, bodyWidth, timestamp, styles)
	case TimelineItemError:
		body := styles.Body.Width(bodyWidth).Render(item.Body)
		header := renderTimelineTimestamp(styles.ErrorLabel, styles.FooterMuted, item.Title, timestamp)
		return styles.ErrorShell.Width(width).Render(header + "\n" + body)
	case TimelineItemProgress:
		return renderTimelineActivity(item, width, styles)
	default:
		if item.Subtype == "welcome" {
			return renderTimelineWelcome(item, width, styles)
		}
		line := fmt.Sprintf("%s  %s", timestamp, item.Body)
		return styles.SystemLine.Width(width).Render(truncateRight(line, width))
	}
}

func renderTimelineActivity(item TimelineItem, width int, styles TimelineStyles) string {
	body := strings.TrimSpace(strings.ReplaceAll(item.Body, "\n", " "))
	if body == "" {
		body = strings.TrimSpace(item.Title)
	}
	if body == "" {
		return ""
	}
	prefix := "  · "
	if item.Subtype == "activity.grouped" {
		prefix = "  • "
	}
	return styles.ProgressLine.Width(width).Render(truncateRight(prefix+body, width))
}

func renderTimelineWelcome(item TimelineItem, width int, styles TimelineStyles) string {
	hint := strings.TrimSpace(item.Body)
	if hint == "" {
		hint = "Ask about this workspace or type /commands"
	}
	prefix := "Workspace ready"
	if workspace := strings.TrimSpace(item.Workspace); workspace != "" {
		prefix = "Workspace " + truncateRight(workspace, max(8, width-20))
	}
	line := prefix + " · " + hint + " · /commands · /model"
	return styles.FooterMuted.Render(truncateRight(line, width))
}

func renderTimelineDecision(item TimelineItem, width, bodyWidth int, timestamp string, styles TimelineStyles) string {
	switch item.Subtype {
	case TimelineSubtypeApprovalApproved:
		body := styles.Body.Width(bodyWidth).Render(item.Body)
		header := renderTimelineTimestamp(styles.SuccessLabel, styles.FooterMuted, firstTimelineText(item.Title, "Approved"), timestamp)
		return styles.SuccessShell.Width(width).Render(header + "\n" + body)
	case TimelineSubtypeApprovalRejected:
		if item.Compact {
			return renderCompactTimelineDecision(styles.RejectionLabel, styles.FooterMuted, firstTimelineText(item.Title, "Rejected"), timestamp, item.Body, width)
		}
		body := styles.Body.Width(bodyWidth).Render(item.Body)
		header := renderTimelineTimestamp(styles.RejectionLabel, styles.FooterMuted, firstTimelineText(item.Title, "Rejected"), timestamp)
		return styles.RejectionShell.Width(width).Render(header + "\n" + body)
	default:
		body := styles.Body.Width(bodyWidth).Render(item.Body)
		if options := strings.TrimSpace(item.DecisionOptions); options != "" {
			body = lipgloss.JoinVertical(lipgloss.Left, body, "", options)
		}
		header := renderTimelineTimestamp(styles.ApprovalLabel, styles.FooterMuted, firstTimelineText(item.Title, "Approval Required"), timestamp)
		return styles.ApprovalShell.Width(width).Render(header + "\n" + body)
	}
}

func renderTimelineTimestamp(labelStyle lipgloss.Style, timeStyle lipgloss.Style, label, timestamp string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return timeStyle.Render(timestamp)
	}
	return labelStyle.Render(label) + timeStyle.Render(" · "+timestamp)
}

func renderCompactTimelineDecision(labelStyle, mutedStyle lipgloss.Style, title, timestamp, body string, width int) string {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(strings.ReplaceAll(body, "\n", " "))
	available := max(0, width-lipgloss.Width(title)-lipgloss.Width(timestamp)-4)
	if body != "" {
		body = truncateRight(body, available)
	}
	line := labelStyle.Render(title) + mutedStyle.Render(" · "+timestamp)
	if body != "" {
		line += mutedStyle.Render("  " + body)
	}
	return line
}

func shouldRenderTimelineAssistantHeader(title string) bool {
	title = strings.TrimSpace(strings.ToLower(title))
	return title != "" && title != "assistant"
}

func firstTimelineText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
