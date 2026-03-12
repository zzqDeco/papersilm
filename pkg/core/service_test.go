package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papersilm/internal/agent"
	"papersilm/internal/config"
	"papersilm/internal/pipeline"
	"papersilm/internal/storage"
	"papersilm/internal/tools"
	"papersilm/pkg/protocol"
)

type testSink struct {
	events []protocol.StreamEvent
}

func (s *testSink) Emit(event protocol.StreamEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestPlanModeCreatesStructuredPlan(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf1 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper1.pdf"), "Paper One")
	pdf2 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper2.pdf"), "Paper Two")

	result, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "compare these papers",
		Sources:        []string{pdf1, pdf2},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}
	if result.Plan == nil {
		t.Fatalf("expected plan")
	}
	if result.Session.Meta.State != protocol.SessionStatePlanned {
		t.Fatalf("unexpected state: %s", result.Session.Meta.State)
	}
	if len(result.Plan.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(result.Plan.Steps))
	}
	if result.Plan.Steps[0].Tool != "distill_paper" || result.Plan.Steps[2].Tool != "compare_papers" {
		t.Fatalf("unexpected steps: %+v", result.Plan.Steps)
	}
	if len(result.Digests) != 0 || result.Comparison != nil {
		t.Fatalf("plan mode should not produce artifacts")
	}
}

func TestConfirmModeInterruptAndApproveResumes(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf1 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper1.pdf"), "Paper One")
	pdf2 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper2.pdf"), "Paper Two")

	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "compare these papers",
		Sources:        []string{pdf1, pdf2},
		PermissionMode: protocol.PermissionModeConfirm,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(confirm): %v", err)
	}
	if planned.Approval == nil {
		t.Fatalf("expected approval payload")
	}
	if planned.Session.Meta.State != protocol.SessionStateAwaitingApproval {
		t.Fatalf("unexpected state: %s", planned.Session.Meta.State)
	}
	resumed, err := svc.Approve(ctx, planned.Session.Meta.SessionID, true, "")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if resumed.Session.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("unexpected state after approve: %s", resumed.Session.Meta.State)
	}
	if len(resumed.Digests) != 2 {
		t.Fatalf("expected 2 digests, got %d: %+v", len(resumed.Digests), resumed.Digests)
	}
	if resumed.Comparison == nil {
		t.Fatalf("expected comparison digest, artifacts=%+v", resumed.Artifacts)
	}
}

func TestRunPlannedExecutesSavedPlan(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Solo")
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "distill this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}

	result, err := svc.RunPlanned(ctx, planned.Session.Meta.SessionID, "zh", "distill")
	if err != nil {
		t.Fatalf("RunPlanned: %v", err)
	}
	if result.Session.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("unexpected state: %s", result.Session.Meta.State)
	}
	if len(result.Digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(result.Digests))
	}
	if result.Comparison != nil {
		t.Fatalf("single paper run should not produce comparison")
	}
}

func newTestService(t *testing.T) (*Service, *testSink) {
	t.Helper()

	cfg := config.Default()
	cfg.BaseDir = t.TempDir()
	store := storage.New(cfg.BaseDir)
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	sink := &testSink{}
	p := pipeline.New(cfg)
	registry := tools.New(p)
	ag := agent.New(registry, cfg)
	return New(store, ag, sink), sink
}

func writeTestPDF(t *testing.T, path, title string) string {
	t.Helper()

	var lines []string
	lines = append(lines, title)
	paragraph := "Abstract We study paper distillation for long academic documents. Method We propose a two stage analysis workflow that extracts problem setup, method details, experiment settings, key numeric results, conclusions, and limitations without spending tokens on general domain background. Experiments We evaluate on Dataset A and Dataset B with consistent prompts and report accuracy 91.2 percent, macro F1 88.3 percent, and latency reductions of 17 percent. Results Our approach outperforms the baseline on all reported metrics and remains stable across ablation settings. Conclusion The method is practical for CLI based paper review. "
	for i := 0; i < 18; i++ {
		lines = append(lines, fmt.Sprintf("%s Section %d.", paragraph, i+1))
	}

	raw := minimalPDF(lines)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func minimalPDF(lines []string) []byte {
	type obj struct {
		id   int
		body string
	}
	var content bytes.Buffer
	content.WriteString("BT\n/F1 12 Tf\n72 760 Td\n")
	for i, line := range lines {
		if i > 0 {
			content.WriteString("0 -18 Td\n")
		}
		content.WriteString("(")
		content.WriteString(escapePDFText(line))
		content.WriteString(") Tj\n")
	}
	content.WriteString("ET")

	objs := []obj{
		{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>"},
		{4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", content.Len(), content.String())},
		{5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"},
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1)
	for _, object := range objs {
		offsets[object.id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", object.id, object.body)
	}
	xrefStart := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objs)+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xrefStart)
	return buf.Bytes()
}

func escapePDFText(in string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`)
	return replacer.Replace(in)
}
