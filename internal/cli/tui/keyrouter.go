package tui

type KeyContext string

const (
	ContextGlobal           KeyContext = "global"
	ContextChat             KeyContext = "chat"
	ContextAutocomplete     KeyContext = "autocomplete"
	ContextApproval         KeyContext = "approval"
	ContextConfirmation     KeyContext = "confirmation"
	ContextPane             KeyContext = "pane"
	ContextModal            KeyContext = "modal"
	ContextTranscript       KeyContext = "transcript"
	ContextTranscriptSearch KeyContext = "transcript_search"
	ContextHistorySearch    KeyContext = "history_search"
	ContextScroll           KeyContext = "scroll"
	ContextFooter           KeyContext = "footer"
)

type KeyAction string

const (
	ActionInput KeyAction = "input"

	ActionQuit           KeyAction = "quit"
	ActionRedraw         KeyAction = "redraw"
	ActionToggleHints    KeyAction = "toggle_hints"
	ActionOpenTranscript KeyAction = "open_transcript"
	ActionOpenHistory    KeyAction = "open_history"
	ActionOpenCommands   KeyAction = "open_commands"
	ActionOpenProvider   KeyAction = "open_provider"

	ActionSubmit      KeyAction = "submit"
	ActionCloseStatus KeyAction = "close_status"
	ActionJumpBottom  KeyAction = "jump_bottom"
	ActionScrollPage  KeyAction = "scroll_page"
	ActionHistoryPrev KeyAction = "history_prev"
	ActionHistoryNext KeyAction = "history_next"
	ActionClosePane   KeyAction = "close_pane"
	ActionPaneScroll  KeyAction = "pane_scroll"

	ActionSuggestionPrev   KeyAction = "suggestion_prev"
	ActionSuggestionNext   KeyAction = "suggestion_next"
	ActionSuggestionAccept KeyAction = "suggestion_accept"
	ActionSuggestionClose  KeyAction = "suggestion_close"

	ActionApprovalPrev   KeyAction = "approval_prev"
	ActionApprovalNext   KeyAction = "approval_next"
	ActionApprovalCommit KeyAction = "approval_commit"
	ActionApprovalReject KeyAction = "approval_reject"

	ActionModalClose  KeyAction = "modal_close"
	ActionModalPrev   KeyAction = "modal_prev"
	ActionModalNext   KeyAction = "modal_next"
	ActionModalCommit KeyAction = "modal_commit"

	ActionTranscriptExit       KeyAction = "transcript_exit"
	ActionTranscriptSearchOpen KeyAction = "transcript_search_open"
	ActionTranscriptSearchNext KeyAction = "transcript_search_next"
	ActionTranscriptSearchPrev KeyAction = "transcript_search_prev"
	ActionTranscriptScroll     KeyAction = "transcript_scroll"

	ActionTranscriptSearchClose  KeyAction = "transcript_search_close"
	ActionTranscriptSearchAccept KeyAction = "transcript_search_accept"

	ActionHistorySearchCancel KeyAction = "history_search_cancel"
	ActionHistorySearchClose  KeyAction = "history_search_close"
	ActionHistorySearchAccept KeyAction = "history_search_accept"
	ActionHistorySearchNext   KeyAction = "history_search_next"
	ActionHistorySearchPrev   KeyAction = "history_search_prev"

	ActionFooterPrev   KeyAction = "footer_prev"
	ActionFooterNext   KeyAction = "footer_next"
	ActionFooterCommit KeyAction = "footer_commit"
	ActionFooterClose  KeyAction = "footer_close"
)

func RouteKey(contexts []KeyContext, key string) KeyAction {
	for _, context := range contexts {
		if action, ok := routeContextKey(context, key); ok {
			return action
		}
	}
	return ActionInput
}

func routeContextKey(context KeyContext, key string) (KeyAction, bool) {
	switch context {
	case ContextGlobal:
		switch key {
		case "ctrl+c", "ctrl+d":
			return ActionQuit, true
		case "ctrl+l":
			return ActionRedraw, true
		case "ctrl+/", "ctrl+_":
			return ActionToggleHints, true
		}
	case ContextModal:
		switch key {
		case "esc":
			return ActionModalClose, true
		case "up":
			return ActionModalPrev, true
		case "down":
			return ActionModalNext, true
		case "enter":
			return ActionModalCommit, true
		}
	case ContextTranscriptSearch:
		switch key {
		case "esc":
			return ActionTranscriptSearchClose, true
		case "enter":
			return ActionTranscriptSearchAccept, true
		case "up", "pgup":
			return ActionTranscriptSearchPrev, true
		case "down", "pgdown":
			return ActionTranscriptSearchNext, true
		}
	case ContextTranscript:
		switch key {
		case "ctrl+o", "ctrl+c", "q", "esc":
			return ActionTranscriptExit, true
		case "/":
			return ActionTranscriptSearchOpen, true
		case "n":
			return ActionTranscriptSearchNext, true
		case "N":
			return ActionTranscriptSearchPrev, true
		case "up", "pgup", "down", "pgdown":
			return ActionTranscriptScroll, true
		}
	case ContextHistorySearch:
		switch key {
		case "ctrl+c":
			return ActionHistorySearchCancel, true
		case "esc", "tab":
			return ActionHistorySearchClose, true
		case "enter":
			return ActionHistorySearchAccept, true
		case "ctrl+r", "down":
			return ActionHistorySearchNext, true
		case "up":
			return ActionHistorySearchPrev, true
		}
	case ContextAutocomplete:
		switch key {
		case "esc":
			return ActionSuggestionClose, true
		case "up":
			return ActionSuggestionPrev, true
		case "down":
			return ActionSuggestionNext, true
		case "tab":
			return ActionSuggestionAccept, true
		}
	case ContextApproval:
		switch key {
		case "up", "left", "shift+tab":
			return ActionApprovalPrev, true
		case "down", "right", "tab":
			return ActionApprovalNext, true
		case "enter", "a", "y", "r", "i":
			return ActionApprovalCommit, true
		case "n", "esc":
			return ActionApprovalReject, true
		}
	case ContextConfirmation:
		switch key {
		case "y", "enter":
			return ActionApprovalCommit, true
		case "n", "esc":
			return ActionApprovalReject, true
		case "up", "left", "shift+tab":
			return ActionApprovalPrev, true
		case "down", "right", "tab", "space":
			return ActionApprovalNext, true
		}
	case ContextPane:
		switch key {
		case "esc":
			return ActionClosePane, true
		case "pgup", "pgdown":
			return ActionPaneScroll, true
		}
	case ContextChat:
		switch key {
		case "ctrl+o":
			return ActionOpenTranscript, true
		case "ctrl+r":
			return ActionOpenHistory, true
		case "ctrl+k":
			return ActionOpenCommands, true
		case "alt+p", "meta+p":
			return ActionOpenProvider, true
		case "esc":
			return ActionCloseStatus, true
		case "end":
			return ActionJumpBottom, true
		case "pgup", "pgdown", "wheelup", "wheeldown":
			return ActionScrollPage, true
		case "enter":
			return ActionSubmit, true
		case "up":
			return ActionHistoryPrev, true
		case "down":
			return ActionHistoryNext, true
		}
	case ContextFooter:
		switch key {
		case "left", "up", "ctrl+p":
			return ActionFooterPrev, true
		case "right", "down", "ctrl+n":
			return ActionFooterNext, true
		case "enter":
			return ActionFooterCommit, true
		case "esc":
			return ActionFooterClose, true
		}
	case ContextScroll:
		switch key {
		case "pgup", "pgdown", "wheelup", "wheeldown", "ctrl+home", "ctrl+end":
			return ActionScrollPage, true
		}
	}
	return "", false
}
