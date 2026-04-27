package storage

import (
	"os"
	"path/filepath"
	"reflect"
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
	if snapshot.TaskBoard != nil {
		t.Fatalf("did not expect task board without a dag-backed plan, got %+v", snapshot.TaskBoard)
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

func TestSnapshotHydratesTaskBoardFromPlanAndExecution(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	now := time.Now().UTC()
	meta := protocol.SessionMeta{
		SessionID:      "sess_task_board",
		State:          protocol.SessionStatePlanned,
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := store.CreateSession(meta); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	plan := protocol.PlanResult{
		PlanID: "plan_1",
		Goal:   "distill paper",
		DAG: protocol.PlanDAG{
			Nodes: []protocol.PlanNode{
				{ID: "paper_summary_p1", Kind: protocol.NodeKindPaperSummary, Goal: "summary", PaperIDs: []string{"p1"}, Status: protocol.NodeStatusCompleted},
				{ID: "merge_digest_p1", Kind: protocol.NodeKindMergeDigest, Goal: "merge", PaperIDs: []string{"p1"}, DependsOn: []string{"paper_summary_p1"}, Produces: []string{"p1"}, Status: protocol.NodeStatusPending},
			},
			Edges: []protocol.PlanEdge{{From: "paper_summary_p1", To: "merge_digest_p1"}},
		},
		CreatedAt: now,
	}
	exec := protocol.ExecutionState{
		PlanID:       "plan_1",
		StaleNodeIDs: []string{"merge_digest_p1"},
		Nodes: []protocol.NodeExecutionState{
			{NodeID: "paper_summary_p1", Status: protocol.NodeStatusCompleted},
			{NodeID: "merge_digest_p1", Status: protocol.NodeStatusPending},
		},
		UpdatedAt: now,
	}
	if err := store.SavePlan(meta.SessionID, plan); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := store.SaveExecutionState(meta.SessionID, exec); err != nil {
		t.Fatalf("SaveExecutionState: %v", err)
	}

	snapshot, err := store.Snapshot(meta.SessionID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot.TaskBoard == nil || len(snapshot.TaskBoard.Tasks) != 2 {
		t.Fatalf("expected hydrated task board, got %+v", snapshot.TaskBoard)
	}
	task, ok := findTask(snapshot.TaskBoard, "merge_digest_p1")
	if !ok || task.Status != protocol.TaskStatusStale {
		t.Fatalf("expected stale task projection, got %+v", task)
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

func TestSnapshotHydratesSkillRunsAndArtifacts(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	now := time.Now().UTC()
	meta := protocol.SessionMeta{
		SessionID:      "sess_skills",
		State:          protocol.SessionStatePlanned,
		PermissionMode: protocol.PermissionModePlan,
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
	run := protocol.SkillRunRecord{
		RunID:      "skill_1",
		SessionID:  meta.SessionID,
		SkillName:  protocol.SkillNameReviewer,
		TargetKind: protocol.SkillTargetKindPaper,
		TargetID:   "paper_1",
		ArtifactID: "skill_1",
		Status:     protocol.SkillRunStatusCompleted,
		Title:      "Reviewer: paper_1",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := store.SaveSkillRun(meta.SessionID, run); err != nil {
		t.Fatalf("SaveSkillRun: %v", err)
	}
	if err := store.SaveSkillArtifactManifest(meta.SessionID, protocol.ArtifactManifest{
		ArtifactID: "skill_1",
		SessionID:  meta.SessionID,
		Kind:       "reviewer_skill",
		Paths: map[string]string{
			"markdown": "/tmp/skill_1.md",
			"json":     "/tmp/skill_1.json",
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("SaveSkillArtifactManifest: %v", err)
	}

	snapshot, err := store.Snapshot(meta.SessionID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snapshot.SkillRuns) != 1 || len(snapshot.SkillArtifacts) != 1 {
		t.Fatalf("expected hydrated skill assets, got runs=%d artifacts=%d", len(snapshot.SkillRuns), len(snapshot.SkillArtifacts))
	}
	if len(snapshot.Workspaces) != 1 || len(snapshot.Workspaces[0].SkillRuns) != 1 {
		t.Fatalf("expected workspace skill run hydration, got %+v", snapshot.Workspaces)
	}
}

func TestInvalidatePlanStateKeepsSkillRunsAndArtifacts(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	now := time.Now().UTC()
	meta := protocol.SessionMeta{
		SessionID:      "sess_keep_skills",
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
	if err := store.SavePlan(meta.SessionID, protocol.PlanResult{PlanID: "plan_1", Goal: "distill", CreatedAt: now}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := store.SaveSkillRun(meta.SessionID, protocol.SkillRunRecord{
		RunID:      "skill_1",
		SessionID:  meta.SessionID,
		SkillName:  protocol.SkillNameReviewer,
		TargetKind: protocol.SkillTargetKindPaper,
		TargetID:   "paper_1",
		ArtifactID: "skill_1",
		Status:     protocol.SkillRunStatusCompleted,
		Title:      "Reviewer: paper_1",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveSkillRun: %v", err)
	}
	if err := store.SaveSkillArtifactManifest(meta.SessionID, protocol.ArtifactManifest{
		ArtifactID: "skill_1",
		SessionID:  meta.SessionID,
		Kind:       "reviewer_skill",
		Paths:      map[string]string{"markdown": "/tmp/skill_1.md"},
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveSkillArtifactManifest: %v", err)
	}

	if err := store.InvalidatePlanState(meta.SessionID); err != nil {
		t.Fatalf("InvalidatePlanState: %v", err)
	}

	runs, err := store.LoadSkillRuns(meta.SessionID)
	if err != nil {
		t.Fatalf("LoadSkillRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected skill runs to persist, got %+v", runs)
	}
	artifacts, err := store.LoadSkillArtifactManifests(meta.SessionID)
	if err != nil {
		t.Fatalf("LoadSkillArtifactManifests: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected skill artifacts to persist, got %+v", artifacts)
	}
}

func TestSnapshotBuildsSkillsOnlyTaskBoard(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	now := time.Now().UTC()
	meta := protocol.SessionMeta{
		SessionID:      "sess_skills_only",
		State:          protocol.SessionStateCompleted,
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
		LastTask:       "skill review",
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
	if err := store.SaveSkillRun(meta.SessionID, protocol.SkillRunRecord{
		RunID:      "skill_1",
		SessionID:  meta.SessionID,
		SkillName:  protocol.SkillNameReviewer,
		TargetKind: protocol.SkillTargetKindPaper,
		TargetID:   "paper_1",
		ArtifactID: "skill_1",
		Status:     protocol.SkillRunStatusCompleted,
		Title:      "Reviewer: paper_1",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveSkillRun: %v", err)
	}

	snapshot, err := store.Snapshot(meta.SessionID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot.TaskBoard == nil || len(snapshot.TaskBoard.Tasks) != 1 {
		t.Fatalf("expected skills-only task board, got %+v", snapshot.TaskBoard)
	}
	task, ok := findTask(snapshot.TaskBoard, "skill_1")
	if !ok || task.Kind != protocol.NodeKindReviewerSkill {
		t.Fatalf("expected reviewer skill task, got %+v", task)
	}
	if len(task.AvailableActions) != 1 || task.AvailableActions[0].Type != protocol.TaskActionInspect {
		t.Fatalf("expected inspect-only task, got %+v", task.AvailableActions)
	}
}

func TestSnapshotHidesPaperSkillRunsWhenSourceRemoved(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	now := time.Now().UTC()
	meta := protocol.SessionMeta{
		SessionID:      "sess_hide_skills",
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
	if err := store.SaveSkillRun(meta.SessionID, protocol.SkillRunRecord{
		RunID:      "skill_1",
		SessionID:  meta.SessionID,
		SkillName:  protocol.SkillNameReviewer,
		TargetKind: protocol.SkillTargetKindPaper,
		TargetID:   "paper_1",
		ArtifactID: "skill_1",
		Status:     protocol.SkillRunStatusCompleted,
		Title:      "Reviewer: paper_1",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveSkillRun: %v", err)
	}
	if err := store.SaveSkillArtifactManifest(meta.SessionID, protocol.ArtifactManifest{
		ArtifactID: "skill_1",
		SessionID:  meta.SessionID,
		Kind:       "reviewer_skill",
		Paths:      map[string]string{"markdown": "/tmp/skill_1.md"},
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveSkillArtifactManifest: %v", err)
	}

	if err := store.SaveSources(meta.SessionID, nil); err != nil {
		t.Fatalf("SaveSources(clear): %v", err)
	}

	snapshot, err := store.Snapshot(meta.SessionID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snapshot.SkillRuns) != 0 || len(snapshot.SkillArtifacts) != 0 {
		t.Fatalf("expected hidden paper skill assets after source removal, got runs=%d artifacts=%d", len(snapshot.SkillRuns), len(snapshot.SkillArtifacts))
	}
	if len(snapshot.Workspaces) != 0 {
		t.Fatalf("expected no workspaces after source removal, got %+v", snapshot.Workspaces)
	}
}

func TestSnapshotKeepsComparisonSkillRunsVisibleAfterPlanInvalidation(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	now := time.Now().UTC()
	meta := protocol.SessionMeta{
		SessionID:      "sess_cmp_visible",
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
	sources := []protocol.PaperRef{
		{PaperID: "paper_1", URI: "/tmp/paper1.pdf", LocalPath: "/tmp/paper1.pdf", SourceType: protocol.SourceTypeLocalPDF, Status: protocol.SourceStatusAttached},
		{PaperID: "paper_2", URI: "/tmp/paper2.pdf", LocalPath: "/tmp/paper2.pdf", SourceType: protocol.SourceTypeLocalPDF, Status: protocol.SourceStatusAttached},
	}
	if err := store.SaveSources(meta.SessionID, sources); err != nil {
		t.Fatalf("SaveSources: %v", err)
	}
	if err := store.SavePlan(meta.SessionID, protocol.PlanResult{PlanID: "plan_cmp", Goal: "compare", CreatedAt: now}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := store.SaveComparison(meta.SessionID, protocol.ComparisonDigest{
		PaperIDs:    []string{"paper_1", "paper_2"},
		Goal:        "compare",
		Language:    "zh",
		Style:       "distill",
		GeneratedAt: now,
	}); err != nil {
		t.Fatalf("SaveComparison: %v", err)
	}
	if err := store.SaveSkillRun(meta.SessionID, protocol.SkillRunRecord{
		RunID:      "skill_cmp",
		SessionID:  meta.SessionID,
		SkillName:  protocol.SkillNameCompareRefinement,
		TargetKind: protocol.SkillTargetKindComparison,
		TargetID:   "comparison",
		PaperIDs:   []string{"paper_1", "paper_2"},
		ArtifactID: "skill_cmp",
		Status:     protocol.SkillRunStatusCompleted,
		Title:      "对比精炼: 对比",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveSkillRun: %v", err)
	}
	if err := store.SaveSkillArtifactManifest(meta.SessionID, protocol.ArtifactManifest{
		ArtifactID: "skill_cmp",
		SessionID:  meta.SessionID,
		Kind:       "compare_refinement_skill",
		Paths:      map[string]string{"markdown": "/tmp/skill_cmp.md"},
		Metadata:   map[string]interface{}{"paper_ids": []string{"paper_1", "paper_2"}},
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveSkillArtifactManifest: %v", err)
	}

	if err := store.InvalidatePlanState(meta.SessionID); err != nil {
		t.Fatalf("InvalidatePlanState: %v", err)
	}

	snapshot, err := store.Snapshot(meta.SessionID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snapshot.SkillRuns) != 1 || len(snapshot.SkillArtifacts) != 1 {
		t.Fatalf("expected comparison skill assets to stay visible, got runs=%d artifacts=%d", len(snapshot.SkillRuns), len(snapshot.SkillArtifacts))
	}
	if !reflect.DeepEqual(snapshot.SkillRuns[0].PaperIDs, []string{"paper_1", "paper_2"}) {
		t.Fatalf("expected paper_ids to survive hydration, got %+v", snapshot.SkillRuns[0].PaperIDs)
	}
	if snapshot.TaskBoard == nil {
		t.Fatalf("expected task board after invalidation when comparison skill run remains visible")
	}
	task, ok := findTask(snapshot.TaskBoard, "skill_cmp")
	if !ok || task.Kind != protocol.NodeKindCompareRefinement {
		t.Fatalf("expected compare refinement task in task board, got %+v", task)
	}
}

func TestSnapshotHidesComparisonSkillRunsWhenPaperSetChanges(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	now := time.Now().UTC()
	meta := protocol.SessionMeta{
		SessionID:      "sess_cmp_hidden",
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
	if err := store.SaveSources(meta.SessionID, []protocol.PaperRef{
		{PaperID: "paper_1", URI: "/tmp/paper1.pdf", LocalPath: "/tmp/paper1.pdf", SourceType: protocol.SourceTypeLocalPDF, Status: protocol.SourceStatusAttached},
		{PaperID: "paper_2", URI: "/tmp/paper2.pdf", LocalPath: "/tmp/paper2.pdf", SourceType: protocol.SourceTypeLocalPDF, Status: protocol.SourceStatusAttached},
	}); err != nil {
		t.Fatalf("SaveSources: %v", err)
	}
	if err := store.SaveSkillRun(meta.SessionID, protocol.SkillRunRecord{
		RunID:      "skill_cmp",
		SessionID:  meta.SessionID,
		SkillName:  protocol.SkillNameCompareRefinement,
		TargetKind: protocol.SkillTargetKindComparison,
		TargetID:   "comparison",
		PaperIDs:   []string{"paper_1", "paper_2"},
		ArtifactID: "skill_cmp",
		Status:     protocol.SkillRunStatusCompleted,
		Title:      "对比精炼: 对比",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveSkillRun: %v", err)
	}
	if err := store.SaveSkillArtifactManifest(meta.SessionID, protocol.ArtifactManifest{
		ArtifactID: "skill_cmp",
		SessionID:  meta.SessionID,
		Kind:       "compare_refinement_skill",
		Paths:      map[string]string{"markdown": "/tmp/skill_cmp.md"},
		Metadata:   map[string]interface{}{"paper_ids": []string{"paper_1", "paper_2"}},
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveSkillArtifactManifest: %v", err)
	}

	if err := store.SaveSources(meta.SessionID, []protocol.PaperRef{
		{PaperID: "paper_1", URI: "/tmp/paper1.pdf", LocalPath: "/tmp/paper1.pdf", SourceType: protocol.SourceTypeLocalPDF, Status: protocol.SourceStatusAttached},
	}); err != nil {
		t.Fatalf("SaveSources(update): %v", err)
	}

	snapshot, err := store.Snapshot(meta.SessionID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snapshot.SkillRuns) != 0 || len(snapshot.SkillArtifacts) != 0 {
		t.Fatalf("expected comparison skill assets to hide after source-set change, got runs=%d artifacts=%d", len(snapshot.SkillRuns), len(snapshot.SkillArtifacts))
	}
}

func TestSnapshotHydratesLegacyComparisonSkillPaperIDsFromArtifactJSON(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	now := time.Now().UTC()
	meta := protocol.SessionMeta{
		SessionID:      "sess_cmp_legacy",
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
	if err := store.SaveSources(meta.SessionID, []protocol.PaperRef{
		{PaperID: "paper_1", URI: "/tmp/paper1.pdf", LocalPath: "/tmp/paper1.pdf", SourceType: protocol.SourceTypeLocalPDF, Status: protocol.SourceStatusAttached},
		{PaperID: "paper_2", URI: "/tmp/paper2.pdf", LocalPath: "/tmp/paper2.pdf", SourceType: protocol.SourceTypeLocalPDF, Status: protocol.SourceStatusAttached},
	}); err != nil {
		t.Fatalf("SaveSources: %v", err)
	}
	if err := store.SaveSkillRun(meta.SessionID, protocol.SkillRunRecord{
		RunID:      "skill_cmp",
		SessionID:  meta.SessionID,
		SkillName:  protocol.SkillNameCompareRefinement,
		TargetKind: protocol.SkillTargetKindComparison,
		TargetID:   "comparison",
		ArtifactID: "skill_cmp",
		Status:     protocol.SkillRunStatusCompleted,
		Title:      "对比精炼: 对比",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveSkillRun: %v", err)
	}

	jsonPath := filepath.Join(store.skillArtifactsDir(meta.SessionID), "skill_cmp.json")
	if err := os.WriteFile(jsonPath, []byte("{\n  \"paper_ids\": [\"paper_1\", \"paper_2\"]\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(skill json): %v", err)
	}
	if err := store.SaveSkillArtifactManifest(meta.SessionID, protocol.ArtifactManifest{
		ArtifactID: "skill_cmp",
		SessionID:  meta.SessionID,
		Kind:       "compare_refinement_skill",
		Paths: map[string]string{
			"markdown": filepath.Join(store.skillArtifactsDir(meta.SessionID), "skill_cmp.md"),
			"json":     jsonPath,
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("SaveSkillArtifactManifest: %v", err)
	}

	snapshot, err := store.Snapshot(meta.SessionID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snapshot.SkillRuns) != 1 {
		t.Fatalf("expected legacy comparison skill run to remain visible, got %+v", snapshot.SkillRuns)
	}
	if !reflect.DeepEqual(snapshot.SkillRuns[0].PaperIDs, []string{"paper_1", "paper_2"}) {
		t.Fatalf("expected hydrated paper_ids from artifact json, got %+v", snapshot.SkillRuns[0].PaperIDs)
	}
}

func TestLoadRecentEventsReturnsNewestValidEvents(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	meta := protocol.SessionMeta{
		SessionID:      "sess_events",
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
	for _, eventType := range []protocol.StreamEventType{
		protocol.EventInit,
		protocol.EventPlan,
		protocol.EventResult,
	} {
		if err := store.AppendEvent(meta.SessionID, protocol.StreamEvent{
			Type:      eventType,
			SessionID: meta.SessionID,
			Message:   string(eventType),
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("AppendEvent(%s): %v", eventType, err)
		}
	}
	f, err := os.OpenFile(store.eventsPath(meta.SessionID), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(events): %v", err)
	}
	if _, err := f.WriteString("{broken json}\n"); err != nil {
		t.Fatalf("WriteString(bad line): %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close(events): %v", err)
	}

	events, err := store.LoadRecentEvents(meta.SessionID, 2)
	if err != nil {
		t.Fatalf("LoadRecentEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 recent events, got %+v", events)
	}
	if events[0].Type != protocol.EventPlan || events[1].Type != protocol.EventResult {
		t.Fatalf("expected newest valid events, got %+v", events)
	}
}

func TestLoadTranscriptSkipsBrokenLines(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	meta := protocol.SessionMeta{
		SessionID:      "sess_transcript",
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
	for _, entry := range []protocol.TranscriptEntry{
		{
			ID:        "msg_1",
			SessionID: meta.SessionID,
			Type:      protocol.TranscriptEntryUser,
			Body:      "hello",
			InputMode: protocol.TranscriptInputPrompt,
			CreatedAt: time.Now().UTC(),
		},
		{
			ID:        "msg_2",
			SessionID: meta.SessionID,
			Type:      protocol.TranscriptEntryAssistant,
			Body:      "world",
			Markdown:  true,
			CreatedAt: time.Now().UTC(),
		},
	} {
		if err := store.AppendTranscriptEntry(meta.SessionID, entry); err != nil {
			t.Fatalf("AppendTranscriptEntry(%s): %v", entry.ID, err)
		}
	}
	f, err := os.OpenFile(store.transcriptPath(meta.SessionID), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(transcript): %v", err)
	}
	if _, err := f.WriteString("{broken json}\n"); err != nil {
		t.Fatalf("WriteString(bad line): %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close(transcript): %v", err)
	}

	entries, err := store.LoadTranscript(meta.SessionID)
	if err != nil {
		t.Fatalf("LoadTranscript: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 transcript entries, got %+v", entries)
	}
	if entries[0].ID != "msg_1" || entries[1].ID != "msg_2" {
		t.Fatalf("expected valid transcript order, got %+v", entries)
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

func findTask(board *protocol.TaskBoard, taskID string) (protocol.TaskCard, bool) {
	if board == nil {
		return protocol.TaskCard{}, false
	}
	for _, task := range board.Tasks {
		if task.TaskID == taskID {
			return task, true
		}
	}
	return protocol.TaskCard{}, false
}
