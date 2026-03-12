package pipeline

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/cloudwego/eino/schema"
	recursive "github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"

	"papersilm/pkg/protocol"
)

type chunk struct {
	Page    int
	Section string
	Content string
}

func (s *Service) Distill(ctx context.Context, sessionID string, ref protocol.PaperRef, lang, style string) (protocol.PaperDigest, error) {
	pages, err := s.readPagesCache(sessionID, ref.PaperID)
	if err != nil {
		return protocol.PaperDigest{}, err
	}
	chunks := s.buildChunks(ctx, pages)
	title := ref.Inspection.Title
	if title == "" {
		title = extractTitle(pages)
	}
	problem := firstSentence(preferredContent(chunks, "abstract", "introduction"))
	method := topSentences(preferredContent(chunks, "method", "abstract"), 4)
	experiments := topSentences(preferredContent(chunks, "experiment", "results"), 4)
	results := extractKeyResults(chunks)
	conclusions := topSentences(preferredContent(chunks, "conclusion", "results"), 3)
	limitations := topSentences(preferredContent(chunks, "limitations"), 3)
	if len(limitations) == 0 {
		limitations = []string{"正文未明显给出局限部分，需谨慎解读结果外推范围。"}
	}
	oneLine := buildOneLine(title, problem, method, results)
	digest := protocol.PaperDigest{
		PaperID:             ref.PaperID,
		Title:               title,
		Problem:             fallback(problem, "该论文的核心问题需要从原文进一步确认。"),
		OneLineSummary:      oneLine,
		MethodSummary:       fallbackSlice(method, "方法细节未在可提取文本中清晰呈现。"),
		ExperimentSummary:   fallbackSlice(experiments, "实验设计细节未在可提取文本中清晰呈现。"),
		KeyResults:          fallbackResults(results),
		Conclusions:         fallbackSlice(conclusions, "作者结论未在可提取文本中清晰呈现。"),
		Limitations:         limitations,
		Citations:           collectDigestCitations(results),
		Language:            lang,
		Style:               style,
		GeneratedAt:         time.Now().UTC(),
		HasBackgroundOmitted: true,
	}
	digest.Markdown = renderPaperDigest(digest)
	return digest, nil
}

func (s *Service) buildChunks(ctx context.Context, pages []Page) []chunk {
	docs := make([]*schema.Document, 0, len(pages))
	for _, page := range pages {
		docs = append(docs, &schema.Document{
			ID:      fmt.Sprintf("page-%d", page.Page),
			Content: page.Content,
			MetaData: map[string]any{
				"page": page.Page,
			},
		})
	}
	splitter, err := recursive.NewSplitter(ctx, &recursive.Config{
		ChunkSize:   2200,
		OverlapSize: 250,
	})
	if err != nil {
		return pageFallbackChunks(pages)
	}
	out, err := splitter.Transform(ctx, docs)
	if err != nil || len(out) == 0 {
		return pageFallbackChunks(pages)
	}
	chunks := make([]chunk, 0, len(out))
	for _, doc := range out {
		page := 0
		switch v := doc.MetaData["page"].(type) {
		case int:
			page = v
		case float64:
			page = int(v)
		}
		content := strings.TrimSpace(doc.Content)
		if content == "" {
			continue
		}
		chunks = append(chunks, chunk{
			Page:    page,
			Section: classifyChunk(content),
			Content: content,
		})
	}
	if len(chunks) == 0 {
		return pageFallbackChunks(pages)
	}
	return chunks
}

func pageFallbackChunks(pages []Page) []chunk {
	out := make([]chunk, 0, len(pages))
	for _, page := range pages {
		out = append(out, chunk{
			Page:    page.Page,
			Section: classifyChunk(page.Content),
			Content: page.Content,
		})
	}
	return out
}

func preferredContent(chunks []chunk, sections ...string) string {
	var selected []chunk
	for _, section := range sections {
		for _, c := range chunks {
			if c.Section == section {
				selected = append(selected, c)
			}
		}
	}
	if len(selected) == 0 {
		for _, c := range chunks {
			if c.Section != "background" && c.Section != "related_work" {
				selected = append(selected, c)
				if len(selected) >= 3 {
					break
				}
			}
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].Page < selected[j].Page
	})
	builder := strings.Builder{}
	for _, c := range selected {
		builder.WriteString(c.Content)
		builder.WriteString("\n")
		if builder.Len() > 5000 {
			break
		}
	}
	return builder.String()
}

func classifyChunk(content string) string {
	l := strings.ToLower(content)
	switch {
	case strings.Contains(l, "abstract"):
		return "abstract"
	case strings.Contains(l, "related work"):
		return "related_work"
	case strings.Contains(l, "introduction"):
		return "background"
	case strings.Contains(l, "method") || strings.Contains(l, "approach") || strings.Contains(l, "framework"):
		return "method"
	case strings.Contains(l, "experiment") || strings.Contains(l, "dataset") || strings.Contains(l, "setting"):
		return "experiment"
	case strings.Contains(l, "result") || strings.Contains(l, "ablation") || strings.Contains(l, "evaluation"):
		return "results"
	case strings.Contains(l, "conclusion"):
		return "conclusion"
	case strings.Contains(l, "limitation"):
		return "limitations"
	default:
		return "body"
	}
}

func topSentences(content string, limit int) []string {
	candidates := splitSentences(content)
	out := make([]string, 0, limit)
	seen := map[string]struct{}{}
	for _, sentence := range candidates {
		sentence = strings.TrimSpace(sentence)
		if len(sentence) < 30 {
			continue
		}
		if _, ok := seen[sentence]; ok {
			continue
		}
		if looksLikeBackground(sentence) {
			continue
		}
		seen[sentence] = struct{}{}
		out = append(out, sentence)
		if len(out) == limit {
			break
		}
	}
	return out
}

func splitSentences(content string) []string {
	parts := strings.FieldsFunc(content, func(r rune) bool {
		switch r {
		case '。', '！', '？', '!', '?', '\n', '\r':
			return true
		case '.':
			return true
		default:
			return unicode.IsControl(r)
		}
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func looksLikeBackground(sentence string) bool {
	l := strings.ToLower(sentence)
	return strings.Contains(l, "has attracted significant attention") ||
		strings.Contains(l, "in recent years") ||
		strings.Contains(l, "related work") ||
		strings.Contains(l, "broadly")
}

func firstSentence(content string) string {
	parts := splitSentences(content)
	for _, part := range parts {
		if len(strings.TrimSpace(part)) >= 20 {
			return strings.TrimSpace(part)
		}
	}
	return ""
}

func buildOneLine(title, problem string, method []string, results []protocol.KeyResult) string {
	summary := title
	if summary == "" {
		summary = "这篇论文"
	}
	if problem != "" {
		summary += "：聚焦 " + trimForLine(problem)
	}
	if len(method) > 0 {
		summary += "；方法上 " + trimForLine(method[0])
	}
	if len(results) > 0 && results[0].Claim != "" {
		summary += "；结果上 " + trimForLine(results[0].Claim)
	}
	return summary
}

func trimForLine(in string) string {
	in = strings.TrimSpace(in)
	if len(in) > 80 {
		return in[:80] + "..."
	}
	return in
}

var numberPattern = regexp.MustCompile(`(?i)(\d+(\.\d+)?%?)`)

func extractKeyResults(chunks []chunk) []protocol.KeyResult {
	out := make([]protocol.KeyResult, 0, 6)
	for _, c := range chunks {
		if c.Section != "results" && c.Section != "conclusion" {
			continue
		}
		for _, sentence := range splitSentences(c.Content) {
			if len(sentence) < 30 || !numberPattern.MatchString(sentence) {
				continue
			}
			out = append(out, protocol.KeyResult{
				Claim: strings.TrimSpace(sentence),
				Value: numberPattern.FindString(sentence),
				Citations: []protocol.Citation{{
					Page:    c.Page,
					Snippet: trimForLine(sentence),
				}},
			})
			if len(out) == 6 {
				return out
			}
		}
	}
	return out
}

func fallback(in, def string) string {
	if strings.TrimSpace(in) == "" {
		return def
	}
	return in
}

func fallbackSlice(in []string, def string) []string {
	if len(in) == 0 {
		return []string{def}
	}
	return in
}

func fallbackResults(in []protocol.KeyResult) []protocol.KeyResult {
	if len(in) == 0 {
		return []protocol.KeyResult{{
			Claim: "未在可提取文本中找到足够明确的量化结果，建议回到原文结果章节核对。",
		}}
	}
	return in
}

func collectDigestCitations(results []protocol.KeyResult) []protocol.Citation {
	out := make([]protocol.Citation, 0, len(results))
	for _, result := range results {
		out = append(out, result.Citations...)
	}
	return out
}

func renderPaperDigest(d protocol.PaperDigest) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(d.Title)
	b.WriteString("\n\n")
	writeSection(&b, "一句话总结", []string{d.OneLineSummary})
	writeSection(&b, "这篇论文做了什么", []string{d.Problem})
	writeSection(&b, "方法核心", d.MethodSummary)
	writeSection(&b, "实验怎么做", d.ExperimentSummary)
	if len(d.KeyResults) > 0 {
		lines := make([]string, 0, len(d.KeyResults))
		for _, kr := range d.KeyResults {
			line := kr.Claim
			if len(kr.Citations) > 0 && kr.Citations[0].Page > 0 {
				line += fmt.Sprintf(" [p.%d]", kr.Citations[0].Page)
			}
			lines = append(lines, line)
		}
		writeSection(&b, "关键结果与数字", lines)
	}
	writeSection(&b, "作者结论", d.Conclusions)
	writeSection(&b, "局限与注意点", d.Limitations)
	return b.String()
}

func writeSection(b *strings.Builder, title string, lines []string) {
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(strings.TrimSpace(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
}
