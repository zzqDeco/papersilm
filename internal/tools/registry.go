package tools

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/internal/pipeline"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/internal/tools/graphtool"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type Registry struct {
	pipeline *pipeline.Service
}

func New(p *pipeline.Service) *Registry {
	return &Registry{pipeline: p}
}

func (r *Registry) Pipeline() *pipeline.Service {
	return r.pipeline
}

type DistillToolInput struct {
	PaperID string `json:"paper_id"`
	Goal    string `json:"goal,omitempty"`
	Lang    string `json:"lang,omitempty"`
	Style   string `json:"style,omitempty"`
}

type DistillToolResult struct {
	PaperID    string `json:"paper_id"`
	DigestID   string `json:"digest_id"`
	ArtifactID string `json:"artifact_id"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

type CompareToolInput struct {
	PaperIDs []string `json:"paper_ids"`
	Goal     string   `json:"goal,omitempty"`
	Lang     string   `json:"lang,omitempty"`
	Style    string   `json:"style,omitempty"`
}

type CompareToolResult struct {
	ComparisonID string `json:"comparison_id"`
	ArtifactID   string `json:"artifact_id"`
	Status       string `json:"status"`
	Reason       string `json:"reason,omitempty"`
}

type ExportArtifactInput struct {
	ArtifactID string `json:"artifact_id"`
	Format     string `json:"format,omitempty"`
	Path       string `json:"path,omitempty"`
}

type ExportArtifactResult struct {
	ArtifactID string            `json:"artifact_id"`
	Paths      map[string]string `json:"paths"`
}

type SessionAssets struct {
	Sources   []protocol.PaperRef         `json:"sources,omitempty"`
	Digests   []protocol.PaperDigest      `json:"digests,omitempty"`
	Compare   *protocol.ComparisonDigest  `json:"comparison,omitempty"`
	Artifacts []protocol.ArtifactManifest `json:"artifacts,omitempty"`
}

type approvalToolInput struct {
	PlanID  string `json:"plan_id"`
	Summary string `json:"summary"`
}

func init() {
	gob.Register(map[string]string{})
	schema.Register[*approvalToolInput]()
}

func (r *Registry) AttachSources(ctx context.Context, store *storage.Store, sessionID string, raw []string) ([]protocol.PaperRef, error) {
	existing, err := store.LoadSources(sessionID)
	if err != nil {
		return nil, err
	}
	combined := make([]string, 0, len(existing)+len(raw))
	seen := make(map[string]struct{}, len(existing)+len(raw))
	for _, ref := range existing {
		if ref.URI == "" {
			continue
		}
		if _, ok := seen[ref.URI]; ok {
			continue
		}
		seen[ref.URI] = struct{}{}
		combined = append(combined, ref.URI)
	}
	for _, src := range raw {
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}
		if _, ok := seen[src]; ok {
			continue
		}
		seen[src] = struct{}{}
		combined = append(combined, src)
	}
	refs, err := r.pipeline.NormalizeSources(ctx, sessionID, combined)
	if err != nil {
		return nil, err
	}
	if err := store.SaveSources(sessionID, refs); err != nil {
		return nil, err
	}
	if err := store.InvalidatePlanState(sessionID); err != nil {
		return nil, err
	}
	return refs, nil
}

func (r *Registry) InspectSources(ctx context.Context, store *storage.Store, sessionID string, paperIDs []string) ([]protocol.PaperRef, error) {
	refs, err := store.LoadSources(sessionID)
	if err != nil {
		return nil, err
	}
	targeted := make(map[string]struct{}, len(paperIDs))
	for _, paperID := range paperIDs {
		targeted[paperID] = struct{}{}
	}
	for i, ref := range refs {
		if len(targeted) > 0 {
			if _, ok := targeted[ref.PaperID]; !ok {
				continue
			}
		}
		updated, _, inspectErr := r.pipeline.InspectSource(ctx, sessionID, ref)
		refs[i] = updated
		if inspectErr != nil {
			continue
		}
	}
	if err := store.SaveSources(sessionID, refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func (r *Registry) ToolNames(includeApproval bool) []string {
	names := []string{"attach_sources", "inspect_sources"}
	if includeApproval {
		names = append(names, "approve_plan")
	}
	return append(names, "distill_paper", "compare_papers", "export_artifact", "list_session_assets")
}

func (r *Registry) BuildExecutionTools(ctx context.Context, store *storage.Store, sessionID string, includeApproval bool) ([]tool.BaseTool, error) {
	out := make([]tool.BaseTool, 0, 5)

	if includeApproval {
		approval, err := toolutils.InferTool("approve_plan", "Review the execution plan and interrupt until approval is explicitly granted.",
			func(ctx context.Context, input approvalToolInput) (string, error) {
				wasInterrupted, hasState, state := tool.GetInterruptState[*approvalToolInput](ctx)
				if wasInterrupted && hasState {
					isTarget, hasData, data := tool.GetResumeContext[string](ctx)
					if !isTarget {
						return "", tool.StatefulInterrupt(ctx, map[string]string{"plan_id": state.PlanID, "summary": state.Summary}, state)
					}
					if !hasData || strings.TrimSpace(strings.ToLower(data)) != "approved" {
						return "plan approval rejected", nil
					}
					return "plan approved", nil
				}
				return "", tool.StatefulInterrupt(ctx, map[string]string{"plan_id": input.PlanID, "summary": input.Summary}, &input)
			})
		if err != nil {
			return nil, err
		}
		out = append(out, approval)
	}

	distillTool, err := r.buildDistillTool(ctx, store, sessionID)
	if err != nil {
		return nil, err
	}
	out = append(out, distillTool)

	compareTool, err := r.buildCompareTool(ctx, store, sessionID)
	if err != nil {
		return nil, err
	}
	out = append(out, compareTool)

	exportTool, err := toolutils.InferTool("export_artifact", "Export an artifact from the session to its existing path or a user-provided path.",
		func(ctx context.Context, input ExportArtifactInput) (*ExportArtifactResult, error) {
			manifests, err := store.LoadArtifactManifests(sessionID)
			if err != nil {
				return nil, err
			}
			for _, manifest := range manifests {
				if manifest.ArtifactID != input.ArtifactID {
					continue
				}
				if strings.TrimSpace(input.Path) == "" {
					return &ExportArtifactResult{ArtifactID: manifest.ArtifactID, Paths: manifest.Paths}, nil
				}
				src := manifest.Paths["markdown"]
				if strings.EqualFold(input.Format, string(protocol.ArtifactFormatJSON)) {
					src = manifest.Paths["json"]
				}
				if src == "" {
					return nil, fmt.Errorf("artifact %s does not have format %s", manifest.ArtifactID, input.Format)
				}
				if err := copyFile(src, input.Path); err != nil {
					return nil, err
				}
				return &ExportArtifactResult{
					ArtifactID: manifest.ArtifactID,
					Paths: map[string]string{
						"source": src,
						"export": input.Path,
					},
				}, nil
			}
			return nil, fmt.Errorf("artifact not found: %s", input.ArtifactID)
		})
	if err != nil {
		return nil, err
	}
	out = append(out, exportTool)

	listAssetsTool, err := toolutils.InferTool("list_session_assets", "List the session sources, digests, comparison, and artifacts.",
		func(ctx context.Context, _ map[string]any) (*SessionAssets, error) {
			snapshot, err := store.Snapshot(sessionID)
			if err != nil {
				return nil, err
			}
			return &SessionAssets{
				Sources:   snapshot.Sources,
				Digests:   snapshot.Digests,
				Compare:   snapshot.Compare,
				Artifacts: snapshot.Artifacts,
			}, nil
		})
	if err != nil {
		return nil, err
	}
	out = append(out, listAssetsTool)

	return out, nil
}

func (r *Registry) LookupAlphaXivOverview(ctx context.Context, sessionID string, ref protocol.PaperRef) (string, bool, error) {
	return r.pipeline.LookupAlphaXivOverview(ctx, sessionID, ref)
}

func (r *Registry) LookupAlphaXivFullText(ctx context.Context, sessionID string, ref protocol.PaperRef) (string, bool, error) {
	return r.pipeline.LookupAlphaXivFullText(ctx, sessionID, ref)
}

func (r *Registry) buildDistillTool(ctx context.Context, store *storage.Store, sessionID string) (tool.BaseTool, error) {
	workflow := compose.NewWorkflow[*DistillToolInput, *DistillToolResult]()
	workflow.AddLambdaNode("distill", compose.InvokableLambda(func(ctx context.Context, input *DistillToolInput) (*DistillToolResult, error) {
		refs, err := store.LoadSources(sessionID)
		if err != nil {
			return nil, err
		}
		ref, err := findSource(refs, input.PaperID)
		if err != nil {
			return &DistillToolResult{PaperID: input.PaperID, Status: "failed", Error: err.Error()}, nil
		}
		lang := fallbackValue(input.Lang, "zh")
		style := fallbackValue(input.Style, "distill")
		digest, err := r.pipeline.Distill(ctx, sessionID, ref, input.Goal, lang, style)
		if err != nil {
			return &DistillToolResult{PaperID: input.PaperID, Status: "failed", Error: err.Error()}, nil
		}
		manifest, err := writeArtifact(store, sessionID, digest.PaperID, "paper_digest", digest.Title, lang, digest.Markdown, digest)
		if err != nil {
			return &DistillToolResult{PaperID: input.PaperID, Status: "failed", Error: err.Error()}, nil
		}
		digest.ArtifactID = manifest.ArtifactID
		if err := store.SaveDigest(sessionID, digest); err != nil {
			return &DistillToolResult{PaperID: input.PaperID, Status: "failed", Error: err.Error()}, nil
		}
		return &DistillToolResult{
			PaperID:    digest.PaperID,
			DigestID:   digest.PaperID,
			ArtifactID: manifest.ArtifactID,
			Status:     "completed",
		}, nil
	})).AddInput(compose.START)
	workflow.End().AddInput("distill")
	return graphtool.NewInvokableGraphTool[*DistillToolInput, *DistillToolResult](workflow, "distill_paper", "Distill a single paper into a structured digest artifact using AlphaXiv overview first for arXiv-capable sources, then AlphaXiv full text, and finally arXiv PDF fallback.")
}

func (r *Registry) buildCompareTool(ctx context.Context, store *storage.Store, sessionID string) (tool.BaseTool, error) {
	workflow := compose.NewWorkflow[*CompareToolInput, *CompareToolResult]()
	workflow.AddLambdaNode("compare", compose.InvokableLambda(func(ctx context.Context, input *CompareToolInput) (*CompareToolResult, error) {
		digests, err := store.LoadDigests(sessionID)
		if err != nil {
			return &CompareToolResult{ComparisonID: "comparison", Status: "failed", Reason: err.Error()}, nil
		}
		selected := filterDigests(digests, input.PaperIDs)
		if len(selected) < 2 {
			return &CompareToolResult{ComparisonID: "comparison", Status: "skipped", Reason: "compare_papers requires at least two digests"}, nil
		}
		lang := fallbackValue(input.Lang, "zh")
		style := fallbackValue(input.Style, "distill")
		goal := fallbackValue(input.Goal, "跨论文综合对比")
		cmp := r.pipeline.Compare(goal, selected, lang, style)
		manifest, err := writeArtifact(store, sessionID, "comparison", "comparison_digest", "comparison", lang, cmp.Markdown, cmp)
		if err != nil {
			return &CompareToolResult{ComparisonID: "comparison", Status: "failed", Reason: err.Error()}, nil
		}
		cmp.ArtifactID = manifest.ArtifactID
		if err := store.SaveComparison(sessionID, cmp); err != nil {
			return &CompareToolResult{ComparisonID: "comparison", Status: "failed", Reason: err.Error()}, nil
		}
		return &CompareToolResult{
			ComparisonID: "comparison",
			ArtifactID:   manifest.ArtifactID,
			Status:       "completed",
		}, nil
	})).AddInput(compose.START)
	workflow.End().AddInput("compare")
	return graphtool.NewInvokableGraphTool[*CompareToolInput, *CompareToolResult](workflow, "compare_papers", "Compare multiple papers based on previously generated structured digests.")
}

func findSource(refs []protocol.PaperRef, paperID string) (protocol.PaperRef, error) {
	for _, ref := range refs {
		if ref.PaperID == paperID {
			return ref, nil
		}
	}
	return protocol.PaperRef{}, fmt.Errorf("paper not found: %s", paperID)
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
	return out
}

func fallbackValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func writeArtifact(store *storage.Store, sessionID, artifactID, kind, source, lang, markdown string, payload any) (protocol.ArtifactManifest, error) {
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
		CreatedAt: time.Now().UTC(),
	}
	if err := store.SaveArtifactManifest(sessionID, manifest); err != nil {
		return protocol.ArtifactManifest{}, err
	}
	return manifest, nil
}
