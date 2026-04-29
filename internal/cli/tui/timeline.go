package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
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
	styles := renderer.Styles
	switch item.Kind {
	case TimelineItemUser:
		body := styles.Body.Render(wrapTimelineText(item.Body, bodyWidth))
		return styles.UserShell.Render(body)
	case TimelineItemAssistant:
		body := item.Body
		if item.Markdown && renderer.Markdown != nil {
			body = renderer.Markdown(item.Body, bodyWidth)
		} else {
			body = styles.Body.Width(bodyWidth).Render(item.Body)
		}
		return body
	case TimelineItemApproval:
		return renderTimelineDecision(item, width, bodyWidth, styles)
	case TimelineItemError:
		body := styles.Body.Width(bodyWidth).Render(item.Body)
		header := styles.ErrorLabel.Render(firstTimelineText(item.Title, "Error"))
		return styles.ErrorShell.Render(header + "\n" + body)
	case TimelineItemProgress:
		return renderTimelineActivity(item, width, styles)
	default:
		if item.Subtype == "welcome" {
			return renderTimelineWelcome(item, width, styles)
		}
		return styles.SystemLine.Width(width).Render(truncateRight(strings.TrimSpace(item.Body), width))
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
	prefix := "· "
	if item.Subtype == "activity.grouped" {
		prefix = "· "
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
	line := prefix + " · " + hint
	return styles.FooterMuted.Render(truncateRight(line, width))
}

func wrapTimelineText(text string, width int) string {
	return strings.TrimRight(wordwrap.String(strings.TrimSpace(text), max(10, width)), "\n")
}

func renderTimelineDecision(item TimelineItem, width, bodyWidth int, styles TimelineStyles) string {
	switch item.Subtype {
	case TimelineSubtypeApprovalApproved:
		body := styles.Body.Width(bodyWidth).Render(item.Body)
		header := styles.SuccessLabel.Render(decisionTitle("✓", firstTimelineText(item.Title, "Approved")))
		return styles.SuccessShell.Render(header + "\n" + body)
	case TimelineSubtypeApprovalRejected:
		if item.Compact {
			return renderCompactTimelineDecision(styles.RejectionLabel, styles.FooterMuted, decisionTitle("✗", firstTimelineText(item.Title, "Rejected")), item.Body, width)
		}
		body := styles.Body.Width(bodyWidth).Render(item.Body)
		header := styles.RejectionLabel.Render(decisionTitle("✗", firstTimelineText(item.Title, "Rejected")))
		return styles.RejectionShell.Render(header + "\n" + body)
	default:
		body := styles.Body.Width(bodyWidth).Render(item.Body)
		if options := strings.TrimSpace(item.DecisionOptions); options != "" {
			body = lipgloss.JoinVertical(lipgloss.Left, body, "", options)
		}
		header := styles.ApprovalLabel.Render(firstTimelineText(item.Title, "Approval Required"))
		return styles.ApprovalShell.Render(header + "\n" + body)
	}
}

func decisionTitle(icon, title string) string {
	title = strings.TrimSpace(title)
	if title == "" || strings.HasPrefix(title, icon) {
		return title
	}
	return icon + " " + title
}

func renderTimelineTimestamp(labelStyle lipgloss.Style, timeStyle lipgloss.Style, label, timestamp string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return timeStyle.Render(timestamp)
	}
	return labelStyle.Render(label) + timeStyle.Render(" · "+timestamp)
}

func renderCompactTimelineDecision(labelStyle, mutedStyle lipgloss.Style, title, body string, width int) string {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(strings.ReplaceAll(body, "\n", " "))
	available := max(0, width-lipgloss.Width(title)-3)
	if body != "" {
		body = truncateRight(body, available)
	}
	line := labelStyle.Render(title)
	if body != "" {
		line += mutedStyle.Render("  " + body)
	}
	return line
}

func firstTimelineText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
