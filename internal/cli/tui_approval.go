package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func (m *tuiModel) approvalPanelActive() bool {
	if m.busy || m.screen != tuiScreenMain || m.focus != tuiFocusInput || m.paneVisible {
		return false
	}
	return m.snapshot.Meta.ApprovalPending || m.snapshot.Meta.State == protocol.SessionStateAwaitingApproval
}

func (m *tuiModel) approvalOptions() []tuiApprovalOption {
	planLabel := firstNonEmpty(m.snapshot.Meta.ActivePlanID, taskBoardPlanID(m.snapshot.TaskBoard), planResultID(m.snapshot.Plan), "current plan")
	pendingTasks := awaitingApprovalTasks(m.snapshot.TaskBoard)
	pendingDetail := fmt.Sprintf("Run %s now", planLabel)
	if len(pendingTasks) > 0 {
		pendingDetail = fmt.Sprintf("Run %d pending %s now", len(pendingTasks), pluralWord(len(pendingTasks), "task"))
	}

	return []tuiApprovalOption{
		{
			Label:   "Approve",
			Detail:  pendingDetail,
			Action:  tuiApprovalApprove,
			Command: "/approve",
		},
		{
			Label:   "Inspect tasks",
			Detail:  "Open the task board before deciding",
			Action:  tuiApprovalInspect,
			Command: "/tasks",
		},
		{
			Label:   "Keep planning",
			Detail:  "Reject this checkpoint without running it",
			Action:  tuiApprovalReject,
			Command: "/reject",
		},
	}
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
}

func (m *tuiModel) syncApprovalSelection() {
	if !m.snapshot.Meta.ApprovalPending && m.snapshot.Meta.State != protocol.SessionStateAwaitingApproval {
		m.approvalSelection = 0
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
		m.approvalSelection = approvalOptionIndex(m.approvalOptions(), tuiApprovalApprove)
	case "i":
		m.approvalSelection = approvalOptionIndex(m.approvalOptions(), tuiApprovalInspect)
	case "r", "n", "esc":
		m.approvalSelection = approvalOptionIndex(m.approvalOptions(), tuiApprovalReject)
	}
}

func approvalOptionIndex(options []tuiApprovalOption, action tuiApprovalAction) int {
	for i, option := range options {
		if option.Action == action && !option.Disabled {
			return i
		}
	}
	return 0
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
	if option.Disabled {
		m.setMainStatus("That approval action is unavailable")
		m.reflow()
		return m, nil
	}

	m.suggestions = nil
	m.sel = 0
	m.historyState.active = false
	m.focus = tuiFocusInput
	m.busy = true
	switch option.Action {
	case tuiApprovalApprove:
		m.setMainStatus("Approving...")
		m.reflow()
		return m, runSlashCmdWithHistory(m.ctx, m.runtime, m.snapshot, option.Command, "", false, true)
	case tuiApprovalInspect:
		m.setMainStatus("Opening tasks...")
		m.reflow()
		return m, runSlashCmdWithHistory(m.ctx, m.runtime, m.snapshot, option.Command, "Tasks", true, true)
	case tuiApprovalReject:
		m.setMainStatus("Keeping plan open...")
		m.reflow()
		return m, runApprovalRejectCmd(m.ctx, m.runtime, m.snapshot)
	default:
		m.busy = false
		m.setMainStatus("Unknown approval action")
		m.reflow()
		return m, nil
	}
}
