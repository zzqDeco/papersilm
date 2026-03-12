package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	toolutils "github.com/cloudwego/eino/components/tool/utils"

	"papersilm/internal/pipeline"
	"papersilm/internal/storage"
	"papersilm/pkg/protocol"
)

type Registry struct {
	pipeline *pipeline.Service
}

func New(p *pipeline.Service) *Registry {
	return &Registry{pipeline: p}
}

func (r *Registry) AttachSources(ctx context.Context, store *storage.Store, sessionID string, raw []string) ([]protocol.PaperRef, error) {
	refs, err := r.pipeline.NormalizeSources(ctx, sessionID, raw)
	if err != nil {
		return nil, err
	}
	if err := store.SaveSources(sessionID, refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func (r *Registry) InspectAndPlan(ctx context.Context, store *storage.Store, sessionID, task string, refs []protocol.PaperRef) (protocol.PlanResult, error) {
	inspected := make([]protocol.PaperRef, 0, len(refs))
	for _, ref := range refs {
		newRef, _, err := r.pipeline.InspectSource(ctx, sessionID, ref)
		if err != nil {
			inspected = append(inspected, newRef)
			continue
		}
		inspected = append(inspected, newRef)
	}
	if err := store.SaveSources(sessionID, inspected); err != nil {
		return protocol.PlanResult{}, err
	}
	return r.pipeline.BuildPlan(task, inspected), nil
}

func (r *Registry) Run(ctx context.Context, store *storage.Store, sessionID string, refs []protocol.PaperRef, lang, style string) ([]protocol.PaperDigest, *protocol.ComparisonDigest, []protocol.ArtifactManifest, error) {
	digests := make([]protocol.PaperDigest, 0, len(refs))
	for _, ref := range refs {
		digest, err := r.pipeline.Distill(ctx, sessionID, ref, lang, style)
		if err != nil {
			return nil, nil, nil, err
		}
		if err := store.SaveDigest(sessionID, digest); err != nil {
			return nil, nil, nil, err
		}
		digests = append(digests, digest)
	}
	var cmp *protocol.ComparisonDigest
	if len(digests) > 1 {
		comparison := r.pipeline.Compare("跨论文综合对比", digests, lang, style)
		if err := store.SaveComparison(sessionID, comparison); err != nil {
			return nil, nil, nil, err
		}
		cmp = &comparison
	}
	artifacts, err := r.writeArtifacts(store, sessionID, digests, cmp)
	if err != nil {
		return nil, nil, nil, err
	}
	return digests, cmp, artifacts, nil
}

func (r *Registry) writeArtifacts(store *storage.Store, sessionID string, digests []protocol.PaperDigest, cmp *protocol.ComparisonDigest) ([]protocol.ArtifactManifest, error) {
	out := make([]protocol.ArtifactManifest, 0, len(digests)+2)
	for _, digest := range digests {
		manifest, err := writeArtifact(store, sessionID, digest.PaperID, "paper_digest", digest.Title, digest.Markdown, digest)
		if err != nil {
			return nil, err
		}
		out = append(out, manifest)
	}
	if cmp != nil {
		manifest, err := writeArtifact(store, sessionID, "comparison", "comparison_digest", "comparison", cmp.Markdown, cmp)
		if err != nil {
			return nil, err
		}
		out = append(out, manifest)
	}
	return out, nil
}

func writeArtifact(store *storage.Store, sessionID, artifactID, kind, source, markdown string, payload interface{}) (protocol.ArtifactManifest, error) {
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
		Language:   "zh",
		Format:     protocol.ArtifactFormatJSON,
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

type attachSourcesInput struct {
	Sources []string `json:"sources"`
}

func (r *Registry) Toolset(store *storage.Store, sessionID string) ([]string, error) {
	// We expose names for CLI / plan output today.
	attachTool, err := toolutils.InferTool("attach_sources", "Attach paper sources to current session", func(ctx context.Context, input attachSourcesInput) ([]protocol.PaperRef, error) {
		return r.AttachSources(ctx, store, sessionID, input.Sources)
	})
	if err != nil {
		return nil, err
	}
	info, err := attachTool.Info(context.Background())
	if err != nil {
		return nil, err
	}
	return []string{info.Name, "inspect_sources", "distill_paper", "compare_papers", "export_artifact", "list_session_assets"}, nil
}
