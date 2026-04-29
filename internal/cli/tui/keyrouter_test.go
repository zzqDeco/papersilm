package tui

import "testing"

func TestRouteKeyUsesTopMostContext(t *testing.T) {
	t.Parallel()

	action := RouteKey([]KeyContext{ContextAutocomplete, ContextPane, ContextChat, ContextGlobal}, "esc")
	if action != ActionSuggestionClose {
		t.Fatalf("expected autocomplete Esc to win, got %q", action)
	}

	action = RouteKey([]KeyContext{ContextPane, ContextChat, ContextGlobal}, "esc")
	if action != ActionClosePane {
		t.Fatalf("expected pane Esc after autocomplete closes, got %q", action)
	}
}

func TestRouteKeyFallsThroughToChat(t *testing.T) {
	t.Parallel()

	action := RouteKey([]KeyContext{ContextPane, ContextChat, ContextGlobal}, "enter")
	if action != ActionSubmit {
		t.Fatalf("expected chat submit fallback, got %q", action)
	}
}

func TestRouteKeyApprovalSelectsBeforeChat(t *testing.T) {
	t.Parallel()

	action := RouteKey([]KeyContext{ContextApproval, ContextChat, ContextGlobal}, "enter")
	if action != ActionApprovalCommit {
		t.Fatalf("expected approval commit before chat submit, got %q", action)
	}

	action = RouteKey([]KeyContext{ContextApproval, ContextChat, ContextGlobal}, "down")
	if action != ActionApprovalNext {
		t.Fatalf("expected approval next before chat history, got %q", action)
	}
}

func TestRouteKeyGlobalQuitAlwaysAvailable(t *testing.T) {
	t.Parallel()

	action := RouteKey([]KeyContext{ContextModal, ContextGlobal}, "ctrl+c")
	if action != ActionQuit {
		t.Fatalf("expected global quit, got %q", action)
	}

	action = RouteKey([]KeyContext{ContextChat, ContextGlobal}, "ctrl+d")
	if action != ActionQuit {
		t.Fatalf("expected ctrl+d exit, got %q", action)
	}

	action = RouteKey([]KeyContext{ContextChat, ContextGlobal}, "ctrl+l")
	if action != ActionRedraw {
		t.Fatalf("expected ctrl+l redraw, got %q", action)
	}

	action = RouteKey([]KeyContext{ContextTranscript, ContextGlobal}, "ctrl+c")
	if action != ActionQuit {
		t.Fatalf("expected ctrl+c to quit from transcript, got %q", action)
	}
}

func TestRouteKeyConfirmationRejectsLikeClaudeCode(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"n", "esc"} {
		action := RouteKey([]KeyContext{ContextApproval, ContextChat, ContextGlobal}, key)
		if action != ActionApprovalReject {
			t.Fatalf("expected %s to reject approval, got %q", key, action)
		}
	}
}
