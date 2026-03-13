package pipeline

import (
	"sort"
	"strings"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func (s *Service) Compare(goal string, digests []protocol.PaperDigest, lang, style string) protocol.ComparisonDigest {
	return s.BuildFinalComparison(goal, digests, s.BuildMethodMatrix(digests), s.BuildExperimentMatrix(digests), s.BuildResultsMatrix(digests), lang, style)
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
