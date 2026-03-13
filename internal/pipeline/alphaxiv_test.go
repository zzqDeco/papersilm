package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestInspectSourcePrefersAlphaXivOverview(t *testing.T) {
	t.Parallel()

	var pdfHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/overview/2401.12345.md":
			http.Redirect(w, r, "/overview-redirected/2401.12345.md", http.StatusMovedPermanently)
		case "/overview-redirected/2401.12345.md":
			_, _ = w.Write([]byte(verboseOverview("Alpha Overview Paper")))
		case "/abs/2401.12345.md":
			http.NotFound(w, r)
		case "/pdf/2401.12345.pdf":
			pdfHits++
			_, _ = w.Write(verbosePDF("Alpha Overview Paper"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := newMockAlphaService(t, server)
	ref := protocol.PaperRef{
		PaperID:                "paper_1",
		URI:                    "2401.12345",
		ResolvedPaperID:        "2401.12345",
		SourceType:             protocol.SourceTypePaperID,
		PreferredContentSource: protocol.ContentSourceAlphaXivOverview,
	}

	updated, pages, err := svc.InspectSource(context.Background(), "sess_1", ref)
	if err != nil {
		t.Fatalf("InspectSource: %v", err)
	}
	if len(pages) != 0 {
		t.Fatalf("expected no pdf pages for overview path")
	}
	if updated.ContentProvenance != protocol.ContentSourceAlphaXivOverview {
		t.Fatalf("expected overview provenance, got %+v", updated)
	}
	if pdfHits != 0 {
		t.Fatalf("expected no pdf fallback, got %d hits", pdfHits)
	}
}

func TestInspectSourceFallsBackToAlphaXivFullText(t *testing.T) {
	t.Parallel()

	var pdfHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/overview/2401.54321.md":
			http.NotFound(w, r)
		case "/abs/2401.54321.md":
			_, _ = w.Write([]byte(verboseFullText("Alpha Full Text Paper")))
		case "/pdf/2401.54321.pdf":
			pdfHits++
			_, _ = w.Write(verbosePDF("Alpha Full Text Paper"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := newMockAlphaService(t, server)
	ref := protocol.PaperRef{
		PaperID:                "paper_2",
		URI:                    "2401.54321",
		ResolvedPaperID:        "2401.54321",
		SourceType:             protocol.SourceTypePaperID,
		PreferredContentSource: protocol.ContentSourceAlphaXivOverview,
	}

	updated, pages, err := svc.InspectSource(context.Background(), "sess_1", ref)
	if err != nil {
		t.Fatalf("InspectSource: %v", err)
	}
	if len(pages) != 0 {
		t.Fatalf("expected no pdf pages for full text path")
	}
	if updated.ContentProvenance != protocol.ContentSourceAlphaXivFullText {
		t.Fatalf("expected full text provenance, got %+v", updated)
	}
	if pdfHits != 0 {
		t.Fatalf("expected no pdf fallback, got %d hits", pdfHits)
	}
}

func TestInspectSourceFallsBackToPDFWhenAlphaXivMissing(t *testing.T) {
	t.Parallel()

	var pdfHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/overview/2401.99999.md", "/abs/2401.99999.md":
			http.NotFound(w, r)
		case "/pdf/2401.99999.pdf":
			pdfHits++
			_, _ = w.Write(verbosePDF("PDF Fallback Paper"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := newMockAlphaService(t, server)
	ref := protocol.PaperRef{
		PaperID:                "paper_3",
		URI:                    "2401.99999",
		ResolvedPaperID:        "2401.99999",
		SourceType:             protocol.SourceTypePaperID,
		PreferredContentSource: protocol.ContentSourceAlphaXivOverview,
	}

	updated, pages, err := svc.InspectSource(context.Background(), "sess_1", ref)
	if err != nil {
		t.Fatalf("InspectSource: %v", err)
	}
	if len(pages) == 0 {
		t.Fatalf("expected pdf pages on fallback")
	}
	if updated.ContentProvenance != protocol.ContentSourceArxivPDFFallback {
		t.Fatalf("expected pdf provenance, got %+v", updated)
	}
	if pdfHits == 0 {
		t.Fatalf("expected pdf fallback to be used")
	}
}

func TestDistillUsesFullTextForDetailedTasks(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/overview/2401.77777.md":
			_, _ = w.Write([]byte(verboseOverview("Detail Paper")))
		case "/abs/2401.77777.md":
			_, _ = w.Write([]byte(strings.Join([]string{
				"# Detail Paper",
				"Abstract",
				"We study detailed inspection for AlphaXiv routing and explain exact equations.",
				"Method",
				"Equation 3 defines the training objective with a contrastive margin.",
				"Experiments",
				"We evaluate on Dataset-A and Dataset-B.",
				"Results",
				"Accuracy reaches 92.4% and F1 reaches 89.1%.",
				"Conclusion",
				"The method handles detailed requests reliably.",
			}, "\n")))
		case "/pdf/2401.77777.pdf":
			t.Fatalf("unexpected pdf fallback for detailed task")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := newMockAlphaService(t, server)
	ref := protocol.PaperRef{
		PaperID:                "paper_4",
		URI:                    "2401.77777",
		ResolvedPaperID:        "2401.77777",
		SourceType:             protocol.SourceTypePaperID,
		PreferredContentSource: protocol.ContentSourceAlphaXivOverview,
	}

	digest, err := svc.Distill(context.Background(), "sess_1", ref, "请解释 equation 3 的含义", "zh", "distill")
	if err != nil {
		t.Fatalf("Distill: %v", err)
	}
	if digest.ContentProvenance != protocol.ContentSourceAlphaXivFullText {
		t.Fatalf("expected full text provenance, got %+v", digest)
	}
	if !strings.Contains(digest.Markdown, "来源：AlphaXiv full text") {
		t.Fatalf("expected provenance label in markdown, got %q", digest.Markdown)
	}
}

func TestCompareMarkdownIncludesProvenance(t *testing.T) {
	t.Parallel()

	svc := New(config.Default())
	cmp := svc.Compare("compare", []protocol.PaperDigest{
		{
			PaperID:           "paper_a",
			OneLineSummary:    "Paper A summary",
			MethodSummary:     []string{"Method A"},
			ExperimentSummary: []string{"Experiment A"},
			KeyResults:        []protocol.KeyResult{{Claim: "Result A"}},
			ContentProvenance: protocol.ContentSourceAlphaXivOverview,
		},
		{
			PaperID:           "paper_b",
			OneLineSummary:    "Paper B summary",
			MethodSummary:     []string{"Method B"},
			ExperimentSummary: []string{"Experiment B"},
			KeyResults:        []protocol.KeyResult{{Claim: "Result B"}},
			ContentProvenance: protocol.ContentSourceArxivPDFFallback,
		},
	}, "zh", "distill")

	if !strings.Contains(cmp.Markdown, "paper_a [AlphaXiv overview]") {
		t.Fatalf("expected overview provenance in comparison markdown: %q", cmp.Markdown)
	}
	if !strings.Contains(cmp.Markdown, "paper_b [arXiv PDF fallback]") {
		t.Fatalf("expected pdf provenance in comparison markdown: %q", cmp.Markdown)
	}
}

func newMockAlphaService(t *testing.T, server *httptest.Server) *Service {
	t.Helper()

	cfg := config.Default()
	cfg.BaseDir = t.TempDir()
	svc := New(cfg)
	svc.httpClient = server.Client()
	svc.arxivBaseURL = server.URL
	svc.alphaXiv = NewAlphaXivClient(server.URL, server.Client())
	return svc
}

func verboseOverview(title string) string {
	body := strings.Join([]string{
		"## Research Paper Analysis: \"" + title + "\"",
		"",
		"### 3. Key Objectives and Motivation",
		strings.Repeat("The paper focuses on extracting core facts instead of broad background discussion. ", 8),
		"",
		"### 4. Methodology",
		strings.Repeat("The method uses a retrieval and synthesis pipeline with structured evidence extraction. ", 8),
		"",
		"### 5. Main Findings and Results",
		strings.Repeat("Results report 91.2% accuracy and 88.4% F1 on benchmark datasets. ", 8),
		"",
		"### 6. Significance and Potential Impact",
		strings.Repeat("This section is intentionally verbose and should not be copied directly into the digest. ", 6),
	}, "\n")
	return body
}

func verboseFullText(title string) string {
	return strings.Join([]string{
		"# " + title,
		"Abstract",
		strings.Repeat("We study precise paper extraction with structured evidence. ", 10),
		"Method",
		strings.Repeat("We use a two-stage retrieval and reranking framework. ", 10),
		"Experiments",
		strings.Repeat("We evaluate on Dataset-A and Dataset-B with multiple metrics. ", 10),
		"Results",
		strings.Repeat("Accuracy reaches 92.4% and F1 reaches 89.1%. ", 10),
		"Conclusion",
		strings.Repeat("The method is practical for paper distillation. ", 6),
	}, "\n")
}

func verbosePDF(title string) []byte {
	var lines []string
	lines = append(lines, title)
	paragraph := "Abstract We study paper distillation for long academic documents. Method We propose a two stage analysis workflow that extracts problem setup, method details, experiment settings, key numeric results, conclusions, and limitations without spending tokens on general domain background. Experiments We evaluate on Dataset A and Dataset B with consistent prompts and report accuracy 91.2 percent, macro F1 88.3 percent, and latency reductions of 17 percent. Results Our approach outperforms the baseline on all reported metrics and remains stable across ablation settings. Conclusion The method is practical for CLI based paper review. "
	for i := 0; i < 18; i++ {
		lines = append(lines, fmt.Sprintf("%s Section %d.", paragraph, i+1))
	}
	return minimalPDF(lines)
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
