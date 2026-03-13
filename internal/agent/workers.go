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

type nodeResult struct {
	nodeID  string
	outputs []protocol.NodeOutputRef
	err     error
}

func (a *Agent) executeNode(ctx context.Context, store *storage.Store, sessionID, goal, lang, style string, state *protocol.ExecutionState, node protocol.PlanNode) ([]protocol.NodeOutputRef, error) {
	switch node.Kind {
	case protocol.NodeKindPaperSummary:
		ref, err := a.findSource(store, sessionID, node.PaperIDs[0])
		if err != nil {
			return nil, err
		}
		material, err := a.tools.Pipeline().LoadSourceMaterial(ctx, sessionID, ref, goal)
		if err != nil {
			return nil, err
		}
		out := a.tools.Pipeline().BuildPaperSummary(ctx, material, lang, style)
		return []protocol.NodeOutputRef{newNodeOutput(node.ID, "paper_summary", out)}, nil
	case protocol.NodeKindExperiment:
		ref, err := a.findSource(store, sessionID, node.PaperIDs[0])
		if err != nil {
			return nil, err
		}
		material, err := a.tools.Pipeline().LoadSourceMaterial(ctx, sessionID, ref, goal)
		if err != nil {
			return nil, err
		}
		out := a.tools.Pipeline().BuildExperimentOutput(ctx, material)
		return []protocol.NodeOutputRef{newNodeOutput(node.ID, "experiment", out)}, nil
	case protocol.NodeKindMathReasoner:
		ref, err := a.findSource(store, sessionID, node.PaperIDs[0])
		if err != nil {
			return nil, err
		}
		material, err := a.tools.Pipeline().LoadSourceMaterial(ctx, sessionID, ref, goal)
		if err != nil {
			return nil, err
		}
		out := a.tools.Pipeline().BuildMathReasoning(ctx, material, goal)
		return []protocol.NodeOutputRef{newNodeOutput(node.ID, "math_reasoning", out)}, nil
	case protocol.NodeKindWebResearch:
		ref, err := a.findSource(store, sessionID, node.PaperIDs[0])
		if err != nil {
			return nil, err
		}
		material, err := a.tools.Pipeline().LoadSourceMaterial(ctx, sessionID, ref, goal)
		if err != nil {
			return nil, err
		}
		out := a.tools.Pipeline().BuildWebResearch(ctx, material, goal)
		return []protocol.NodeOutputRef{newNodeOutput(node.ID, "web_research", out)}, nil
	case protocol.NodeKindMergeDigest:
		return a.executeMergeDigest(store, sessionID, lang, style, state, node)
	case protocol.NodeKindMethodCompare:
		return a.executeCompareRow(store, sessionID, node, state, a.tools.Pipeline().BuildMethodMatrix, "method_matrix")
	case protocol.NodeKindExperimentCompare:
		return a.executeCompareRow(store, sessionID, node, state, a.tools.Pipeline().BuildExperimentMatrix, "experiment_matrix")
	case protocol.NodeKindResultsCompare:
		return a.executeCompareRow(store, sessionID, node, state, a.tools.Pipeline().BuildResultsMatrix, "results_matrix")
	case protocol.NodeKindFinalSynthesis:
		return a.executeFinalSynthesis(store, sessionID, goal, lang, style, state, node)
	case protocol.NodeKind("distill_paper"):
		return a.executeLegacyDistill(ctx, store, sessionID, goal, lang, style, node)
	case protocol.NodeKind("compare_papers"):
		return a.executeLegacyCompare(store, sessionID, goal, lang, style, node)
	default:
		return nil, fmt.Errorf("unsupported node kind: %s", node.Kind)
	}
}

func (a *Agent) executeMergeDigest(store *storage.Store, sessionID, lang, style string, state *protocol.ExecutionState, node protocol.PlanNode) ([]protocol.NodeOutputRef, error) {
	summaryRef, ok := firstOutputByKind(state, "paper_summary", node.PaperIDs[0])
	if !ok {
		return nil, fmt.Errorf("missing paper_summary output for %s", node.PaperIDs[0])
	}
	experimentRef, ok := firstOutputByKind(state, "experiment", node.PaperIDs[0])
	if !ok {
		return nil, fmt.Errorf("missing experiment output for %s", node.PaperIDs[0])
	}
	var summary pipeline.PaperSummaryOutput
	if err := decodeNodeOutput(summaryRef, &summary); err != nil {
		return nil, err
	}
	var experiment pipeline.ExperimentOutput
	if err := decodeNodeOutput(experimentRef, &experiment); err != nil {
		return nil, err
	}
	var mathOutput *pipeline.MathReasoningOutput
	if mathRef, ok := firstOutputByKind(state, "math_reasoning", node.PaperIDs[0]); ok {
		var out pipeline.MathReasoningOutput
		if err := decodeNodeOutput(mathRef, &out); err == nil {
			mathOutput = &out
		}
	}
	var webOutput *pipeline.WebResearchOutput
	if webRef, ok := firstOutputByKind(state, "web_research", node.PaperIDs[0]); ok {
		var out pipeline.WebResearchOutput
		if err := decodeNodeOutput(webRef, &out); err == nil {
			webOutput = &out
		}
	}
	digest := a.tools.Pipeline().MergePaperDigest(summary, experiment, mathOutput, webOutput, lang, style)
	manifest, err := persistArtifact(store, sessionID, digest.PaperID, "paper_digest", digest.Title, lang, digest.Markdown, digest)
	if err != nil {
		return nil, err
	}
	digest.ArtifactID = manifest.ArtifactID
	if err := store.SaveDigest(sessionID, digest); err != nil {
		return nil, err
	}
	return []protocol.NodeOutputRef{
		newNodeOutput(node.ID, "paper_digest", map[string]any{
			"paper_id":    digest.PaperID,
			"artifact_id": manifest.ArtifactID,
			"title":       digest.Title,
		}),
	}, nil
}

func (a *Agent) executeCompareRow(store *storage.Store, sessionID string, node protocol.PlanNode, _ *protocol.ExecutionState, build func([]protocol.PaperDigest) protocol.ComparisonMatrixRow, kind string) ([]protocol.NodeOutputRef, error) {
	digests, err := store.LoadDigests(sessionID)
	if err != nil {
		return nil, err
	}
	selected := filterDigests(digests, node.PaperIDs)
	if len(selected) < 2 {
		return nil, fmt.Errorf("%s requires at least two digests", node.Kind)
	}
	row := build(selected)
	return []protocol.NodeOutputRef{newNodeOutput(node.ID, kind, row)}, nil
}

func (a *Agent) executeFinalSynthesis(store *storage.Store, sessionID, goal, lang, style string, state *protocol.ExecutionState, node protocol.PlanNode) ([]protocol.NodeOutputRef, error) {
	digests, err := store.LoadDigests(sessionID)
	if err != nil {
		return nil, err
	}
	selected := filterDigests(digests, node.PaperIDs)
	if len(selected) < 2 {
		return nil, fmt.Errorf("final synthesis requires at least two digests")
	}
	methodRef, ok := firstOutputByNodeID(state, "method_compare")
	if !ok {
		return nil, fmt.Errorf("missing method comparison output")
	}
	experimentRef, ok := firstOutputByNodeID(state, "experiment_compare")
	if !ok {
		return nil, fmt.Errorf("missing experiment comparison output")
	}
	resultsRef, ok := firstOutputByNodeID(state, "results_compare")
	if !ok {
		return nil, fmt.Errorf("missing results comparison output")
	}
	var methodRow protocol.ComparisonMatrixRow
	if err := decodeNodeOutput(methodRef, &methodRow); err != nil {
		return nil, err
	}
	var experimentRow protocol.ComparisonMatrixRow
	if err := decodeNodeOutput(experimentRef, &experimentRow); err != nil {
		return nil, err
	}
	var resultRow protocol.ComparisonMatrixRow
	if err := decodeNodeOutput(resultsRef, &resultRow); err != nil {
		return nil, err
	}
	cmp := a.tools.Pipeline().BuildFinalComparison(goal, selected, methodRow, experimentRow, resultRow, lang, style)
	manifest, err := persistArtifact(store, sessionID, "comparison", "comparison_digest", "comparison", lang, cmp.Markdown, cmp)
	if err != nil {
		return nil, err
	}
	cmp.ArtifactID = manifest.ArtifactID
	if err := store.SaveComparison(sessionID, cmp); err != nil {
		return nil, err
	}
	return []protocol.NodeOutputRef{
		newNodeOutput(node.ID, "comparison_digest", map[string]any{
			"artifact_id": manifest.ArtifactID,
			"paper_ids":   cmp.PaperIDs,
		}),
	}, nil
}

func (a *Agent) executeLegacyDistill(ctx context.Context, store *storage.Store, sessionID, goal, lang, style string, node protocol.PlanNode) ([]protocol.NodeOutputRef, error) {
	ref, err := a.findSource(store, sessionID, node.PaperIDs[0])
	if err != nil {
		return nil, err
	}
	digest, err := a.tools.Pipeline().Distill(ctx, sessionID, ref, goal, lang, style)
	if err != nil {
		return nil, err
	}
	manifest, err := persistArtifact(store, sessionID, digest.PaperID, "paper_digest", digest.Title, lang, digest.Markdown, digest)
	if err != nil {
		return nil, err
	}
	digest.ArtifactID = manifest.ArtifactID
	if err := store.SaveDigest(sessionID, digest); err != nil {
		return nil, err
	}
	return []protocol.NodeOutputRef{
		newNodeOutput(node.ID, "paper_digest", map[string]any{
			"paper_id":    digest.PaperID,
			"artifact_id": manifest.ArtifactID,
		}),
	}, nil
}

func (a *Agent) executeLegacyCompare(store *storage.Store, sessionID, goal, lang, style string, node protocol.PlanNode) ([]protocol.NodeOutputRef, error) {
	digests, err := store.LoadDigests(sessionID)
	if err != nil {
		return nil, err
	}
	selected := filterDigests(digests, node.PaperIDs)
	if len(selected) < 2 {
		return nil, fmt.Errorf("compare requires at least two digests")
	}
	cmp := a.tools.Pipeline().Compare(goal, selected, lang, style)
	manifest, err := persistArtifact(store, sessionID, "comparison", "comparison_digest", "comparison", lang, cmp.Markdown, cmp)
	if err != nil {
		return nil, err
	}
	cmp.ArtifactID = manifest.ArtifactID
	if err := store.SaveComparison(sessionID, cmp); err != nil {
		return nil, err
	}
	return []protocol.NodeOutputRef{
		newNodeOutput(node.ID, "comparison_digest", map[string]any{
			"artifact_id": manifest.ArtifactID,
		}),
	}, nil
}

func (a *Agent) findSource(store *storage.Store, sessionID, paperID string) (protocol.PaperRef, error) {
	refs, err := store.LoadSources(sessionID)
	if err != nil {
		return protocol.PaperRef{}, err
	}
	for _, ref := range refs {
		if ref.PaperID == paperID {
			return ref, nil
		}
	}
	return protocol.PaperRef{}, fmt.Errorf("paper not found: %s", paperID)
}

func firstOutputByKind(state *protocol.ExecutionState, kind, paperID string) (protocol.NodeOutputRef, bool) {
	for _, out := range state.Outputs {
		if out.Kind != kind {
			continue
		}
		if value, ok := out.Data["paper_id"].(string); ok && value == paperID {
			return out, true
		}
	}
	return protocol.NodeOutputRef{}, false
}

func firstOutputByNodeID(state *protocol.ExecutionState, nodeID string) (protocol.NodeOutputRef, bool) {
	for _, out := range state.Outputs {
		if out.NodeID == nodeID {
			return out, true
		}
	}
	return protocol.NodeOutputRef{}, false
}

func newNodeOutput(nodeID, kind string, payload any) protocol.NodeOutputRef {
	raw, _ := json.Marshal(payload)
	var data map[string]any
	_ = json.Unmarshal(raw, &data)
	return protocol.NodeOutputRef{
		NodeID:    nodeID,
		Kind:      kind,
		Data:      data,
		CreatedAt: time.Now().UTC(),
	}
}

func decodeNodeOutput(ref protocol.NodeOutputRef, target any) error {
	raw, err := json.Marshal(ref.Data)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func filterDigests(digests []protocol.PaperDigest, paperIDs []string) []protocol.PaperDigest {
	if len(paperIDs) == 0 {
		return digests
	}
	allowed := make(map[string]struct{}, len(paperIDs))
	for _, id := range paperIDs {
		allowed[id] = struct{}{}
	}
	out := make([]protocol.PaperDigest, 0, len(paperIDs))
	for _, digest := range digests {
		if _, ok := allowed[digest.PaperID]; ok {
			out = append(out, digest)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PaperID < out[j].PaperID
	})
	return out
}

func persistArtifact(store *storage.Store, sessionID, artifactID, kind, source, lang, markdown string, payload any) (protocol.ArtifactManifest, error) {
	base := filepath.Join(store.BaseDir(), "sessions", sessionID, "artifacts")
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
	if strings.TrimSpace(source) == "" {
		manifest.Source = artifactID
	}
	return manifest, store.SaveArtifactManifest(sessionID, manifest)
}
