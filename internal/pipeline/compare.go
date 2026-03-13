package pipeline

import (
	"sort"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func (s *Service) Compare(goal string, digests []protocol.PaperDigest, lang, style string) protocol.ComparisonDigest {
	sortDigests(digests)
	methodRow := protocol.ComparisonMatrixRow{Dimension: "method", Values: map[string]string{}}
	experimentRow := protocol.ComparisonMatrixRow{Dimension: "experiment", Values: map[string]string{}}
	resultRow := protocol.ComparisonMatrixRow{Dimension: "results", Values: map[string]string{}}
	for _, digest := range digests {
		methodRow.Values[digest.PaperID] = firstLine(digest.MethodSummary)
		experimentRow.Values[digest.PaperID] = firstLine(digest.ExperimentSummary)
		resultRow.Values[digest.PaperID] = firstResult(digest.KeyResults)
	}
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

func collectPaperIDs(digests []protocol.PaperDigest) []string {
	out := make([]string, 0, len(digests))
	for _, digest := range digests {
		out = append(out, digest.PaperID)
	}
	return out
}

func firstLine(lines []string) string {
	if len(lines) == 0 {
		return "unknown"
	}
	return lines[0]
}

func firstResult(results []protocol.KeyResult) string {
	if len(results) == 0 {
		return "unknown"
	}
	return results[0].Claim
}

func buildSynthesis(digests []protocol.PaperDigest) []string {
	if len(digests) == 0 {
		return []string{"没有可比较的论文摘要。"}
	}
	if len(digests) == 1 {
		return []string{"当前只有一篇论文，未触发真正的跨论文比较。"}
	}
	out := []string{
		"系统已先为每篇论文建立结构化摘要，再在方法、实验和结果三个主轴上做横向聚合。",
		"如果某篇论文缺少清晰实验细节或量化结果，对比矩阵会保留 unknown，而不是补写推断。",
	}
	return out
}

func renderComparison(c protocol.ComparisonDigest) string {
	var b strings.Builder
	b.WriteString("# 多论文对比\n\n")
	writeSection(&b, "任务目标", []string{c.Goal})
	writeSection(&b, "论文列表", c.PaperIDs)
	summaries := make([]string, 0, len(c.PaperSummaries))
	for _, digest := range c.PaperSummaries {
		label := provenanceLabel(digest.ContentProvenance)
		if label != "" {
			summaries = append(summaries, digest.PaperID+" ["+label+"]: "+digest.OneLineSummary)
			continue
		}
		summaries = append(summaries, digest.PaperID+": "+digest.OneLineSummary)
	}
	writeSection(&b, "逐篇一句话总结", summaries)
	writeMatrix(&b, "方法对比", c.MethodMatrix)
	writeMatrix(&b, "实验设置对比", c.ExperimentMatrix)
	writeMatrix(&b, "关键结果与数字对比", c.ResultMatrix)
	writeSection(&b, "结论与适用场景", c.Synthesis)
	writeSection(&b, "局限与不确定点", c.Limitations)
	return b.String()
}

func writeMatrix(b *strings.Builder, title string, rows []protocol.ComparisonMatrixRow) {
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n")
	for _, row := range rows {
		keys := make([]string, 0, len(row.Values))
		for key := range row.Values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			b.WriteString("- ")
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(strings.TrimSpace(row.Values[key]))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
}
