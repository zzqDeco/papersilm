package tui

import (
	"testing"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestPermissionStatePreservesDraftAcrossSameRequest(t *testing.T) {
	t.Parallel()

	request := testPermissionRequest("req_1")
	var state PermissionState
	state.Sync(true, request)
	state.Selection = 1
	state.FeedbackMode = "accept"
	state.Feedback = "use the shorter path"

	state.Sync(true, request)
	if state.Selection != 1 || state.FeedbackMode != "accept" || state.Feedback != "use the shorter path" {
		t.Fatalf("expected same request sync to preserve state, got %+v", state)
	}
}

func TestPermissionStateResetsOnNewRequest(t *testing.T) {
	t.Parallel()

	var state PermissionState
	state.Sync(true, testPermissionRequest("req_1"))
	state.Selection = 2
	state.FeedbackMode = "reject"
	state.Feedback = "try something else"
	state.DetailsOpen = true

	state.Sync(true, testPermissionRequest("req_2"))
	if state.Selection != 0 || state.FeedbackMode != "" || state.Feedback != "" || state.DetailsOpen {
		t.Fatalf("expected new request sync to reset local state, got %+v", state)
	}
}

func TestPermissionStateResetsWhenPreviewChanges(t *testing.T) {
	t.Parallel()

	request := testPermissionRequest("req_1")
	request.Preview = protocol.PermissionPreview{
		Kind: "diff",
		Diff: "-old\n+new",
	}
	var state PermissionState
	state.Sync(true, request)
	state.Selection = 2
	state.FeedbackMode = "reject"
	state.Feedback = "do not edit this way"

	request.Preview.Diff = "-old\n+different"
	state.Sync(true, request)
	if state.Selection != 0 || state.FeedbackMode != "" || state.Feedback != "" {
		t.Fatalf("expected preview change to reset stale decision state, got %+v", state)
	}
}

func TestPermissionStateResetsWhenOptionsChange(t *testing.T) {
	t.Parallel()

	request := testPermissionRequest("req_1")
	var state PermissionState
	state.Sync(true, request)
	state.Selection = 1
	state.FeedbackMode = "accept"
	state.Feedback = "continue with tests"

	request.Options = []protocol.PermissionOption{
		{Value: "reject", Label: "No", Scope: "node", Feedback: "reject"},
		{Value: "accept-once", Label: "Yes", Scope: "node", Feedback: "accept"},
	}
	state.Sync(true, request)
	if state.Selection != 0 || state.FeedbackMode != "" || state.Feedback != "" {
		t.Fatalf("expected option changes to reset stale decision state, got %+v", state)
	}
}

func TestPermissionStateMovesFeedbackAndScope(t *testing.T) {
	t.Parallel()

	var state PermissionState
	state.Sync(true, testPermissionRequest("req_1"))
	state.MoveSelection(-1)
	if option, _ := state.SelectedOption(); option.Value != "reject" {
		t.Fatalf("expected wrapped selection to reject, got %+v", option)
	}
	if !state.ToggleFeedback() || state.FeedbackMode != "reject" {
		t.Fatalf("expected reject feedback mode, got %+v", state)
	}
	state.AppendText("no")
	state.Backspace()
	state.Newline()
	state.AppendText("use tests")
	if got := state.ConsumeFeedback(); got != "n\nuse tests" {
		t.Fatalf("expected consumed feedback, got %q", got)
	}

	state.SelectValue("accept-once")
	if !state.CycleScope("accept") {
		t.Fatalf("expected scope cycle to find accept-session")
	}
	if option, _ := state.SelectedOption(); option.Value != "accept-session" {
		t.Fatalf("expected accept-session after scope cycle, got %+v", option)
	}
}

func testPermissionRequest(id string) protocol.PermissionRequest {
	return protocol.PermissionRequest{
		RequestID: id,
		Tool:      "workspace_command",
		Title:     "Run command",
		Command:   "go test ./...",
		Options: []protocol.PermissionOption{
			{Value: "accept-once", Label: "Yes", Scope: "node", Feedback: "accept"},
			{Value: "accept-session", Label: "Yes, during this session", Scope: "command-prefix", Feedback: "accept"},
			{Value: "reject", Label: "No", Scope: "node", Feedback: "reject"},
		},
	}
}
