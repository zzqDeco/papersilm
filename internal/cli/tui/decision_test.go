package tui

import (
	"strings"
	"testing"
)

func TestRenderDecisionPanelUsesSingleDividerAndRows(t *testing.T) {
	t.Parallel()

	rendered := RenderDecisionPanel(DecisionPanel{
		Width:   40,
		Title:   "Approval required",
		Summary: "Plan is waiting",
		Hint:    "Y approve · N reject",
		Rows: []ListRow{
			{Label: "Approve", Detail: "Run plan", Selected: true, SelectedPrefix: "❯ "},
			{Label: "Keep planning", Detail: "Reject"},
		},
	})
	if !strings.Contains(rendered, strings.Repeat("─", 40)) {
		t.Fatalf("expected divider, got %q", rendered)
	}
	for _, want := range []string{"Approval required", "❯ Approve", " – Run plan", "Keep planning", "Y approve"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in decision panel, got %q", want, rendered)
		}
	}
	if strings.ContainsAny(rendered, "┌┐└┘") {
		t.Fatalf("expected no box corners, got %q", rendered)
	}
}

func TestRenderPermissionDialogUsesFeedbackLabel(t *testing.T) {
	t.Parallel()

	rendered := RenderPermissionDialog(PermissionDialog{
		Width:         50,
		Title:         "Run command",
		Question:      "Do you want to run this command?",
		FeedbackMode:  "reject",
		FeedbackLabel: "No and tell papersilm what to do differently",
		Feedback:      "use tests only",
	})
	for _, want := range []string{"No and tell papersilm what to do differently", "› use tests only"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in permission dialog, got %q", want, rendered)
		}
	}
}

func TestRenderPermissionDialogUsesDiffPreviewHierarchy(t *testing.T) {
	t.Parallel()

	rendered := RenderPermissionDialog(PermissionDialog{
		Width:       60,
		Title:       "Edit file",
		PreviewKind: "diff",
		Preview:     "--- README.md\n+++ README.md\n-old line\n+new line",
	})
	for _, want := range []string{"│ --- README.md", "│ +++ README.md", "│ -old line", "│ +new line"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in diff preview, got %q", want, rendered)
		}
	}
}

func TestRenderPermissionDialogUsesCommandPreviewHierarchy(t *testing.T) {
	t.Parallel()

	rendered := RenderPermissionDialog(PermissionDialog{
		Width:       60,
		Title:       "Run command",
		PreviewKind: "command",
		Preview:     "$ go test ./...\ncwd: /tmp/workspace\nsession scope: go test",
	})
	for _, want := range []string{"│ $ go test ./...", "│ cwd: /tmp/workspace", "│ session scope: go test"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in command preview, got %q", want, rendered)
		}
	}
}
