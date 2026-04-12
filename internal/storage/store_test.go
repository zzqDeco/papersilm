package storage

import (
	"testing"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestStoreSnapshotRoundTrip(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	meta := protocol.SessionMeta{
		SessionID:      "sess_test",
		State:          protocol.SessionStateIdle,
		PermissionMode: protocol.PermissionModeConfirm,
		Language:       "zh",
		Style:          "distill",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := store.CreateSession(meta); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sources := []protocol.PaperRef{{PaperID: "paper_1", URI: "/tmp/paper.pdf", SourceType: protocol.SourceTypeLocalPDF}}
	if err := store.SaveSources(meta.SessionID, sources); err != nil {
		t.Fatalf("SaveSources: %v", err)
	}
	plan := protocol.PlanResult{Goal: "compare papers", ApprovalRequired: true, CreatedAt: time.Now().UTC()}
	if err := store.SavePlan(meta.SessionID, plan); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}

	snapshot, err := store.Snapshot(meta.SessionID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot.Meta.SessionID != meta.SessionID {
		t.Fatalf("unexpected session id: %+v", snapshot.Meta)
	}
	if len(snapshot.Sources) != 1 || snapshot.Plan == nil {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestLoadPlanUpgradesLegacyStepsToDAG(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	meta := protocol.SessionMeta{
		SessionID:      "sess_legacy",
		State:          protocol.SessionStateIdle,
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := store.CreateSession(meta); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	plan := protocol.PlanResult{
		Goal: "legacy plan",
		Steps: []protocol.PlanStep{
			{ID: "step_01", Tool: "distill_paper", PaperIDs: []string{"paper_1"}, Goal: "distill", ExpectedArtifact: "paper_1"},
			{ID: "step_02", Tool: "compare_papers", PaperIDs: []string{"paper_1", "paper_2"}, Goal: "compare", ExpectedArtifact: "comparison"},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := store.SavePlan(meta.SessionID, plan); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}

	loaded, err := store.LoadPlan(meta.SessionID)
	if err != nil {
		t.Fatalf("LoadPlan: %v", err)
	}
	if loaded == nil || len(loaded.DAG.Nodes) != 2 {
		t.Fatalf("expected legacy dag upgrade, got %+v", loaded)
	}
	if loaded.DAG.Nodes[0].Status != protocol.NodeStatusReady {
		t.Fatalf("expected first legacy node ready, got %+v", loaded.DAG.Nodes[0])
	}
}

func TestSnapshotHydratesWorkspaceWithoutPersistedState(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	now := time.Now().UTC()
	meta := protocol.SessionMeta{
		SessionID:      "sess_workspace",
		State:          protocol.SessionStateCompleted,
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := store.CreateSession(meta); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	source := protocol.PaperRef{
		PaperID:         "paper_1",
		URI:             "https://alphaxiv.org/overview/1706.03762",
		ResolvedPaperID: "1706.03762",
		SourceType:      protocol.SourceTypeAlphaXivOverview,
		Status:          protocol.SourceStatusInspected,
	}
	if err := store.SaveSources(meta.SessionID, []protocol.PaperRef{source}); err != nil {
		t.Fatalf("SaveSources: %v", err)
	}
	digest := protocol.PaperDigest{
		PaperID:     "paper_1",
		Title:       "Attention Is All You Need",
		GeneratedAt: now,
	}
	if err := store.SaveDigest(meta.SessionID, digest); err != nil {
		t.Fatalf("SaveDigest: %v", err)
	}
	manifest := protocol.ArtifactManifest{
		ArtifactID: "paper_1",
		SessionID:  meta.SessionID,
		Kind:       "paper_digest",
		Format:     protocol.ArtifactFormatMarkdown,
		Paths: map[string]string{
			"markdown": "/tmp/paper_1.md",
			"json":     "/tmp/paper_1.json",
		},
		CreatedAt: now,
	}
	if err := store.SaveArtifactManifest(meta.SessionID, manifest); err != nil {
		t.Fatalf("SaveArtifactManifest: %v", err)
	}

	snapshot, err := store.Snapshot(meta.SessionID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snapshot.Workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(snapshot.Workspaces))
	}
	workspace := snapshot.Workspaces[0]
	if workspace.PaperID != source.PaperID {
		t.Fatalf("unexpected workspace: %+v", workspace)
	}
	if workspace.Source == nil || workspace.Source.PaperID != source.PaperID {
		t.Fatalf("expected hydrated source, got %+v", workspace.Source)
	}
	if workspace.Digest == nil || workspace.Digest.PaperID != digest.PaperID {
		t.Fatalf("expected hydrated digest, got %+v", workspace.Digest)
	}
	for _, want := range []string{
		"https://alphaxiv.org/overview/1706.03762",
		"https://arxiv.org/abs/1706.03762",
		"https://arxiv.org/pdf/1706.03762.pdf",
		"https://alphaxiv.org/abs/1706.03762",
		"/tmp/paper_1.md",
		"/tmp/paper_1.json",
	} {
		if !workspaceHasResource(workspace, want) {
			t.Fatalf("expected resource %q in %+v", want, workspace.Resources)
		}
	}
}

func TestInvalidatePlanStateKeepsWorkspaceState(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	now := time.Now().UTC()
	meta := protocol.SessionMeta{
		SessionID:      "sess_keep_workspace",
		State:          protocol.SessionStateCompleted,
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := store.CreateSession(meta); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := store.SaveSources(meta.SessionID, []protocol.PaperRef{{
		PaperID:    "paper_1",
		URI:        "/tmp/paper.pdf",
		LocalPath:  "/tmp/paper.pdf",
		SourceType: protocol.SourceTypeLocalPDF,
		Status:     protocol.SourceStatusAttached,
	}}); err != nil {
		t.Fatalf("SaveSources: %v", err)
	}
	if err := store.SavePlan(meta.SessionID, protocol.PlanResult{
		PlanID:    "plan_1",
		Goal:      "distill paper",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := store.SaveDigest(meta.SessionID, protocol.PaperDigest{
		PaperID:     "paper_1",
		Title:       "Paper One",
		GeneratedAt: now,
	}); err != nil {
		t.Fatalf("SaveDigest: %v", err)
	}
	if err := store.SaveArtifactManifest(meta.SessionID, protocol.ArtifactManifest{
		ArtifactID: "paper_1",
		SessionID:  meta.SessionID,
		Kind:       "paper_digest",
		Paths: map[string]string{
			"markdown": "/tmp/paper_1.md",
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("SaveArtifactManifest: %v", err)
	}
	if err := store.SaveWorkspaceState(meta.SessionID, protocol.PaperWorkspace{
		PaperID: "paper_1",
		Notes: []protocol.PaperNote{{
			ID:        "note_1",
			Title:     "Important note",
			Body:      "keep this note",
			CreatedAt: now,
			UpdatedAt: now,
		}},
		Annotations: []protocol.PaperAnnotation{{
			ID:    "ann_1",
			Title: "Page note",
			Body:  "page annotation",
			Anchor: protocol.AnchorRef{
				Kind: protocol.AnchorKindPage,
				Page: 2,
			},
			CreatedAt: now,
			UpdatedAt: now,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveWorkspaceState: %v", err)
	}

	if err := store.InvalidatePlanState(meta.SessionID); err != nil {
		t.Fatalf("InvalidatePlanState: %v", err)
	}

	snapshot, err := store.Snapshot(meta.SessionID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot.Plan != nil || snapshot.Execution != nil {
		t.Fatalf("expected plan state to be cleared, got %+v", snapshot)
	}
	if len(snapshot.Digests) != 0 || len(snapshot.Artifacts) != 0 {
		t.Fatalf("expected digests/artifacts cleared, got %+v", snapshot)
	}
	if len(snapshot.Workspaces) != 1 {
		t.Fatalf("expected workspace to remain, got %+v", snapshot.Workspaces)
	}
	if len(snapshot.Workspaces[0].Notes) != 1 || len(snapshot.Workspaces[0].Annotations) != 1 {
		t.Fatalf("expected persisted workspace state, got %+v", snapshot.Workspaces[0])
	}
}

func workspaceHasResource(workspace protocol.PaperWorkspace, uri string) bool {
	for _, resource := range workspace.Resources {
		if resource.URI == uri {
			return true
		}
	}
	return false
}
