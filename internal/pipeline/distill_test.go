package pipeline

import (
	"context"
	"testing"

	"papersilm/internal/config"
	"papersilm/pkg/protocol"
)

func TestDistillFromCachedPages(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.BaseDir = t.TempDir()
	svc := New(cfg)
	pages := []Page{
		{Page: 1, Content: "A Strong Paper Title\nAbstract\nWe study task X. Our method improves performance by 12.5% over baseline."},
		{Page: 2, Content: "Method\nWe propose a two-stage framework with retrieval and reranking."},
		{Page: 3, Content: "Experiments\nWe evaluate on Dataset-A and Dataset-B.\nResults\nAccuracy improves to 91.2%."},
		{Page: 4, Content: "Conclusion\nOur approach is simple and effective.\nLimitations\nThe method is only tested on English datasets."},
	}
	if err := svc.writePagesCache("sess_1", "paper_1", pages); err != nil {
		t.Fatalf("writePagesCache: %v", err)
	}
	ref := protocol.PaperRef{
		PaperID: "paper_1",
		Inspection: protocol.SourceInspection{
			Title: "A Strong Paper Title",
		},
	}
	digest, err := svc.Distill(context.Background(), "sess_1", ref, "zh", "distill")
	if err != nil {
		t.Fatalf("Distill: %v", err)
	}
	if digest.Title == "" || digest.OneLineSummary == "" {
		t.Fatalf("digest missing key fields: %+v", digest)
	}
	if len(digest.KeyResults) == 0 {
		t.Fatalf("expected key results")
	}
	if digest.Markdown == "" {
		t.Fatalf("expected markdown output")
	}
}

