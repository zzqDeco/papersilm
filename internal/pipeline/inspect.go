package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	fileloader "github.com/cloudwego/eino-ext/components/document/loader/file"
	pdfparser "github.com/cloudwego/eino-ext/components/document/parser/pdf"
	"github.com/cloudwego/eino/components/document"
	einoschema "github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type Page struct {
	Page    int    `json:"page"`
	Content string `json:"content"`
}

func (s *Service) InspectSource(ctx context.Context, sessionID string, ref protocol.PaperRef) (protocol.PaperRef, []Page, error) {
	path, err := s.ensureLocalPDF(ctx, sessionID, ref)
	if err != nil {
		ref.Status = protocol.SourceStatusFailed
		ref.Inspection.ExtractableText = false
		ref.Inspection.FailureReason = err.Error()
		return ref, nil, err
	}
	ref.LocalPath = path

	pages, err := s.loadPages(ctx, path)
	if err != nil {
		ref.Status = protocol.SourceStatusFailed
		ref.Inspection.ExtractableText = false
		ref.Inspection.FailureReason = err.Error()
		return ref, nil, err
	}
	ref.Status = protocol.SourceStatusInspected
	ref.Inspection.PageCount = len(pages)
	ref.Inspection.ExtractableText = len(pages) > 0 && hasEnoughText(pages)
	ref.Inspection.Title = extractTitle(pages)
	ref.Inspection.SectionHints = extractSectionHints(pages)
	ref.Inspection.Comparable = ref.Inspection.ExtractableText
	ref.Inspection.SampleIntroduction = introSnippet(pages)

	if !ref.Inspection.ExtractableText {
		ref.Inspection.FailureReason = "pdf text extraction produced too little text; scanned pdf or unsupported layout"
	}
	if err := s.writePagesCache(sessionID, ref.PaperID, pages); err != nil {
		return ref, nil, err
	}
	return ref, pages, nil
}

func (s *Service) ensureLocalPDF(ctx context.Context, sessionID string, ref protocol.PaperRef) (string, error) {
	if ref.LocalPath != "" {
		return ref.LocalPath, nil
	}
	url, err := canonicalArxivPDF(ref.URI)
	if err != nil {
		return "", err
	}
	cachePath := filepath.Join(s.config.BaseDir, "sessions", sessionID, "cache", ref.PaperID+".pdf")
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("download pdf failed: %s", resp.Status)
	}
	f, err := os.Create(cachePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return cachePath, nil
}

func (s *Service) loadPages(ctx context.Context, path string) ([]Page, error) {
	parser, err := pdfparser.NewPDFParser(ctx, &pdfparser.Config{ToPages: true})
	if err != nil {
		return nil, err
	}
	loader, err := fileloader.NewFileLoader(ctx, &fileloader.FileLoaderConfig{
		UseNameAsID: true,
		Parser:      parser,
	})
	if err != nil {
		return nil, err
	}
	docs, err := loader.Load(ctx, document.Source{URI: path})
	if err != nil {
		return nil, err
	}
	pages := make([]Page, 0, len(docs))
	for idx, doc := range docs {
		content := cleanText(doc)
		if content == "" {
			continue
		}
		pages = append(pages, Page{
			Page:    idx + 1,
			Content: content,
		})
	}
	return pages, nil
}

func cleanText(doc *einoschema.Document) string {
	content := strings.ReplaceAll(doc.Content, "\u0000", "")
	lines := strings.Split(content, "\n")
	trimmed := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		trimmed = append(trimmed, line)
	}
	return strings.Join(trimmed, "\n")
}

func hasEnoughText(pages []Page) bool {
	total := 0
	for _, page := range pages {
		total += len(page.Content)
	}
	return total > 1200
}

func extractTitle(pages []Page) string {
	if len(pages) == 0 {
		return ""
	}
	lines := strings.Split(pages[0].Content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || len(line) < 8 {
			continue
		}
		if isLikelyHeading(line) {
			return line
		}
	}
	return filepath.Base("paper")
}

func extractSectionHints(pages []Page) []string {
	hints := make(map[string]struct{})
	for _, page := range pages {
		for _, line := range strings.Split(page.Content, "\n") {
			label := classifySectionLine(line)
			if label != "" {
				hints[label] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(hints))
	for hint := range hints {
		out = append(out, hint)
	}
	return out
}

func introSnippet(pages []Page) string {
	for _, page := range pages {
		for _, line := range strings.Split(page.Content, "\n") {
			if strings.Contains(strings.ToLower(line), "introduction") {
				return line
			}
		}
	}
	if len(pages) == 0 {
		return ""
	}
	snippet := pages[0].Content
	if len(snippet) > 240 {
		return snippet[:240]
	}
	return snippet
}

var headingPattern = regexp.MustCompile(`^[A-Z0-9][A-Za-z0-9 ,:;()/\-]{5,120}$`)

func isLikelyHeading(line string) bool {
	return headingPattern.MatchString(strings.TrimSpace(line))
}

func classifySectionLine(line string) string {
	l := strings.ToLower(strings.TrimSpace(line))
	switch {
	case strings.Contains(l, "abstract"):
		return "abstract"
	case strings.Contains(l, "introduction"):
		return "introduction"
	case strings.Contains(l, "related work"):
		return "related_work"
	case strings.Contains(l, "method") || strings.Contains(l, "approach"):
		return "method"
	case strings.Contains(l, "experiment"):
		return "experiment"
	case strings.Contains(l, "result") || strings.Contains(l, "evaluation"):
		return "results"
	case strings.Contains(l, "conclusion"):
		return "conclusion"
	case strings.Contains(l, "limitation"):
		return "limitations"
	default:
		return ""
	}
}

func (s *Service) pagesCachePath(sessionID, paperID string) string {
	return filepath.Join(s.config.BaseDir, "sessions", sessionID, "cache", paperID+".pages.json")
}

func (s *Service) writePagesCache(sessionID, paperID string, pages []Page) error {
	raw, err := json.MarshalIndent(pages, "", "  ")
	if err != nil {
		return err
	}
	path := s.pagesCachePath(sessionID, paperID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (s *Service) readPagesCache(sessionID, paperID string) ([]Page, error) {
	raw, err := os.ReadFile(s.pagesCachePath(sessionID, paperID))
	if err != nil {
		return nil, err
	}
	var pages []Page
	if err := json.Unmarshal(raw, &pages); err != nil {
		return nil, err
	}
	return pages, nil
}
