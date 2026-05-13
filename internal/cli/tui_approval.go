package cli

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

const (
	tuiPermissionAcceptOnce    = "accept-once"
	tuiPermissionAcceptSession = "accept-session"
	tuiPermissionReject        = "reject"

	tuiPermissionFeedbackAccept = "accept"
	tuiPermissionFeedbackReject = "reject"

	tuiPermissionDetailsPaneTitle = "Permission Details"
)

func (m *tuiModel) approvalPanelActive() bool {
	if m.busy || m.screen != tuiScreenMain {
		return false
	}
	return m.snapshot.Meta.ApprovalPending || m.snapshot.Meta.State == protocol.SessionStateAwaitingApproval || m.richApprovalActive()
}

func (m *tuiModel) richApprovalActive() bool {
	return m.snapshot.Approval != nil && len(m.snapshot.Approval.Requests) > 0
}

func (m *tuiModel) activePermissionRequest() (protocol.PermissionRequest, bool) {
	if m.snapshot.Approval != nil {
		activeID := strings.TrimSpace(m.snapshot.Approval.ActiveRequestID)
		for _, request := range m.snapshot.Approval.Requests {
			if activeID == "" || request.RequestID == activeID {
				return request, true
			}
		}
		if len(m.snapshot.Approval.Requests) > 0 {
			return m.snapshot.Approval.Requests[0], true
		}
	}
	return m.legacyPermissionRequest(), false
}

func (m *tuiModel) legacyPermissionRequest() protocol.PermissionRequest {
	summary := strings.TrimSpace(summarizePendingApproval(m.snapshot))
	if summary == "" {
		summary = "Plan is waiting for approval."
	}
	return protocol.PermissionRequest{
		RequestID: "legacy-approval",
		Tool:      "plan_checkpoint",
		Operation: "plan",
		Title:     "Review plan checkpoint",
		Question:  "Do you want papersilm to proceed?",
		Summary:   summary,
		Options: []protocol.PermissionOption{
			{Value: tuiPermissionAcceptOnce, Label: "Yes", Description: "Run this pending batch now", Scope: "node", Feedback: tuiPermissionFeedbackAccept},
			{Value: tuiPermissionAcceptSession, Label: "Yes, during this session", Description: "Continue this session without asking again for this checkpoint", Scope: "session", Feedback: tuiPermissionFeedbackAccept},
			{Value: tuiPermissionReject, Label: "No", Description: "Reject this checkpoint", Scope: "node", Feedback: tuiPermissionFeedbackReject},
		},
	}
}

func (m *tuiModel) approvalOptions() []protocol.PermissionOption {
	request, _ := m.activePermissionRequest()
	if len(request.Options) > 0 {
		return request.Options
	}
	return m.legacyPermissionRequest().Options
}

func (m *tuiModel) moveApprovalSelection(delta int) {
	options := m.approvalOptions()
	if len(options) == 0 {
		m.approvalSelection = 0
		return
	}
	next := m.approvalSelection + delta
	if next < 0 {
		next = len(options) - 1
	}
	if next >= len(options) {
		next = 0
	}
	m.approvalSelection = next
	m.approvalFeedbackMode = ""
	m.refreshApprovalExplanationPane()
}

func (m *tuiModel) syncApprovalSelection() {
	if !m.approvalPanelActive() {
		m.approvalSelection = 0
		m.approvalFeedbackMode = ""
		m.approvalFeedback = ""
		return
	}
	options := m.approvalOptions()
	if len(options) == 0 {
		m.approvalSelection = 0
		return
	}
	m.approvalSelection = clamp(m.approvalSelection, 0, len(options)-1)
}

func (m *tuiModel) setApprovalSelectionForKey(key string) {
	switch key {
	case "a", "y":
		m.approvalSelection = approvalOptionIndex(m.approvalOptions(), tuiPermissionAcceptOnce)
	case "r", "n", "esc":
		m.approvalSelection = approvalOptionIndex(m.approvalOptions(), tuiPermissionReject)
	}
}

func approvalOptionIndex(options []protocol.PermissionOption, value string) int {
	for i, option := range options {
		if option.Value == value {
			return i
		}
	}
	return 0
}

func (m *tuiModel) toggleApprovalFeedback() {
	options := m.approvalOptions()
	if len(options) == 0 {
		return
	}
	m.approvalSelection = clamp(m.approvalSelection, 0, len(options)-1)
	option := options[m.approvalSelection]
	if strings.TrimSpace(option.Feedback) == "" {
		m.setMainStatus("Selected option does not accept feedback")
		return
	}
	if m.approvalFeedbackMode == option.Feedback {
		m.approvalFeedbackMode = ""
		return
	}
	m.approvalFeedbackMode = option.Feedback
}

func (m *tuiModel) cycleApprovalScope() {
	options := m.approvalOptions()
	if len(options) == 0 {
		return
	}
	current := options[clamp(m.approvalSelection, 0, len(options)-1)]
	if current.Feedback == tuiPermissionFeedbackAccept {
		for i := 1; i <= len(options); i++ {
			next := (m.approvalSelection + i) % len(options)
			if options[next].Feedback == tuiPermissionFeedbackAccept && options[next].Scope != current.Scope {
				m.approvalSelection = next
				m.approvalFeedbackMode = ""
				m.refreshApprovalExplanationPane()
				return
			}
		}
	}
	for i := 1; i <= len(options); i++ {
		next := (m.approvalSelection + i) % len(options)
		if options[next].Value == current.Value && options[next].Scope != current.Scope {
			m.approvalSelection = next
			m.approvalFeedbackMode = ""
			m.refreshApprovalExplanationPane()
			return
		}
	}
	m.setMainStatus("No alternate scope for this decision")
}

func (m *tuiModel) openApprovalExplanation() {
	m.openPane(tuiPermissionDetailsPaneTitle, m.permissionDetailPaneBody())
}

func (m *tuiModel) toggleApprovalExplanation() {
	if m.paneVisible && m.paneTitle == tuiPermissionDetailsPaneTitle {
		m.paneVisible = false
		m.focus = tuiFocusInput
		m.setMainStatus("Permission details closed")
		return
	}
	m.openApprovalExplanation()
}

func (m *tuiModel) refreshApprovalExplanationPane() {
	if !m.paneVisible || m.paneTitle != tuiPermissionDetailsPaneTitle {
		return
	}
	m.paneBody = m.permissionDetailPaneBody()
	m.pane.SetContent(m.renderPaneBody(max(20, m.width-8)))
}

func (m *tuiModel) permissionDetailPaneBody() string {
	request, _ := m.activePermissionRequest()
	options := m.approvalOptions()
	selected := 0
	if len(options) > 0 {
		selected = clamp(m.approvalSelection, 0, len(options)-1)
	}
	return permissionDetailPaneText(request, options, selected)
}

func permissionPreviewText(request protocol.PermissionRequest) string {
	switch request.Preview.Kind {
	case "diff":
		if strings.TrimSpace(request.Preview.Diff) != "" {
			return strings.TrimSpace(request.Preview.Diff)
		}
		if strings.TrimSpace(request.Preview.Summary) != "" {
			return strings.TrimSpace(request.Preview.Summary)
		}
	case "command":
		lines := make([]string, 0, 4)
		if strings.TrimSpace(request.Command) != "" {
			lines = append(lines, "$ "+strings.TrimSpace(request.Command))
		}
		if strings.TrimSpace(request.Preview.Summary) != "" {
			lines = append(lines, strings.TrimSpace(request.Preview.Summary))
		}
		if strings.TrimSpace(request.Preview.CommandPrefix) != "" {
			lines = append(lines, "session scope: "+strings.TrimSpace(request.Preview.CommandPrefix))
		}
		if len(lines) > 0 {
			return strings.Join(lines, "\n")
		}
	case "error":
		if strings.TrimSpace(request.Preview.Summary) != "" {
			return "Preview error: " + strings.TrimSpace(request.Preview.Summary)
		}
	}
	if strings.TrimSpace(request.Preview.NewContent) != "" {
		return strings.TrimSpace(request.Preview.NewContent)
	}
	if strings.TrimSpace(request.Preview.Summary) != "" {
		return strings.TrimSpace(request.Preview.Summary)
	}
	return ""
}

func permissionDetailPaneText(request protocol.PermissionRequest, options []protocol.PermissionOption, selected int) string {
	lines := []string{firstNonEmpty(request.Title, "Permission request")}
	if text := strings.TrimSpace(request.Question); text != "" {
		lines = append(lines, text)
	}
	if text := strings.TrimSpace(request.Subtitle); text != "" {
		lines = appendPermissionDetailSection(lines, "Context", text)
	}
	if text := strings.TrimSpace(request.Summary); text != "" {
		lines = appendPermissionDetailSection(lines, "Summary", text)
	}

	if tool := permissionToolLine(request); tool != "" {
		lines = appendPermissionDetailSection(lines, "Tool", tool)
	}

	switch request.Preview.Kind {
	case "command":
		lines = appendCommandPermissionDetails(lines, request)
	case "diff":
		lines = appendEditPermissionDetails(lines, request)
	default:
		lines = appendGenericPermissionDetails(lines, request)
	}

	if len(options) > 0 {
		selected = clamp(selected, 0, len(options)-1)
		lines = appendPermissionDetailSection(lines, "Decision", permissionDecisionDetail(options, selected))
	}

	lines = appendPermissionDetailSection(lines, "Keys", "Esc/Ctrl+E close details · Enter/Y allow · N reject · Tab feedback · Shift+Tab scope")
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func appendCommandPermissionDetails(lines []string, request protocol.PermissionRequest) []string {
	if command := strings.TrimSpace(request.Command); command != "" {
		lines = appendPermissionDetailSection(lines, "Command", "$ "+command)
	}
	if cwd := strings.TrimSpace(request.Preview.Summary); cwd != "" {
		lines = appendPermissionDetailSection(lines, "Working directory", cwd)
	}
	if prefix := strings.TrimSpace(request.Preview.CommandPrefix); prefix != "" {
		lines = appendPermissionDetailSection(lines, "Session scope", prefix)
	}
	return appendPermissionDetailSection(lines, "Risk", "Shell commands can read, write, or execute workspace files. Only allow commands you trust.")
}

func appendEditPermissionDetails(lines []string, request protocol.PermissionRequest) []string {
	if target := strings.TrimSpace(request.TargetPath); target != "" {
		lines = appendPermissionDetailSection(lines, "Target", target)
	}
	if diff := strings.TrimSpace(request.Preview.Diff); diff != "" {
		return appendPermissionDetailSection(lines, "Diff preview", diff)
	}
	if preview := permissionPreviewText(request); preview != "" {
		return appendPermissionDetailSection(lines, "Preview", preview)
	}
	return lines
}

func appendGenericPermissionDetails(lines []string, request protocol.PermissionRequest) []string {
	if target := strings.TrimSpace(request.TargetPath); target != "" {
		lines = appendPermissionDetailSection(lines, "Target", target)
	}
	if command := strings.TrimSpace(request.Command); command != "" {
		lines = appendPermissionDetailSection(lines, "Command", "$ "+command)
	}
	if preview := permissionPreviewText(request); preview != "" {
		lines = appendPermissionDetailSection(lines, "Preview", preview)
	}
	return lines
}

func appendPermissionDetailSection(lines []string, title string, body string) []string {
	body = strings.TrimSpace(body)
	if body == "" {
		return lines
	}
	return append(lines, "", title, body)
}

func permissionToolLine(request protocol.PermissionRequest) string {
	parts := make([]string, 0, 2)
	if tool := strings.TrimSpace(request.Tool); tool != "" {
		parts = append(parts, tool)
	}
	if operation := strings.TrimSpace(request.Operation); operation != "" {
		parts = append(parts, operation)
	}
	return strings.Join(parts, " · ")
}

func permissionDecisionDetail(options []protocol.PermissionOption, selected int) string {
	if len(options) == 0 {
		return ""
	}
	selected = clamp(selected, 0, len(options)-1)
	lines := []string{"selected: " + permissionOptionDetail(options[selected])}
	for _, option := range options {
		lines = append(lines, "  "+permissionOptionDetail(option))
	}
	return strings.Join(lines, "\n")
}

func permissionOptionDetail(option protocol.PermissionOption) string {
	parts := []string{firstNonEmpty(strings.TrimSpace(option.Label), strings.TrimSpace(option.Value))}
	if scope := strings.TrimSpace(option.Scope); scope != "" {
		parts = append(parts, scope)
	}
	if feedback := strings.TrimSpace(option.Feedback); feedback != "" {
		parts = append(parts, "feedback:"+feedback)
	}
	if description := strings.TrimSpace(option.Description); description != "" {
		parts = append(parts, description)
	}
	return strings.Join(parts, " · ")
}

func (m *tuiModel) handleApprovalFeedbackInput(msg tea.KeyMsg) bool {
	if !m.approvalKeyboardActive() || m.approvalFeedbackMode == "" {
		return false
	}
	switch msg.Type {
	case tea.KeyRunes:
		m.approvalFeedback += string(msg.Runes)
	case tea.KeySpace:
		m.approvalFeedback += " "
	case tea.KeyBackspace:
		if len(m.approvalFeedback) > 0 {
			runes := []rune(m.approvalFeedback)
			m.approvalFeedback = string(runes[:len(runes)-1])
		}
	case tea.KeyDelete:
		// Delete behaves like backspace because feedback uses a simple append buffer.
		if len(m.approvalFeedback) > 0 {
			runes := []rune(m.approvalFeedback)
			m.approvalFeedback = string(runes[:len(runes)-1])
		}
	case tea.KeyCtrlJ:
		m.approvalFeedback += "\n"
	default:
		return false
	}
	return true
}

func (m *tuiModel) shouldCaptureApprovalFeedbackKey(msg tea.KeyMsg) bool {
	if !m.approvalKeyboardActive() || m.approvalFeedbackMode == "" {
		return false
	}
	switch msg.Type {
	case tea.KeyRunes, tea.KeySpace, tea.KeyBackspace, tea.KeyDelete, tea.KeyCtrlJ:
		return true
	default:
		return false
	}
}

func (m *tuiModel) commitApprovalSelection(key string) (tea.Model, tea.Cmd) {
	if m.busy {
		m.setMainStatus("A run is already in progress")
		m.reflow()
		return m, nil
	}
	m.setApprovalSelectionForKey(key)
	options := m.approvalOptions()
	if len(options) == 0 {
		return m, nil
	}
	m.approvalSelection = clamp(m.approvalSelection, 0, len(options)-1)
	option := options[m.approvalSelection]

	m.suggestions = nil
	m.sel = 0
	m.historyState.active = false
	m.focus = tuiFocusInput
	m.busy = true
	m.approvalFeedbackMode = ""

	feedback := strings.TrimSpace(m.approvalFeedback)
	m.approvalFeedback = ""

	if !m.richApprovalActive() {
		switch option.Value {
		case tuiPermissionReject:
			m.setMainStatus("Rejecting...")
			m.reflow()
			return m, runApprovalRejectCmd(m.ctx, m.runtime, m.snapshot, feedback)
		default:
			m.setMainStatus("Approving...")
			m.reflow()
			return m, runSlashCmdWithHistory(m.ctx, m.runtime, m.snapshot, "/approve", "", false, true)
		}
	}

	request, _ := m.activePermissionRequest()
	decision := protocol.PermissionDecision{
		RequestID: request.RequestID,
		Value:     option.Value,
		Scope:     option.Scope,
		Feedback:  feedback,
	}
	switch option.Value {
	case tuiPermissionReject:
		m.setMainStatus("Rejecting tool use...")
	case tuiPermissionAcceptSession:
		m.setMainStatus("Allowing during this session...")
	default:
		m.setMainStatus("Allowing once...")
	}
	m.reflow()
	return m, runPermissionDecisionCmd(m.ctx, m.runtime, m.snapshot, decision)
}

func permissionDecisionInput(decision protocol.PermissionDecision) string {
	input := "/permission " + strings.TrimSpace(decision.Value)
	if strings.TrimSpace(decision.Scope) != "" {
		input += " " + strings.TrimSpace(decision.Scope)
	}
	if strings.TrimSpace(decision.Feedback) != "" {
		input += " -- " + strings.TrimSpace(decision.Feedback)
	}
	return input
}

func runPermissionDecisionCmd(ctx context.Context, runtime *tuiRuntimeManager, snapshot protocol.SessionSnapshot, decision protocol.PermissionDecision) tea.Cmd {
	return func() tea.Msg {
		before := snapshot
		result, err := runtime.svc.DecidePermission(ctx, snapshot.Meta.SessionID, decision)
		after := snapshot
		text := fmt.Sprintf("Permission decision: %s", decision.Value)
		if err == nil {
			after = result.Session
		}
		return tuiExecDoneMsg{
			Input:       permissionDecisionInput(decision),
			SkipHistory: true,
			Before:      before,
			After:       after,
			Text:        text,
			Err:         err,
		}
	}
}
