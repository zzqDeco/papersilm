package cli

import (
	tea "github.com/charmbracelet/bubbletea"

	tuiui "github.com/zzqDeco/papersilm/internal/cli/tui"
)

func (m *tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := tuiui.RouteKey(m.keyContexts(), msg.String())
	switch action {
	case tuiui.ActionQuit:
		return m, tea.Quit
	case tuiui.ActionRedraw:
		return m, tea.ClearScreen
	case tuiui.ActionOpenTranscript:
		m.openTranscriptScreen(false)
		m.reflow()
		return m, nil
	case tuiui.ActionOpenHistory:
		cmd := m.openHistorySearch()
		m.reflow()
		return m, cmd
	case tuiui.ActionOpenCommands:
		cmd := m.openCommandPalette()
		m.reflow()
		return m, cmd
	case tuiui.ActionOpenProvider:
		cmd := m.openProviderModal()
		m.reflow()
		return m, cmd
	case tuiui.ActionToggleHints:
		m.toggleHints()
		m.reflow()
		return m, nil
	case tuiui.ActionSuggestionClose:
		m.suggestions = nil
		m.sel = 0
		m.focus = tuiFocusInput
		m.reflow()
		return m, nil
	case tuiui.ActionClosePane:
		m.paneVisible = false
		m.focus = tuiFocusInput
		m.setMainStatus("Pane closed")
		m.reflow()
		return m, nil
	case tuiui.ActionCloseStatus:
		m.setMainStatus("")
		m.reflow()
		return m, nil
	case tuiui.ActionPaneScroll:
		var cmd tea.Cmd
		m.pane, cmd = m.pane.Update(msg)
		return m, cmd
	case tuiui.ActionScrollPage:
		var cmd tea.Cmd
		m.timeline, cmd = m.timeline.Update(msg)
		m.updateScrollState()
		return m, cmd
	case tuiui.ActionJumpBottom:
		m.jumpMainToBottom()
		m.reflow()
		return m, nil
	case tuiui.ActionSuggestionPrev:
		m.focus = tuiFocusSuggestion
		m.sel = clamp(m.sel-1, 0, len(m.suggestions)-1)
		m.reflow()
		return m, nil
	case tuiui.ActionSuggestionNext:
		m.focus = tuiFocusSuggestion
		m.sel = clamp(m.sel+1, 0, len(m.suggestions)-1)
		m.reflow()
		return m, nil
	case tuiui.ActionSuggestionAccept:
		m.applySuggestion(m.suggestions[m.sel])
		m.reflow()
		return m, nil
	case tuiui.ActionApprovalPrev:
		m.moveApprovalSelection(-1)
		m.reflow()
		return m, nil
	case tuiui.ActionApprovalNext:
		m.moveApprovalSelection(1)
		m.reflow()
		return m, nil
	case tuiui.ActionApprovalCommit:
		return m.commitApprovalSelection(msg.String())
	case tuiui.ActionApprovalReject:
		return m.commitApprovalSelection(msg.String())
	case tuiui.ActionApprovalFeedback:
		m.toggleApprovalFeedback()
		m.reflow()
		return m, nil
	case tuiui.ActionApprovalScope:
		m.cycleApprovalScope()
		m.reflow()
		return m, nil
	case tuiui.ActionApprovalExplain:
		m.openApprovalExplanation()
		m.reflow()
		return m, nil
	case tuiui.ActionHistoryPrev:
		if m.shouldUseHistoryUp() {
			m.historyUp()
			m.refreshSuggestions()
			m.reflow()
			return m, nil
		}
	case tuiui.ActionHistoryNext:
		if m.shouldUseHistoryDown() {
			m.historyDown()
			m.refreshSuggestions()
			m.reflow()
			return m, nil
		}
	case tuiui.ActionSubmit:
		return m.submitInput()
	case tuiui.ActionModalClose:
		m.closeModal()
		m.reflow()
		return m, nil
	case tuiui.ActionModalPrev:
		if len(m.modal.Visible) > 0 {
			m.modal.Selection = clamp(m.modal.Selection-1, 0, len(m.modal.Visible)-1)
		}
		m.reflow()
		return m, nil
	case tuiui.ActionModalNext:
		if len(m.modal.Visible) > 0 {
			m.modal.Selection = clamp(m.modal.Selection+1, 0, len(m.modal.Visible)-1)
		}
		m.reflow()
		return m, nil
	case tuiui.ActionModalCommit:
		return m.commitModalSelection()
	case tuiui.ActionTranscriptExit:
		m.closeTranscriptScreen()
		m.reflow()
		return m, nil
	case tuiui.ActionTranscriptSearchOpen:
		m.openTranscriptSearch()
		m.reflow()
		return m, m.searchIn.Focus()
	case tuiui.ActionTranscriptSearchNext:
		m.moveTranscriptSearchSelection(1)
		m.reflow()
		return m, nil
	case tuiui.ActionTranscriptSearchPrev:
		m.moveTranscriptSearchSelection(-1)
		m.reflow()
		return m, nil
	case tuiui.ActionTranscriptScroll:
		var cmd tea.Cmd
		m.transcript, cmd = m.transcript.Update(msg)
		return m, cmd
	case tuiui.ActionTranscriptSearchClose:
		m.closeTranscriptSearch(true)
		m.reflow()
		return m, nil
	case tuiui.ActionTranscriptSearchAccept:
		m.jumpToSearchSelection()
		m.closeTranscriptSearch(false)
		m.reflow()
		return m, nil
	case tuiui.ActionHistorySearchCancel:
		cmd := m.closeHistorySearch(true)
		m.reflow()
		return m, cmd
	case tuiui.ActionHistorySearchClose:
		cmd := m.acceptHistorySearch(false)
		m.reflow()
		return m, cmd
	case tuiui.ActionHistorySearchAccept:
		cmd := m.acceptHistorySearch(true)
		m.reflow()
		if cmd == nil {
			return m, nil
		}
		return m, cmd
	case tuiui.ActionHistorySearchNext:
		m.moveHistorySearchSelection(1)
		m.reflow()
		return m, nil
	case tuiui.ActionHistorySearchPrev:
		m.moveHistorySearchSelection(-1)
		m.reflow()
		return m, nil
	}

	if m.handleApprovalFeedbackInput(msg) {
		m.reflow()
		return m, nil
	}
	if m.approvalKeyboardActive() {
		m.setMainStatus("Permission request active: choose Yes/No or press Tab to add feedback")
		m.reflow()
		return m, nil
	}
	return m.handleTextInput(msg)
}

func (m *tuiModel) keyContexts() []tuiui.KeyContext {
	if m.modal.Kind != tuiModalNone {
		return []tuiui.KeyContext{tuiui.ContextModal, tuiui.ContextGlobal}
	}
	if m.screen == tuiScreenTranscript {
		if m.focus == tuiFocusTranscriptSearch {
			return []tuiui.KeyContext{tuiui.ContextTranscriptSearch, tuiui.ContextTranscript, tuiui.ContextGlobal}
		}
		return []tuiui.KeyContext{tuiui.ContextTranscript, tuiui.ContextGlobal}
	}
	if m.focus == tuiFocusHistorySearch {
		return []tuiui.KeyContext{tuiui.ContextHistorySearch, tuiui.ContextGlobal}
	}
	contexts := make([]tuiui.KeyContext, 0, 4)
	if len(m.suggestions) > 0 {
		contexts = append(contexts, tuiui.ContextAutocomplete)
	}
	if m.paneVisible {
		contexts = append(contexts, tuiui.ContextPane)
	}
	if m.approvalKeyboardActive() {
		contexts = append(contexts, tuiui.ContextConfirmation)
	}
	contexts = append(contexts, tuiui.ContextChat, tuiui.ContextGlobal)
	return contexts
}

func (m *tuiModel) approvalKeyboardActive() bool {
	return m.approvalPanelActive()
}

func isPromptEditingKey(msg tea.KeyMsg) bool {
	if msg.Alt {
		return false
	}
	switch msg.Type {
	case tea.KeyRunes, tea.KeySpace, tea.KeyBackspace, tea.KeyDelete,
		tea.KeyLeft, tea.KeyRight, tea.KeyHome, tea.KeyEnd, tea.KeyCtrlJ:
		return true
	default:
		return false
	}
}

func (m *tuiModel) handleTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal.Kind != tuiModalNone {
		var cmd tea.Cmd
		m.modalIn, cmd = m.modalIn.Update(msg)
		m.refreshModalChoices()
		m.reflow()
		return m, cmd
	}
	if m.focus == tuiFocusHistorySearch {
		var cmd tea.Cmd
		m.historyIn, cmd = m.historyIn.Update(msg)
		m.refreshHistorySearch()
		m.reflow()
		return m, cmd
	}
	if m.focus == tuiFocusTranscriptSearch {
		var cmd tea.Cmd
		m.searchIn, cmd = m.searchIn.Update(msg)
		m.refreshTranscriptSearch()
		m.reflow()
		return m, cmd
	}
	if m.screen == tuiScreenTranscript {
		return m, nil
	}

	var cmd tea.Cmd
	focusCmd := m.input.Focus()
	m.input, cmd = m.input.Update(msg)
	m.focus = tuiFocusInput
	if m.historyState.active {
		m.historyState.active = false
		m.historyState.index = 0
		m.promptController.CancelHistory()
	}
	m.promptController.SetValue(m.input.Value())
	m.refreshSuggestions()
	m.reflow()
	return m, tea.Batch(focusCmd, cmd)
}
