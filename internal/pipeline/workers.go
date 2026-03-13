package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type SourceMaterial struct {
	Ref        protocol.PaperRef
	Title      string
	Chunks     []chunk
	Provenance protocol.ContentSource
}

type PaperSummaryOutput struct {
	PaperID           string                 `json:"paper_id"`
	Title             string                 `json:"title"`
	Problem           string                 `json:"problem"`
	OneLineSummary    string                 `json:"one_line_summary"`
	MethodSummary     []string               `json:"method_summary"`
	Provenance        protocol.ContentSource `json:"provenance"`
	HasBackgroundTrim bool                   `json:"has_background_trim"`
}

type ExperimentOutput struct {
	PaperID           string               `json:"paper_id"`
	ExperimentSummary []string             `json:"experiment_summary"`
	KeyResults        []protocol.KeyResult `json:"key_results"`
	Conclusions       []string             `json:"conclusions"`
	Limitations       []string             `json:"limitations"`
	Citations         []protocol.Citation  `json:"citations"`
}

type MathReasoningOutput struct {
	PaperID string   `json:"paper_id"`
	Notes   []string `json:"notes"`
}

type WebResearchOutput struct {
	PaperID string   `json:"paper_id"`
	Notes   []string `json:"notes"`
}

func (s *Service) LoadSourceMaterial(ctx context.Context, sessionID string, ref protocol.PaperRef, goal string) (SourceMaterial, error) {
	if supportsAlphaXiv(ref) {
		if material, ok, err := s.loadAlphaXivMaterial(ctx, sessionID, ref, goal); err != nil {
			return SourceMaterial{}, err
		} else if ok {
			return material, nil
		}
	}

	pages, err := s.readPagesCache(sessionID, ref.PaperID)
	if err != nil {
		return SourceMaterial{}, err
	}
	provenance := protocol.ContentSourceUnknown
	if supportsAlphaXiv(ref) {
		provenance = protocol.ContentSourceArxivPDFFallback
	}
	title := extractTitle(pages)
	if strings.TrimSpace(title) == "" {
		title = ref.Inspection.Title
	}
	return SourceMaterial{
		Ref:        ref,
		Title:      title,
		Chunks:     s.buildChunks(ctx, pages),
		Provenance: provenance,
	}, nil
}

func (s *Service) loadAlphaXivMaterial(ctx context.Context, sessionID string, ref protocol.PaperRef, goal string) (SourceMaterial, bool, error) {
	preferFullText := ref.PreferredContentSource == protocol.ContentSourceAlphaXivFullText
	detailRequested := needsDetailedContent(goal)

	if !preferFullText {
		if overview, ok, err := s.LookupAlphaXivOverview(ctx, sessionID, ref); err != nil {
			return SourceMaterial{}, false, err
		} else if ok {
			if !detailRequested || alphaOverviewSupportsDetail(goal, overview) {
				return SourceMaterial{
					Ref:        ref,
					Title:      extractAlphaTitle(overview),
					Chunks:     alphaMarkdownToChunks(overview),
					Provenance: protocol.ContentSourceAlphaXivOverview,
				}, true, nil
			}
		}
	}

	if fullText, ok, err := s.LookupAlphaXivFullText(ctx, sessionID, ref); err != nil {
		return SourceMaterial{}, false, err
	} else if ok {
		return SourceMaterial{
			Ref:        ref,
			Title:      extractAlphaTitle(fullText),
			Chunks:     s.buildChunks(ctx, []Page{{Page: 0, Content: fullText}}),
			Provenance: protocol.ContentSourceAlphaXivFullText,
		}, true, nil
	}

	if preferFullText {
		if overview, ok, err := s.LookupAlphaXivOverview(ctx, sessionID, ref); err != nil {
			return SourceMaterial{}, false, err
		} else if ok {
			return SourceMaterial{
				Ref:        ref,
				Title:      extractAlphaTitle(overview),
				Chunks:     alphaMarkdownToChunks(overview),
				Provenance: protocol.ContentSourceAlphaXivOverview,
			}, true, nil
		}
	}

	return SourceMaterial{}, false, nil
}

func (s *Service) BuildPaperSummary(_ context.Context, material SourceMaterial, lang, style string) PaperSummaryOutput {
	title := strings.TrimSpace(material.Title)
	if title == "" {
		title = fallback(material.Ref.Inspection.Title, fallback(material.Ref.ResolvedPaperID, material.Ref.PaperID))
	}
	problem := firstSentence(preferredContent(material.Chunks, "abstract", "introduction"))
	method := topSentences(preferredContent(material.Chunks, "method", "abstract"), 4)
	results := extractKeyResults(material.Chunks)
	return PaperSummaryOutput{
		PaperID:           material.Ref.PaperID,
		Title:             title,
		Problem:           fallback(problem, "该论文的核心问题需要从原文进一步确认。"),
		OneLineSummary:    buildOneLine(title, problem, method, results),
		MethodSummary:     fallbackSlice(method, "方法细节未在可提取文本中清晰呈现。"),
		Provenance:        material.Provenance,
		HasBackgroundTrim: true,
	}
}

func (s *Service) BuildExperimentOutput(_ context.Context, material SourceMaterial) ExperimentOutput {
	experiments := topSentences(preferredContent(material.Chunks, "experiment", "results"), 4)
	results := extractKeyResults(material.Chunks)
	conclusions := topSentences(preferredContent(material.Chunks, "conclusion", "results"), 3)
	limitations := topSentences(preferredContent(material.Chunks, "limitations"), 3)
	if len(limitations) == 0 {
		limitations = []string{"正文未明显给出局限部分，需谨慎解读结果外推范围。"}
	}
	return ExperimentOutput{
		PaperID:           material.Ref.PaperID,
		ExperimentSummary: fallbackSlice(experiments, "实验设计细节未在可提取文本中清晰呈现。"),
		KeyResults:        fallbackResults(results),
		Conclusions:       fallbackSlice(conclusions, "作者结论未在可提取文本中清晰呈现。"),
		Limitations:       limitations,
		Citations:         collectDigestCitations(results),
	}
}

func (s *Service) BuildMathReasoning(_ context.Context, material SourceMaterial, goal string) MathReasoningOutput {
	notes := make([]string, 0, 3)
	for _, c := range material.Chunks {
		for _, sentence := range splitSentences(c.Content) {
			l := strings.ToLower(sentence)
			if strings.Contains(l, "equation") || strings.Contains(l, "proof") || strings.Contains(l, "theorem") ||
				strings.Contains(l, "derivation") || strings.Contains(l, "公式") || strings.Contains(l, "证明") || strings.Contains(l, "推导") {
				notes = append(notes, strings.TrimSpace(sentence))
				if len(notes) == 3 {
					return MathReasoningOutput{PaperID: material.Ref.PaperID, Notes: notes}
				}
			}
		}
	}
	if len(notes) == 0 {
		notes = []string{fmt.Sprintf("当前目标包含数学或公式细读需求，但 %s 中没有提取到足够清晰的公式/证明文本。", material.Ref.PaperID)}
	}
	_ = goal
	return MathReasoningOutput{PaperID: material.Ref.PaperID, Notes: notes}
}

func (s *Service) BuildWebResearch(_ context.Context, material SourceMaterial, goal string) WebResearchOutput {
	notes := []string{
		fmt.Sprintf("外部研究请求已启用；当前以论文可得的 AlphaXiv/arXiv 上下文为主，没有额外扩展到更宽泛网页搜索。"),
		fallback(firstSentence(preferredContent(material.Chunks, "abstract", "background")), "未从结构化源中提取到额外上下文。"),
	}
	_ = goal
	return WebResearchOutput{PaperID: material.Ref.PaperID, Notes: notes}
}

func (s *Service) MergePaperDigest(summary PaperSummaryOutput, experiment ExperimentOutput, math *MathReasoningOutput, web *WebResearchOutput, lang, style string) protocol.PaperDigest {
	method := append([]string(nil), summary.MethodSummary...)
	if math != nil && len(math.Notes) > 0 {
		method = append(method, "数学细节："+math.Notes[0])
	}
	conclusions := append([]string(nil), experiment.Conclusions...)
	if web != nil && len(web.Notes) > 0 {
		conclusions = append(conclusions, "外部上下文："+web.Notes[0])
	}
	digest := protocol.PaperDigest{
		PaperID:              summary.PaperID,
		Title:                summary.Title,
		Problem:              summary.Problem,
		OneLineSummary:       summary.OneLineSummary,
		MethodSummary:        method,
		ExperimentSummary:    experiment.ExperimentSummary,
		KeyResults:           experiment.KeyResults,
		Conclusions:          conclusions,
		Limitations:          experiment.Limitations,
		Citations:            experiment.Citations,
		Language:             lang,
		Style:                style,
		ContentProvenance:    summary.Provenance,
		GeneratedAt:          time.Now().UTC(),
		HasBackgroundOmitted: summary.HasBackgroundTrim,
	}
	digest.Markdown = renderPaperDigest(digest)
	return digest
}

func (s *Service) BuildMethodMatrix(digests []protocol.PaperDigest) protocol.ComparisonMatrixRow {
	sortDigests(digests)
	row := protocol.ComparisonMatrixRow{Dimension: "method", Values: map[string]string{}}
	for _, digest := range digests {
		row.Values[digest.PaperID] = firstLine(digest.MethodSummary)
	}
	return row
}

func (s *Service) BuildExperimentMatrix(digests []protocol.PaperDigest) protocol.ComparisonMatrixRow {
	sortDigests(digests)
	row := protocol.ComparisonMatrixRow{Dimension: "experiment", Values: map[string]string{}}
	for _, digest := range digests {
		row.Values[digest.PaperID] = firstLine(digest.ExperimentSummary)
	}
	return row
}

func (s *Service) BuildResultsMatrix(digests []protocol.PaperDigest) protocol.ComparisonMatrixRow {
	sortDigests(digests)
	row := protocol.ComparisonMatrixRow{Dimension: "results", Values: map[string]string{}}
	for _, digest := range digests {
		row.Values[digest.PaperID] = firstResult(digest.KeyResults)
	}
	return row
}

func (s *Service) BuildFinalComparison(goal string, digests []protocol.PaperDigest, methodRow, experimentRow, resultRow protocol.ComparisonMatrixRow, lang, style string) protocol.ComparisonDigest {
	sortDigests(digests)
	cmp := protocol.ComparisonDigest{
		PaperIDs:         collectPaperIDs(digests),
		Goal:             goal,
		PaperSummaries:   digests,
		MethodMatrix:     []protocol.ComparisonMatrixRow{methodRow},
		ExperimentMatrix: []protocol.ComparisonMatrixRow{experimentRow},
		ResultMatrix:     []protocol.ComparisonMatrixRow{resultRow},
		Synthesis:        buildSynthesis(digests),
		Limitations:      []string{"该对比基于单篇结构化摘要聚合而来，建议对关键实验和统计显著性回到原文核对。"},
		Language:         lang,
		Style:            style,
		GeneratedAt:      time.Now().UTC(),
	}
	cmp.Markdown = renderComparison(cmp)
	return cmp
}
