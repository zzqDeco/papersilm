package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zzqDeco/papersilm/internal/config"
)

func TestNormalizeSources(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "paper.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatalf("write temp pdf: %v", err)
	}

	svc := New(config.Default())
	refs, err := svc.NormalizeSources(context.Background(), "sess_test", []string{
		pdfPath,
		"2401.12345v2",
		"https://arxiv.org/abs/2501.12345",
		"https://arxiv.org/pdf/2401.00001.pdf",
		"https://alphaxiv.org/overview/1706.03762",
		"https://alphaxiv.org/abs/1706.03762",
	})
	if err != nil {
		t.Fatalf("NormalizeSources: %v", err)
	}
	if got, want := len(refs), 6; got != want {
		t.Fatalf("len(refs)=%d want=%d", got, want)
	}
	if refs[0].LocalPath == "" || refs[0].SourceType != "local_pdf" {
		t.Fatalf("unexpected local ref: %+v", refs[0])
	}
	if refs[1].SourceType != "paper_id" || refs[1].ResolvedPaperID != "2401.12345v2" {
		t.Fatalf("unexpected paper id ref: %+v", refs[1])
	}
	if refs[2].SourceType != "arxiv_abs" {
		t.Fatalf("unexpected arxiv abs ref: %+v", refs[1])
	}
	if refs[3].SourceType != "arxiv_pdf" {
		t.Fatalf("unexpected arxiv pdf ref: %+v", refs[3])
	}
	if refs[4].SourceType != "alphaxiv_overview" {
		t.Fatalf("unexpected alphaxiv overview ref: %+v", refs[4])
	}
	if refs[5].SourceType != "alphaxiv_abs" {
		t.Fatalf("unexpected alphaxiv abs ref: %+v", refs[5])
	}
}
