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
			{Label: "Approve", Detail: "Run plan", Selected: true},
			{Label: "Keep planning", Detail: "Reject"},
		},
	})
	if !strings.Contains(rendered, strings.Repeat("▔", 40)) {
		t.Fatalf("expected divider, got %q", rendered)
	}
	for _, want := range []string{"Approval required", "+ Approve", "Keep planning", "Y approve"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in decision panel, got %q", want, rendered)
		}
	}
	if strings.ContainsAny(rendered, "┌┐└┘") {
		t.Fatalf("expected no box corners, got %q", rendered)
	}
}
