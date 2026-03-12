package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"papersilm/internal/config"
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
		"https://arxiv.org/abs/2501.12345",
		"https://arxiv.org/pdf/2401.00001.pdf",
	})
	if err != nil {
		t.Fatalf("NormalizeSources: %v", err)
	}
	if got, want := len(refs), 3; got != want {
		t.Fatalf("len(refs)=%d want=%d", got, want)
	}
	if refs[0].LocalPath == "" || refs[0].SourceType != "local_pdf" {
		t.Fatalf("unexpected local ref: %+v", refs[0])
	}
	if refs[1].SourceType != "arxiv_abs" {
		t.Fatalf("unexpected arxiv abs ref: %+v", refs[1])
	}
	if refs[2].SourceType != "arxiv_pdf" {
		t.Fatalf("unexpected arxiv pdf ref: %+v", refs[2])
	}
}

