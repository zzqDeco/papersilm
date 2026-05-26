package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func (r *Runtime) ListSkills(sessionID string) ([]protocol.SkillDescriptor, error) {
	meta, err := r.store.LoadMeta(sessionID)
	if err != nil {
		return nil, err
	}
	return skillDescriptors(meta.Language), nil
}

func (r *Runtime) RunSkill(ctx context.Context, sessionID, skillName, targetID string) (protocol.SkillRunResult, error) {
	snapshot, err := r.store.Snapshot(sessionID)
	if err != nil {
		return protocol.SkillRunResult{}, err
	}
	descriptor, err := lookupSkillDescriptor(skillName, snapshot.Meta.Language)
	if err != nil {
		return protocol.SkillRunResult{}, err
	}
	if strings.TrimSpace(targetID) == "" {
		targetID = defaultSkillTarget(descriptor, snapshot)
	}
	if strings.TrimSpace(targetID) == "" {
		return protocol.SkillRunResult{}, fmt.Errorf("skill target is required")
	}
	paperIDs := []string{targetID}
	if descriptor.TargetKind == protocol.SkillTargetKindComparison {
		if snapshot.Compare == nil || len(snapshot.Compare.PaperIDs) == 0 {
			return protocol.SkillRunResult{}, fmt.Errorf("comparison skill requires an existing comparison")
		}
		if targetID != "comparison" {
			return protocol.SkillRunResult{}, fmt.Errorf("comparison skill target must be comparison")
		}
		paperIDs = append([]string(nil), snapshot.Compare.PaperIDs...)
	}
	now := time.Now().UTC()
	run := protocol.SkillRunRecord{
		RunID:      fmt.Sprintf("skill_%d", now.UnixNano()),
		SessionID:  sessionID,
		SkillName:  descriptor.Name,
		TargetKind: descriptor.TargetKind,
		TargetID:   targetID,
		PaperIDs:   paperIDs,
		Status:     protocol.SkillRunStatusRunning,
		Title:      descriptor.Title,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := r.store.SaveSkillRun(sessionID, run); err != nil {
		return protocol.SkillRunResult{}, err
	}
	markdown := skillMarkdown(descriptor, snapshot, targetID)
	manifest, err := r.writeSkillArtifact(sessionID, run.RunID, descriptor.ArtifactKind, descriptor.Title, snapshot.Meta.Language, markdown, map[string]any{
		"skill":     descriptor.Name,
		"target":    targetID,
		"paper_ids": paperIDs,
		"markdown":  markdown,
	})
	if err != nil {
		run.Status = protocol.SkillRunStatusFailed
		run.Error = err.Error()
		run.UpdatedAt = time.Now().UTC()
		_ = r.store.SaveSkillRun(sessionID, run)
		return protocol.SkillRunResult{}, err
	}
	run.Status = protocol.SkillRunStatusCompleted
	run.ArtifactID = manifest.ArtifactID
	run.Summary = descriptor.Summary
	run.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveSkillRun(sessionID, run); err != nil {
		return protocol.SkillRunResult{}, err
	}
	fresh, err := r.store.Snapshot(sessionID)
	if err != nil {
		return protocol.SkillRunResult{}, err
	}
	result := protocol.SkillRunResult{Session: fresh, Descriptor: descriptor, Run: run, Artifact: &manifest}
	_ = ctx
	_ = r.emit(sessionID, protocol.EventArtifactWritten, "skill artifact written", manifest)
	_ = r.emit(sessionID, protocol.EventResult, "skill run completed", run)
	return result, nil
}

func skillDescriptors(lang string) []protocol.SkillDescriptor {
	zh := !strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "en")
	if zh {
		return []protocol.SkillDescriptor{
			{Name: protocol.SkillNameReviewer, Title: "审稿视角", Summary: "为单篇论文生成审稿式评估。", TargetKind: protocol.SkillTargetKindPaper, ArtifactKind: "reviewer_skill"},
			{Name: protocol.SkillNameEquationExplain, Title: "公式解读", Summary: "解释关键公式、前提假设和可能的失效方式。", TargetKind: protocol.SkillTargetKindPaper, ArtifactKind: "equation_explain_skill"},
			{Name: protocol.SkillNameRelatedWorkMap, Title: "相关工作映射", Summary: "基于当前文档构建相关方法、比较维度和后续核查项。", TargetKind: protocol.SkillTargetKindPaper, ArtifactKind: "related_work_map_skill"},
			{Name: protocol.SkillNameCompareRefinement, Title: "对比精炼", Summary: "把已有 comparison 进一步收敛成更清晰的决策框架。", TargetKind: protocol.SkillTargetKindComparison, ArtifactKind: "compare_refinement_skill"},
		}
	}
	return []protocol.SkillDescriptor{
		{Name: protocol.SkillNameReviewer, Title: "Reviewer", Summary: "Generate a reviewer-style assessment for a single paper.", TargetKind: protocol.SkillTargetKindPaper, ArtifactKind: "reviewer_skill"},
		{Name: protocol.SkillNameEquationExplain, Title: "Equation Explain", Summary: "Explain key equations, assumptions, and failure modes.", TargetKind: protocol.SkillTargetKindPaper, ArtifactKind: "equation_explain_skill"},
		{Name: protocol.SkillNameRelatedWorkMap, Title: "Related Work Map", Summary: "Build a document-grounded map of related methods.", TargetKind: protocol.SkillTargetKindPaper, ArtifactKind: "related_work_map_skill"},
		{Name: protocol.SkillNameCompareRefinement, Title: "Compare Refinement", Summary: "Refine an existing comparison into a clearer decision frame.", TargetKind: protocol.SkillTargetKindComparison, ArtifactKind: "compare_refinement_skill"},
	}
}

func lookupSkillDescriptor(name, lang string) (protocol.SkillDescriptor, error) {
	for _, descriptor := range skillDescriptors(lang) {
		if string(descriptor.Name) == strings.TrimSpace(name) {
			return descriptor, nil
		}
	}
	return protocol.SkillDescriptor{}, fmt.Errorf("unknown skill: %s", name)
}

func defaultSkillTarget(descriptor protocol.SkillDescriptor, snapshot protocol.SessionSnapshot) string {
	if descriptor.TargetKind == protocol.SkillTargetKindComparison {
		if snapshot.Compare != nil {
			return "comparison"
		}
		return ""
	}
	if len(snapshot.Sources) > 0 {
		return snapshot.Sources[0].PaperID
	}
	if len(snapshot.Digests) > 0 {
		return snapshot.Digests[0].PaperID
	}
	return ""
}

func skillMarkdown(descriptor protocol.SkillDescriptor, snapshot protocol.SessionSnapshot, targetID string) string {
	title := descriptor.Title
	if targetID != "" {
		title += " · " + targetID
	}
	lines := []string{"# " + title, "", descriptor.Summary, ""}
	if descriptor.TargetKind == protocol.SkillTargetKindComparison && snapshot.Compare != nil {
		lines = append(lines, "## Decision Frame", "", strings.Join(snapshot.Compare.Synthesis, "\n"))
	} else {
		for _, digest := range snapshot.Digests {
			if digest.PaperID != targetID {
				continue
			}
			lines = append(lines, "## Summary", "", digest.OneLineSummary, "", "## Follow-up", "", "- Verify key claims against the original paper.", "- Check limitations and experimental setup.")
			break
		}
	}
	return strings.Join(lines, "\n")
}

func (r *Runtime) writeSkillArtifact(sessionID, artifactID, kind, title, lang, markdown string, payload any) (protocol.ArtifactManifest, error) {
	base := filepath.Join(r.store.BaseDir(), "sessions", sessionID, "skills")
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
		Source:     title,
		Language:   lang,
		Format:     protocol.ArtifactFormatMarkdown,
		Paths: map[string]string{
			"markdown": mdPath,
			"json":     jsonPath,
		},
		CreatedAt: time.Now().UTC(),
	}
	if payloadMap, ok := payload.(map[string]any); ok {
		if paperIDs, ok := payloadMap["paper_ids"]; ok {
			manifest.Metadata = map[string]interface{}{"paper_ids": paperIDs}
		}
	}
	if err := r.store.SaveSkillArtifactManifest(sessionID, manifest); err != nil {
		return protocol.ArtifactManifest{}, err
	}
	return manifest, nil
}
