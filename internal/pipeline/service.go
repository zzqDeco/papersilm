package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"papersilm/internal/config"
	"papersilm/pkg/protocol"
)

type Service struct {
	config config.Config
}

func New(cfg config.Config) *Service {
	return &Service{config: cfg}
}

func (s *Service) NormalizeSources(_ context.Context, sessionID string, raw []string) ([]protocol.PaperRef, error) {
	out := make([]protocol.PaperRef, 0, len(raw))
	for idx, src := range raw {
		ref, err := s.normalizeSource(sessionID, idx, src)
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, nil
}

func (s *Service) normalizeSource(sessionID string, idx int, raw string) (protocol.PaperRef, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return protocol.PaperRef{}, fmt.Errorf("empty source")
	}
	id := buildPaperID(sessionID, idx, trimmed)
	ref := protocol.PaperRef{
		PaperID:    id,
		URI:        trimmed,
		Label:      defaultLabel(trimmed),
		Status:     protocol.SourceStatusAttached,
		SourceType: protocol.SourceTypeLocalPDF,
	}
	switch {
	case isArxivAbs(trimmed):
		ref.SourceType = protocol.SourceTypeArxivAbs
	case isArxivPDF(trimmed):
		ref.SourceType = protocol.SourceTypeArxivPDF
	default:
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return protocol.PaperRef{}, err
		}
		if strings.ToLower(filepath.Ext(abs)) != ".pdf" {
			return protocol.PaperRef{}, fmt.Errorf("only pdf sources are supported in v1: %s", trimmed)
		}
		if _, err := os.Stat(abs); err != nil {
			return protocol.PaperRef{}, err
		}
		ref.URI = abs
		ref.LocalPath = abs
	}
	return ref, nil
}

func buildPaperID(sessionID string, idx int, uri string) string {
	replacer := strings.NewReplacer("https://", "", "http://", "", "/", "_", ".", "_", ":", "_", "-", "_")
	base := replacer.Replace(uri)
	if len(base) > 36 {
		base = base[:36]
	}
	return fmt.Sprintf("%s_%02d_%s", sessionID[len(sessionID)-6:], idx+1, base)
}

func defaultLabel(uri string) string {
	if isArxivAbs(uri) || isArxivPDF(uri) {
		return "arxiv"
	}
	return filepath.Base(uri)
}

var (
	arxivAbsPattern = regexp.MustCompile(`^https?://arxiv\.org/abs/([^/?#]+)$`)
	arxivPDFPattern = regexp.MustCompile(`^https?://arxiv\.org/pdf/([^/?#]+)(\.pdf)?$`)
)

func isArxivAbs(in string) bool {
	return arxivAbsPattern.MatchString(in)
}

func isArxivPDF(in string) bool {
	return arxivPDFPattern.MatchString(in)
}

func canonicalArxivPDF(in string) (string, error) {
	if m := arxivAbsPattern.FindStringSubmatch(in); len(m) == 2 {
		return "https://arxiv.org/pdf/" + m[1] + ".pdf", nil
	}
	if m := arxivPDFPattern.FindStringSubmatch(in); len(m) >= 2 {
		return "https://arxiv.org/pdf/" + m[1] + ".pdf", nil
	}
	return "", fmt.Errorf("not an arxiv source: %s", in)
}

func (s *Service) BuildPlan(task string, refs []protocol.PaperRef) protocol.PlanResult {
	toolPlan := []string{"attach_sources", "inspect_sources"}
	willCompare := len(refs) > 1
	if willCompare {
		toolPlan = append(toolPlan, "distill_paper (for each source)", "compare_papers")
	} else {
		toolPlan = append(toolPlan, "distill_paper")
	}
	risks := make([]string, 0, len(refs)+2)
	for _, ref := range refs {
		if !ref.Inspection.ExtractableText && ref.Inspection.FailureReason != "" {
			risks = append(risks, fmt.Sprintf("%s: %s", ref.PaperID, ref.Inspection.FailureReason))
		}
		if ref.Inspection.PageCount > 50 {
			risks = append(risks, fmt.Sprintf("%s: long paper (%d pages)", ref.PaperID, ref.Inspection.PageCount))
		}
	}
	if len(risks) == 0 {
		risks = append(risks, "no major inspection risks detected")
	}
	return protocol.PlanResult{
		Goal:               strings.TrimSpace(task),
		SourceSummary:      refs,
		ExtractionStrategy: []string{"inspect page-by-page pdf content", "suppress background/related-work heavy chunks", "produce per-paper digest first", "aggregate comparison from structured digests only"},
		ExpectedSections:   expectedSections(willCompare),
		Risks:              risks,
		ToolPlan:           toolPlan,
		WillCompare:        willCompare,
		ApprovalRequired:   true,
		CreatedAt:          time.Now().UTC(),
	}
}

func expectedSections(willCompare bool) []string {
	sections := []string{
		"一句话总结",
		"这篇论文做了什么",
		"方法核心",
		"实验怎么做",
		"关键结果与数字",
		"作者结论",
		"局限与注意点",
	}
	if willCompare {
		return append([]string{"任务目标", "论文列表", "逐篇一句话总结", "方法对比", "实验设置对比", "关键结果与数字对比", "结论与适用场景", "局限与不确定点"}, sections...)
	}
	return sections
}

func sortDigests(digests []protocol.PaperDigest) {
	sort.Slice(digests, func(i, j int) bool {
		return digests[i].PaperID < digests[j].PaperID
	})
}

