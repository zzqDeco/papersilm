package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/internal/pipeline"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

var builtinSkillDescriptors = []protocol.SkillDescriptor{
	{
		Name:         protocol.SkillNameReviewer,
		Title:        "Reviewer",
		Summary:      "Generate a reviewer-style assessment for a single paper.",
		TargetKind:   protocol.SkillTargetKindPaper,
		ArtifactKind: "reviewer_skill",
	},
	{
		Name:         protocol.SkillNameEquationExplain,
		Title:        "Equation Explain",
		Summary:      "Explain the key equations, assumptions, and likely failure modes.",
		TargetKind:   protocol.SkillTargetKindPaper,
		ArtifactKind: "equation_explain_skill",
	},
	{
		Name:         protocol.SkillNameRelatedWorkMap,
		Title:        "Related Work Map",
		Summary:      "Build a document-grounded map of related methods and follow-up checks.",
		TargetKind:   protocol.SkillTargetKindPaper,
		ArtifactKind: "related_work_map_skill",
	},
	{
		Name:         protocol.SkillNameCompareRefinement,
		Title:        "Compare Refinement",
		Summary:      "Refine an existing comparison into a clearer decision frame and next checks.",
		TargetKind:   protocol.SkillTargetKindComparison,
		ArtifactKind: "compare_refinement_skill",
	},
}

type reviewerSkillArtifact struct {
	PaperID     string    `json:"paper_id"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Strengths   []string  `json:"strengths"`
	Weaknesses  []string  `json:"weaknesses"`
	Questions   []string  `json:"questions"`
	Confidence  string    `json:"confidence"`
	GeneratedAt time.Time `json:"generated_at"`
}

type equationExplainSkillArtifact struct {
	PaperID      string    `json:"paper_id"`
	Title        string    `json:"title"`
	FocusPoints  []string  `json:"focus_points"`
	Intuition    []string  `json:"intuition"`
	Assumptions  []string  `json:"assumptions"`
	FailureModes []string  `json:"failure_modes"`
	GeneratedAt  time.Time `json:"generated_at"`
}

type relatedWorkMapSkillArtifact struct {
	PaperID         string    `json:"paper_id"`
	Title           string    `json:"title"`
	ComparisonAxes  []string  `json:"comparison_axes"`
	MethodNeighbors []string  `json:"method_neighbors"`
	Gaps            []string  `json:"gaps"`
	FollowUpChecks  []string  `json:"follow_up_checks"`
	GeneratedAt     time.Time `json:"generated_at"`
}

type compareRefinementSkillArtifact struct {
	ComparisonID          string    `json:"comparison_id"`
	PaperIDs              []string  `json:"paper_ids"`
	DecisionFrame         []string  `json:"decision_frame"`
	MajorDeltas           []string  `json:"major_deltas"`
	EvidenceGaps          []string  `json:"evidence_gaps"`
	RecommendedNextChecks []string  `json:"recommended_next_checks"`
	GeneratedAt           time.Time `json:"generated_at"`
}

func (a *Agent) ListSkills(_ string) ([]protocol.SkillDescriptor, error) {
	return append([]protocol.SkillDescriptor(nil), builtinSkillDescriptors...), nil
}

func (a *Agent) RunSkill(ctx context.Context, store *storage.Store, sink EventSink, sessionID, skillName, targetID string) (protocol.SkillRunResult, error) {
	snapshot, err := store.Snapshot(sessionID)
	if err != nil {
		return protocol.SkillRunResult{}, err
	}

	descriptor, err := lookupSkillDescriptor(skillName)
	if err != nil {
		return protocol.SkillRunResult{}, err
	}
	resolvedTargetID, err := resolveSkillTarget(descriptor, snapshot, targetID)
	if err != nil {
		return protocol.SkillRunResult{}, err
	}

	now := time.Now().UTC()
	run := protocol.SkillRunRecord{
		RunID:      fmt.Sprintf("skill_%d", now.UnixNano()),
		SessionID:  sessionID,
		SkillName:  descriptor.Name,
		TargetKind: descriptor.TargetKind,
		TargetID:   resolvedTargetID,
		Status:     protocol.SkillRunStatusRunning,
		Title:      skillRunTitle(descriptor, snapshot, resolvedTargetID),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := store.SaveSkillRun(sessionID, run); err != nil {
		return protocol.SkillRunResult{}, err
	}
	if err := a.emit(store, sink, sessionID, protocol.EventAnalysis, "skill run started", run); err != nil {
		return protocol.SkillRunResult{}, err
	}

	manifest, summary, runErr := a.executeSkill(ctx, store, snapshot, descriptor, run)
	if runErr != nil {
		run.Status = protocol.SkillRunStatusFailed
		run.Error = runErr.Error()
		run.UpdatedAt = time.Now().UTC()
		_ = store.SaveSkillRun(sessionID, run)
		_ = a.emit(store, sink, sessionID, protocol.EventError, "skill run failed", run)
		return protocol.SkillRunResult{}, runErr
	}

	run.Status = protocol.SkillRunStatusCompleted
	run.ArtifactID = manifest.ArtifactID
	run.Summary = strings.TrimSpace(summary)
	run.UpdatedAt = time.Now().UTC()
	if err := store.SaveSkillRun(sessionID, run); err != nil {
		return protocol.SkillRunResult{}, err
	}
	if err := a.emit(store, sink, sessionID, protocol.EventArtifactWritten, "skill artifact written", manifest); err != nil {
		return protocol.SkillRunResult{}, err
	}
	fresh, err := store.Snapshot(sessionID)
	if err != nil {
		return protocol.SkillRunResult{}, err
	}
	result := protocol.SkillRunResult{
		Session:    fresh,
		Descriptor: descriptor,
		Run:        run,
		Artifact:   &manifest,
	}
	if err := a.emit(store, sink, sessionID, protocol.EventResult, "skill run completed", result.Run); err != nil {
		return protocol.SkillRunResult{}, err
	}
	return result, nil
}

func lookupSkillDescriptor(name string) (protocol.SkillDescriptor, error) {
	needle := protocol.SkillName(strings.TrimSpace(name))
	for _, descriptor := range builtinSkillDescriptors {
		if descriptor.Name == needle {
			return descriptor, nil
		}
	}
	return protocol.SkillDescriptor{}, fmt.Errorf("unknown skill: %s", name)
}

func resolveSkillTarget(descriptor protocol.SkillDescriptor, snapshot protocol.SessionSnapshot, targetID string) (string, error) {
	targetID = strings.TrimSpace(targetID)
	switch descriptor.TargetKind {
	case protocol.SkillTargetKindPaper:
		if targetID == "" {
			if len(snapshot.Workspaces) == 1 {
				return snapshot.Workspaces[0].PaperID, nil
			}
			ids := make([]string, 0, len(snapshot.Workspaces))
			for _, workspace := range snapshot.Workspaces {
				ids = append(ids, workspace.PaperID)
			}
			sort.Strings(ids)
			return "", fmt.Errorf("skill %s requires a paper_id target; available papers: %s", descriptor.Name, strings.Join(ids, ", "))
		}
		for _, workspace := range snapshot.Workspaces {
			if workspace.PaperID == targetID {
				return targetID, nil
			}
		}
		return "", fmt.Errorf("paper not found in current session: %s", targetID)
	case protocol.SkillTargetKindComparison:
		if targetID == "" || targetID == "comparison" {
			if snapshot.Compare == nil || len(snapshot.Digests) < 2 {
				return "", fmt.Errorf("skill %s requires an existing comparison with at least two digests", descriptor.Name)
			}
			return "comparison", nil
		}
		return "", fmt.Errorf("skill %s only supports target 'comparison'", descriptor.Name)
	default:
		return "", fmt.Errorf("unsupported skill target kind: %s", descriptor.TargetKind)
	}
}

func skillRunTitle(descriptor protocol.SkillDescriptor, snapshot protocol.SessionSnapshot, targetID string) string {
	label := targetID
	if descriptor.TargetKind == protocol.SkillTargetKindComparison {
		label = "comparison"
	} else if workspace, ok := findWorkspaceByPaperID(snapshot.Workspaces, targetID); ok {
		if workspace.Digest != nil && strings.TrimSpace(workspace.Digest.Title) != "" {
			label = workspace.Digest.Title
		} else if workspace.Source != nil && strings.TrimSpace(workspace.Source.Inspection.Title) != "" {
			label = workspace.Source.Inspection.Title
		}
	}
	return descriptor.Title + ": " + label
}

func (a *Agent) executeSkill(ctx context.Context, store *storage.Store, snapshot protocol.SessionSnapshot, descriptor protocol.SkillDescriptor, run protocol.SkillRunRecord) (protocol.ArtifactManifest, string, error) {
	switch descriptor.Name {
	case protocol.SkillNameReviewer:
		workspace, ok := findWorkspaceByPaperID(snapshot.Workspaces, run.TargetID)
		if !ok {
			return protocol.ArtifactManifest{}, "", fmt.Errorf("workspace not found: %s", run.TargetID)
		}
		payload, markdown, summary, err := a.buildReviewerSkill(ctx, snapshot.Meta.SessionID, snapshot.Meta.Language, workspace)
		if err != nil {
			return protocol.ArtifactManifest{}, "", err
		}
		manifest, err := persistSkillArtifact(store, snapshot.Meta.SessionID, run.RunID, descriptor.ArtifactKind, run.TargetID, snapshot.Meta.Language, markdown, payload)
		return manifest, summary, err
	case protocol.SkillNameEquationExplain:
		workspace, ok := findWorkspaceByPaperID(snapshot.Workspaces, run.TargetID)
		if !ok {
			return protocol.ArtifactManifest{}, "", fmt.Errorf("workspace not found: %s", run.TargetID)
		}
		payload, markdown, summary, err := a.buildEquationExplainSkill(ctx, snapshot.Meta.SessionID, snapshot.Meta.Language, workspace)
		if err != nil {
			return protocol.ArtifactManifest{}, "", err
		}
		manifest, err := persistSkillArtifact(store, snapshot.Meta.SessionID, run.RunID, descriptor.ArtifactKind, run.TargetID, snapshot.Meta.Language, markdown, payload)
		return manifest, summary, err
	case protocol.SkillNameRelatedWorkMap:
		workspace, ok := findWorkspaceByPaperID(snapshot.Workspaces, run.TargetID)
		if !ok {
			return protocol.ArtifactManifest{}, "", fmt.Errorf("workspace not found: %s", run.TargetID)
		}
		payload, markdown, summary, err := a.buildRelatedWorkMapSkill(ctx, snapshot.Meta.SessionID, snapshot.Meta.Language, workspace)
		if err != nil {
			return protocol.ArtifactManifest{}, "", err
		}
		manifest, err := persistSkillArtifact(store, snapshot.Meta.SessionID, run.RunID, descriptor.ArtifactKind, run.TargetID, snapshot.Meta.Language, markdown, payload)
		return manifest, summary, err
	case protocol.SkillNameCompareRefinement:
		if snapshot.Compare == nil || len(snapshot.Digests) < 2 {
			return protocol.ArtifactManifest{}, "", fmt.Errorf("compare-refinement requires an existing comparison with at least two digests")
		}
		payload, markdown, summary := buildCompareRefinementSkill(*snapshot.Compare, snapshot.Digests)
		manifest, err := persistSkillArtifact(store, snapshot.Meta.SessionID, run.RunID, descriptor.ArtifactKind, "comparison", snapshot.Meta.Language, markdown, payload)
		return manifest, summary, err
	default:
		return protocol.ArtifactManifest{}, "", fmt.Errorf("unsupported skill: %s", descriptor.Name)
	}
}

func (a *Agent) buildReviewerSkill(ctx context.Context, sessionID, lang string, workspace protocol.PaperWorkspace) (reviewerSkillArtifact, string, string, error) {
	digest, err := a.ensureDigestForSkill(ctx, sessionID, workspace, lang)
	if err != nil {
		return reviewerSkillArtifact{}, "", "", err
	}
	confidence := "medium"
	if workspace.Digest != nil {
		confidence = "high"
	}
	payload := reviewerSkillArtifact{
		PaperID:    workspace.PaperID,
		Title:      digest.Title,
		Summary:    firstNonEmpty(digest.OneLineSummary, digest.Problem, "该论文需要进一步精读后再给出更完整审稿意见。"),
		Strengths:  nonEmptyUnique(digest.MethodSummary, digest.ExperimentSummary, []string{firstResultClaim(digest.KeyResults)}),
		Weaknesses: nonEmptyUnique(digest.Limitations, []string{"关键实验与统计显著性仍需回到原文核对。"}),
		Questions: nonEmptyUnique(
			[]string{
				"作者是否提供了足够的消融或对照实验来支撑核心设计选择？",
				"实验设置是否覆盖了目标应用最关键的边界条件？",
			},
			[]string{firstNonEmpty(digest.Problem, "")},
		),
		Confidence:  confidence,
		GeneratedAt: time.Now().UTC(),
	}
	if len(payload.Strengths) > 3 {
		payload.Strengths = payload.Strengths[:3]
	}
	if len(payload.Weaknesses) > 3 {
		payload.Weaknesses = payload.Weaknesses[:3]
	}
	if len(payload.Questions) > 3 {
		payload.Questions = payload.Questions[:3]
	}
	markdown := renderReviewerSkill(payload)
	return payload, markdown, payload.Summary, nil
}

func (a *Agent) buildEquationExplainSkill(ctx context.Context, sessionID, lang string, workspace protocol.PaperWorkspace) (equationExplainSkillArtifact, string, string, error) {
	material, err := a.loadWorkspaceMaterial(ctx, sessionID, workspace, "explain the key equations and assumptions")
	if err != nil {
		return equationExplainSkillArtifact{}, "", "", err
	}
	math := a.tools.Pipeline().BuildMathReasoning(ctx, material, "equation explain")
	digest, _ := a.ensureDigestForSkill(ctx, sessionID, workspace, lang)
	payload := equationExplainSkillArtifact{
		PaperID:      workspace.PaperID,
		Title:        firstNonEmpty(digest.Title, workspace.PaperID),
		FocusPoints:  fallbackSlice(trimmedLimit(math.Notes, 4), "当前文本里没有足够清晰的公式段落，只能给出保守说明。"),
		Intuition:    fallbackSlice(trimmedLimit(digest.MethodSummary, 3), "建议先结合方法章节和符号定义回看原文。"),
		Assumptions:  fallbackSlice(nonEmptyUnique([]string{firstNonEmpty(digest.Problem, "方法默认训练/评估数据与论文目标场景一致。")}), "方法前提需要结合原文定义核对。"),
		FailureModes: fallbackSlice(trimmedLimit(digest.Limitations, 3), "当公式定义、边界条件或符号说明不完整时，解释结论会明显变弱。"),
		GeneratedAt:  time.Now().UTC(),
	}
	markdown := renderEquationExplainSkill(payload)
	return payload, markdown, firstNonEmpty(payload.FocusPoints...), nil
}

func (a *Agent) buildRelatedWorkMapSkill(ctx context.Context, sessionID, lang string, workspace protocol.PaperWorkspace) (relatedWorkMapSkillArtifact, string, string, error) {
	material, err := a.loadWorkspaceMaterial(ctx, sessionID, workspace, "map related work and adjacent methods")
	if err != nil {
		return relatedWorkMapSkillArtifact{}, "", "", err
	}
	digest, err := a.ensureDigestForSkill(ctx, sessionID, workspace, lang)
	if err != nil {
		return relatedWorkMapSkillArtifact{}, "", "", err
	}
	relatedSentences := collectChunkSentences(material, 4, "related_work", "background", "method")
	payload := relatedWorkMapSkillArtifact{
		PaperID:         workspace.PaperID,
		Title:           firstNonEmpty(digest.Title, workspace.PaperID),
		ComparisonAxes:  fallbackSlice(nonEmptyUnique([]string{digest.Problem}, digest.MethodSummary, digest.ExperimentSummary), "需要围绕任务定义、方法设计和评估设置三条主轴来定位相关工作。"),
		MethodNeighbors: fallbackSlice(relatedSentences, "当前可提取文本没有清晰列出相关工作，可先从 introduction / related work 段落回查。"),
		Gaps:            fallbackSlice(trimmedLimit(digest.Limitations, 3), "文内没有明确列出的空白点，需要结合实验未覆盖的场景继续核对。"),
		FollowUpChecks: []string{
			"回到 related work 段落，确认作者比较的是方法框架、训练策略还是评估设定。",
			"核对论文是否给出与最强 baseline 的同条件实验，而不是不同设定下的间接比较。",
			"确认作者宣称的优势来自方法本身，而不是数据、模型规模或 prompt 预算差异。",
		},
		GeneratedAt: time.Now().UTC(),
	}
	if len(payload.ComparisonAxes) > 4 {
		payload.ComparisonAxes = payload.ComparisonAxes[:4]
	}
	if len(payload.MethodNeighbors) > 4 {
		payload.MethodNeighbors = payload.MethodNeighbors[:4]
	}
	markdown := renderRelatedWorkMapSkill(payload)
	return payload, markdown, firstNonEmpty(payload.ComparisonAxes...), nil
}

func buildCompareRefinementSkill(cmp protocol.ComparisonDigest, digests []protocol.PaperDigest) (compareRefinementSkillArtifact, string, string) {
	majorDeltas := make([]string, 0, 3)
	for _, row := range append(append([]protocol.ComparisonMatrixRow{}, cmp.MethodMatrix...), append(cmp.ExperimentMatrix, cmp.ResultMatrix...)...) {
		if len(row.Values) == 0 {
			continue
		}
		keys := make([]string, 0, len(row.Values))
		for key := range row.Values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, key+"="+strings.TrimSpace(row.Values[key]))
		}
		majorDeltas = append(majorDeltas, row.Dimension+": "+strings.Join(parts, " | "))
	}
	evidenceGaps := append([]string(nil), cmp.Limitations...)
	for _, digest := range digests {
		if len(digest.KeyResults) == 0 {
			evidenceGaps = append(evidenceGaps, digest.PaperID+": 缺少足够明确的量化结果，最好回到原文结果表核对。")
		}
	}
	payload := compareRefinementSkillArtifact{
		ComparisonID: "comparison",
		PaperIDs:     append([]string(nil), cmp.PaperIDs...),
		DecisionFrame: fallbackSlice(nonEmptyUnique(
			[]string{cmp.Goal},
			cmp.Synthesis,
		), "优先按任务目标、方法差异、实验有效性三个层面收敛比较结论。"),
		MajorDeltas:           fallbackSlice(majorDeltas, "当前 comparison 中还没有足够结构化的差异项。"),
		EvidenceGaps:          fallbackSlice(evidenceGaps, "现有 comparison 缺少更细的实验与结果证据，需要回查原文表格。"),
		RecommendedNextChecks: []string{"核对每篇论文的实验设置是否真正可比。", "把结论按适用场景拆开，而不是只保留总分高低。", "对关键 claim 回到原文表格和附录做二次确认。"},
		GeneratedAt:           time.Now().UTC(),
	}
	markdown := renderCompareRefinementSkill(payload)
	return payload, markdown, firstNonEmpty(payload.DecisionFrame...)
}

func (a *Agent) ensureDigestForSkill(ctx context.Context, sessionID string, workspace protocol.PaperWorkspace, lang string) (protocol.PaperDigest, error) {
	if workspace.Digest != nil {
		return *workspace.Digest, nil
	}
	material, err := a.loadWorkspaceMaterial(ctx, sessionID, workspace, "skill analysis")
	if err != nil {
		return protocol.PaperDigest{}, err
	}
	summary := a.tools.Pipeline().BuildPaperSummary(ctx, material, lang, "distill")
	experiment := a.tools.Pipeline().BuildExperimentOutput(ctx, material)
	var math *pipeline.MathReasoningOutput
	out := a.tools.Pipeline().BuildMathReasoning(ctx, material, "explain equations")
	math = &out
	return a.tools.Pipeline().MergePaperDigest(summary, experiment, math, nil, lang, "distill"), nil
}

func (a *Agent) loadWorkspaceMaterial(ctx context.Context, sessionID string, workspace protocol.PaperWorkspace, goal string) (pipeline.SourceMaterial, error) {
	if workspace.Source == nil {
		return pipeline.SourceMaterial{}, fmt.Errorf("workspace source not found: %s", workspace.PaperID)
	}
	return a.tools.Pipeline().LoadSourceMaterial(ctx, sessionID, *workspace.Source, goal)
}

func findWorkspaceByPaperID(workspaces []protocol.PaperWorkspace, paperID string) (protocol.PaperWorkspace, bool) {
	for _, workspace := range workspaces {
		if workspace.PaperID == paperID {
			return workspace, true
		}
	}
	return protocol.PaperWorkspace{}, false
}

func collectChunkSentences(material pipeline.SourceMaterial, limit int, sections ...string) []string {
	allowed := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		allowed[section] = struct{}{}
	}
	out := make([]string, 0, limit)
	seen := map[string]struct{}{}
	for _, chunk := range material.Chunks {
		if len(allowed) > 0 {
			if _, ok := allowed[chunk.Section]; !ok {
				continue
			}
		}
		for _, sentence := range splitSkillSentences(chunk.Content) {
			if len(sentence) < 30 {
				continue
			}
			if _, ok := seen[sentence]; ok {
				continue
			}
			seen[sentence] = struct{}{}
			out = append(out, sentence)
			if len(out) == limit {
				return out
			}
		}
	}
	return out
}

func splitSkillSentences(content string) []string {
	parts := strings.FieldsFunc(content, func(r rune) bool {
		switch r {
		case '\n', '\r', '.', '。', '!', '！', '?', '？':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func persistSkillArtifact(store *storage.Store, sessionID, artifactID, kind, source, lang, markdown string, payload any) (protocol.ArtifactManifest, error) {
	base := filepath.Join(store.BaseDir(), "sessions", sessionID, "skill-artifacts")
	mdPath := filepath.Join(base, artifactID+".md")
	jsonPath := filepath.Join(base, artifactID+".json")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return protocol.ArtifactManifest{}, err
	}
	if err := os.WriteFile(mdPath, []byte(markdown), 0o644); err != nil {
		return protocol.ArtifactManifest{}, err
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return protocol.ArtifactManifest{}, err
	}
	if err := os.WriteFile(jsonPath, raw, 0o644); err != nil {
		return protocol.ArtifactManifest{}, err
	}
	manifest := protocol.ArtifactManifest{
		ArtifactID: artifactID,
		SessionID:  sessionID,
		Kind:       kind,
		Source:     source,
		Language:   lang,
		Format:     protocol.ArtifactFormatMarkdown,
		Paths: map[string]string{
			"markdown": mdPath,
			"json":     jsonPath,
		},
		Metadata: map[string]interface{}{
			"generated_at": time.Now().UTC().Format(time.RFC3339),
		},
		CreatedAt: time.Now().UTC(),
	}
	return manifest, store.SaveSkillArtifactManifest(sessionID, manifest)
}

func renderReviewerSkill(payload reviewerSkillArtifact) string {
	var b strings.Builder
	b.WriteString("# Reviewer\n\n")
	writeSkillSection(&b, "Summary", []string{payload.Summary})
	writeSkillSection(&b, "Strengths", payload.Strengths)
	writeSkillSection(&b, "Weaknesses", payload.Weaknesses)
	writeSkillSection(&b, "Questions", payload.Questions)
	writeSkillSection(&b, "Confidence", []string{payload.Confidence})
	return b.String()
}

func renderEquationExplainSkill(payload equationExplainSkillArtifact) string {
	var b strings.Builder
	b.WriteString("# Equation Explain\n\n")
	writeSkillSection(&b, "Focus Points", payload.FocusPoints)
	writeSkillSection(&b, "Intuition", payload.Intuition)
	writeSkillSection(&b, "Assumptions", payload.Assumptions)
	writeSkillSection(&b, "Failure Modes", payload.FailureModes)
	return b.String()
}

func renderRelatedWorkMapSkill(payload relatedWorkMapSkillArtifact) string {
	var b strings.Builder
	b.WriteString("# Related Work Map\n\n")
	writeSkillSection(&b, "Comparison Axes", payload.ComparisonAxes)
	writeSkillSection(&b, "Method Neighbors", payload.MethodNeighbors)
	writeSkillSection(&b, "Gaps", payload.Gaps)
	writeSkillSection(&b, "Follow Up Checks", payload.FollowUpChecks)
	return b.String()
}

func renderCompareRefinementSkill(payload compareRefinementSkillArtifact) string {
	var b strings.Builder
	b.WriteString("# Compare Refinement\n\n")
	writeSkillSection(&b, "Decision Frame", payload.DecisionFrame)
	writeSkillSection(&b, "Major Deltas", payload.MajorDeltas)
	writeSkillSection(&b, "Evidence Gaps", payload.EvidenceGaps)
	writeSkillSection(&b, "Recommended Next Checks", payload.RecommendedNextChecks)
	return b.String()
}

func writeSkillSection(b *strings.Builder, title string, lines []string) {
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func firstResultClaim(results []protocol.KeyResult) string {
	if len(results) == 0 {
		return ""
	}
	return strings.TrimSpace(results[0].Claim)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func trimmedLimit(values []string, limit int) []string {
	out := make([]string, 0, min(limit, len(values)))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
		if len(out) == limit {
			break
		}
	}
	return out
}

func nonEmptyUnique(groups ...[]string) []string {
	out := make([]string, 0, 6)
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}

func fallbackSlice(values []string, fallback string) []string {
	if len(values) == 0 {
		return []string{fallback}
	}
	return values
}
