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
	for _, want := range []string{"│ No and tell papersilm what to do differently", "│ › use tests only"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in permission dialog, got %q", want, rendered)
		}
	}
}

func TestRenderPermissionDialogUsesAmendPlaceholder(t *testing.T) {
	t.Parallel()

	rendered := RenderPermissionDialog(PermissionDialog{
		Width:               50,
		Title:               "Run command",
		FeedbackMode:        "accept",
		FeedbackLabel:       "Yes and tell papersilm what to do next",
		FeedbackPlaceholder: "Add optional feedback",
	})
	for _, want := range []string{"│ Yes and tell papersilm what to do next", "│ › Add optional feedback"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in amend placeholder, got %q", want, rendered)
		}
	}
}

func TestRenderPermissionDialogLimitsFeedbackHeight(t *testing.T) {
	t.Parallel()

	rendered := RenderPermissionDialog(PermissionDialog{
		Width:         50,
		Title:         "Run command",
		FeedbackMode:  "reject",
		FeedbackLabel: "No and tell papersilm what to do differently",
		Feedback:      "one\ntwo\nthree\nfour\nfive",
	})
	if strings.Contains(rendered, "five") || !strings.Contains(rendered, "│ …") {
		t.Fatalf("expected feedback text to truncate after four lines, got %q", rendered)
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

func TestRenderPermissionDialogHonorsPreviewMaxLines(t *testing.T) {
	t.Parallel()

	rendered := RenderPermissionDialog(PermissionDialog{
		Width:           80,
		Title:           "Edit file",
		PreviewKind:     "diff",
		PreviewMaxLines: 3,
		Preview:         "--- README.md\n+++ README.md\n-old line\n+new line\n+another line",
	})
	if strings.Contains(rendered, "+new line") || !strings.Contains(rendered, "│ …") {
		t.Fatalf("expected preview to truncate after max lines, got %q", rendered)
	}
}

func TestRenderPermissionDialogUsesCompactOptionRows(t *testing.T) {
	t.Parallel()

	rendered := RenderPermissionDialog(PermissionDialog{
		Width:    80,
		Title:    "Run command",
		Question: "Do you want to run this command?",
		Rows: []ListRow{
			{
				Label:          "Yes, during this session",
				Detail:         "prefix go test · Allow this command prefix for this session",
				Selected:       true,
				SelectedPrefix: "❯ ",
			},
			{Label: "No", Detail: "Reject this tool use"},
		},
	})
	for _, want := range []string{"❯ Yes, during this session", "prefix go test", "No – Reject this tool use"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in compact permission dialog, got %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "Yes, during this session      ") {
		t.Fatalf("expected compact permission rows instead of padded table rows, got %q", rendered)
	}
}

func TestRenderPermissionDialogKeeps80ColumnPromptCompact(t *testing.T) {
	t.Parallel()

	rendered := RenderPermissionDialog(PermissionDialog{
		Width:           80,
		Title:           "Edit file",
		Question:        "Do you want to make this workspace edit?",
		PreviewKind:     "diff",
		PreviewMaxLines: 4,
		Preview:         "--- README.md\n+++ README.md\n-old line\n+new line\n+another line\n+more context",
		Rows: []ListRow{
			{Label: "Yes", Detail: "Allow this tool use once", SelectedPrefix: "❯ "},
			{Label: "Yes, during this session", Detail: "path README.md · Allow edits to this file for this session", Selected: true, SelectedPrefix: "❯ "},
			{Label: "No", Detail: "Reject this tool use", SelectedPrefix: "❯ "},
		},
		Hint: "Enter yes · N no · Tab amend · Shift+Tab scope · Ctrl+E details",
	})
	if got := strings.Count(rendered, "\n") + 1; got > 12 {
		t.Fatalf("expected compact 80-column permission prompt, got %d lines:\n%s", got, rendered)
	}
	for _, want := range []string{"│ …", "path README.md", "Tab amend"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in compact prompt, got %q", want, rendered)
		}
	}
}
