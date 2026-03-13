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
