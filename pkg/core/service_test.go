package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzqDeco/papersilm/internal/agent"
	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/pipeline"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/internal/tools"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type testSink struct {
	events []protocol.StreamEvent
}

func (s *testSink) Emit(event protocol.StreamEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestPlanModeCreatesStructuredPlan(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf1 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper1.pdf"), "Paper One")
	pdf2 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper2.pdf"), "Paper Two")

	result, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "compare these papers",
		Sources:        []string{pdf1, pdf2},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}
	if result.Plan == nil {
		t.Fatalf("expected plan")
	}
	if result.Plan.TaskBoard == nil || result.Session.TaskBoard == nil {
		t.Fatalf("expected hydrated task board, plan=%+v session=%+v", result.Plan.TaskBoard, result.Session.TaskBoard)
	}
	if result.Session.Meta.State != protocol.SessionStatePlanned {
		t.Fatalf("unexpected state: %s", result.Session.Meta.State)
	}
	if len(result.Plan.DAG.Nodes) != 10 {
		t.Fatalf("expected 10 dag nodes, got %d", len(result.Plan.DAG.Nodes))
	}
	if !result.Plan.WillCompare {
		t.Fatalf("expected compare branch in dag")
	}
	readyNodes := 0
	for _, node := range result.Plan.DAG.Nodes {
		if node.Status == protocol.NodeStatusReady {
			readyNodes++
		}
	}
	if readyNodes == 0 {
		t.Fatalf("expected ready dag nodes")
	}
	if len(result.Plan.TaskBoard.Tasks) != len(result.Plan.DAG.Nodes) {
		t.Fatalf("expected task board to mirror dag nodes, got %d tasks for %d nodes", len(result.Plan.TaskBoard.Tasks), len(result.Plan.DAG.Nodes))
	}
	if len(result.Digests) != 0 || result.Comparison != nil {
		t.Fatalf("plan mode should not produce artifacts")
	}
}

func TestConfirmModeInterruptAndApproveResumes(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf1 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper1.pdf"), "Paper One")
	pdf2 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper2.pdf"), "Paper Two")

	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "compare these papers",
		Sources:        []string{pdf1, pdf2},
		PermissionMode: protocol.PermissionModeConfirm,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(confirm): %v", err)
	}
	if planned.Approval == nil {
		t.Fatalf("expected approval payload")
	}
	if len(planned.Approval.PendingNodeIDs) == 0 {
		t.Fatalf("expected pending node ids")
	}
	if planned.Session.Meta.State != protocol.SessionStateAwaitingApproval {
		t.Fatalf("unexpected state: %s", planned.Session.Meta.State)
	}
	resumed, err := svc.Approve(ctx, planned.Session.Meta.SessionID, true, "")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if resumed.Session.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("unexpected state after approve: %s", resumed.Session.Meta.State)
	}
	if len(resumed.Digests) != 2 {
		t.Fatalf("expected 2 digests, got %d: %+v", len(resumed.Digests), resumed.Digests)
	}
	if resumed.Comparison == nil {
		t.Fatalf("expected comparison digest, artifacts=%+v", resumed.Artifacts)
	}
}

func TestRunPlannedExecutesSavedPlan(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Solo")
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "distill this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}

	result, err := svc.RunPlanned(ctx, planned.Session.Meta.SessionID, "zh", "distill")
	if err != nil {
		t.Fatalf("RunPlanned: %v", err)
	}
	if result.Session.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("unexpected state: %s", result.Session.Meta.State)
	}
	if result.Session.Execution == nil || !result.Session.Execution.Finalized {
		t.Fatalf("expected finalized execution state")
	}
	if len(result.Digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(result.Digests))
	}
	if result.Comparison != nil {
		t.Fatalf("single paper run should not produce comparison")
	}
}

func TestPlanModeAddsMathWorkerForDetailRequests(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Math")
	result, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "explain the key equation and proof in this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}
	found := false
	for _, node := range result.Plan.DAG.Nodes {
		if node.Kind == protocol.NodeKindMathReasoner {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected math_reasoner node in dag")
	}
}

func TestPlanModeAddsWebWorkerForExternalRequests(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Web")
	result, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "summarize this paper and include latest external landscape",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}
	found := false
	for _, node := range result.Plan.DAG.Nodes {
		if node.Kind == protocol.NodeKindWebResearch {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected web_research node in dag")
	}
}

func TestAutoRunIncludesHydratedWorkspace(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Workspace")
	result, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "distill this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(auto): %v", err)
	}
	if len(result.Session.Workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %+v", result.Session.Workspaces)
	}
	workspace := result.Session.Workspaces[0]
	if workspace.Source == nil || workspace.Source.PaperID == "" {
		t.Fatalf("expected hydrated source, got %+v", workspace.Source)
	}
	if workspace.Digest == nil || workspace.Digest.PaperID != workspace.PaperID {
		t.Fatalf("expected hydrated digest, got %+v", workspace.Digest)
	}
	if !workspaceHasResource(workspace, pdf) {
		t.Fatalf("expected source resource in %+v", workspace.Resources)
	}
	if !workspaceHasResource(workspace, result.Artifacts[0].Paths["markdown"]) {
		t.Fatalf("expected artifact markdown resource in %+v", workspace.Resources)
	}
}

func TestRunTaskExecutesBlockedDependencyClosure(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Task Run")
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "distill this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}
	paperID := planned.Session.Sources[0].PaperID
	targetTaskID := "merge_digest_" + paperID

	result, err := svc.RunTask(ctx, planned.Session.Meta.SessionID, targetTaskID, "zh", "distill")
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	if result.Session.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("expected completed state, got %s", result.Session.Meta.State)
	}
	if len(result.Digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(result.Digests))
	}
	mergeTask, ok := findTaskByID(result.Session.TaskBoard, targetTaskID)
	if !ok || mergeTask.Status != protocol.TaskStatusCompleted {
		t.Fatalf("expected completed merge task, got %+v", mergeTask)
	}
	if statusCount(result.Session.TaskBoard, protocol.TaskStatusCompleted) != len(result.Session.TaskBoard.Tasks) {
		t.Fatalf("expected all tasks completed, got %+v", result.Session.TaskBoard.Tasks)
	}
}

func TestApproveTaskExecutesOnlySelectedPendingTask(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf1 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper1.pdf"), "Paper One")
	pdf2 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper2.pdf"), "Paper Two")

	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "compare these papers",
		Sources:        []string{pdf1, pdf2},
		PermissionMode: protocol.PermissionModeConfirm,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(confirm): %v", err)
	}
	if planned.Approval == nil || len(planned.Approval.PendingNodeIDs) < 2 {
		t.Fatalf("expected multi-node approval batch, got %+v", planned.Approval)
	}

	targetTaskID := planned.Approval.PendingNodeIDs[0]
	result, err := svc.ApproveTask(ctx, planned.Session.Meta.SessionID, targetTaskID, true, "")
	if err != nil {
		t.Fatalf("ApproveTask: %v", err)
	}
	if result.Session.Meta.State != protocol.SessionStateAwaitingApproval {
		t.Fatalf("expected session to remain awaiting approval, got %s", result.Session.Meta.State)
	}
	if result.Approval == nil || len(result.Approval.PendingNodeIDs) != len(planned.Approval.PendingNodeIDs)-1 {
		t.Fatalf("expected remaining pending approvals, got %+v", result.Approval)
	}
	task, ok := findTaskByID(result.Session.TaskBoard, targetTaskID)
	if !ok || task.Status != protocol.TaskStatusCompleted {
		t.Fatalf("expected approved task completed, got %+v", task)
	}
	for _, pendingID := range result.Approval.PendingNodeIDs {
		pendingTask, ok := findTaskByID(result.Session.TaskBoard, pendingID)
		if !ok || pendingTask.Status != protocol.TaskStatusAwaitingApproval {
			t.Fatalf("expected pending task to remain awaiting approval, got %+v", pendingTask)
		}
	}
}

func TestRunTaskRerunMarksDescendantsStale(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf1 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper1.pdf"), "Paper One")
	pdf2 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper2.pdf"), "Paper Two")

	initial, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "compare these papers",
		Sources:        []string{pdf1, pdf2},
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(auto): %v", err)
	}
	paperID := initial.Session.Sources[0].PaperID
	targetTaskID := "merge_digest_" + paperID

	result, err := svc.RunTask(ctx, initial.Session.Meta.SessionID, targetTaskID, "zh", "distill")
	if err != nil {
		t.Fatalf("RunTask(rerun): %v", err)
	}
	if result.Session.Meta.State != protocol.SessionStatePlanned {
		t.Fatalf("expected planned state after partial rerun, got %s", result.Session.Meta.State)
	}
	if result.Comparison != nil {
		t.Fatalf("expected comparison artifact to be cleared during rerun")
	}
	mergeTask, ok := findTaskByID(result.Session.TaskBoard, targetTaskID)
	if !ok || mergeTask.Status != protocol.TaskStatusCompleted {
		t.Fatalf("expected rerun target completed, got %+v", mergeTask)
	}
	for _, taskID := range []string{"method_compare", "experiment_compare", "results_compare", "final_synthesis"} {
		task, ok := findTaskByID(result.Session.TaskBoard, taskID)
		if !ok || task.Status != protocol.TaskStatusStale {
			t.Fatalf("expected descendant %s stale, got %+v", taskID, task)
		}
	}
}

func TestWorkspaceNotesAndAnnotationsSurviveReplan(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Notes")
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "distill this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}
	paperID := planned.Session.Sources[0].PaperID

	afterNote, err := svc.AddWorkspaceNote(planned.Session.Meta.SessionID, paperID, "Keep this note for later reasoning and review.")
	if err != nil {
		t.Fatalf("AddWorkspaceNote: %v", err)
	}
	workspace, ok := findWorkspaceByPaperID(afterNote.Workspaces, paperID)
	if !ok || len(workspace.Notes) != 1 {
		t.Fatalf("expected saved note, got %+v", afterNote.Workspaces)
	}
	if workspace.Notes[0].Title == "" {
		t.Fatalf("expected derived note title, got %+v", workspace.Notes[0])
	}

	afterAnnotation, err := svc.AddWorkspaceAnnotation(planned.Session.Meta.SessionID, paperID, protocol.AnchorRef{
		Kind: protocol.AnchorKindPage,
		Page: 3,
	}, "This page contains the key experimental setup.")
	if err != nil {
		t.Fatalf("AddWorkspaceAnnotation: %v", err)
	}
	workspace, ok = findWorkspaceByPaperID(afterAnnotation.Workspaces, paperID)
	if !ok || len(workspace.Annotations) != 1 {
		t.Fatalf("expected saved annotation, got %+v", afterAnnotation.Workspaces)
	}

	replanned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "distill this paper again",
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
		SessionID:      planned.Session.Meta.SessionID,
	})
	if err != nil {
		t.Fatalf("Execute(replan): %v", err)
	}
	workspace, ok = findWorkspaceByPaperID(replanned.Session.Workspaces, paperID)
	if !ok {
		t.Fatalf("expected workspace after replan, got %+v", replanned.Session.Workspaces)
	}
	if len(workspace.Notes) != 1 || len(workspace.Annotations) != 1 {
		t.Fatalf("expected workspace state to survive replan, got %+v", workspace)
	}
}

func TestAttachSourcesReplaceFailurePreservesExistingSession(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Replace Safety")
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "distill this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}
	paperID := planned.Session.Sources[0].PaperID
	afterNote, err := svc.AddWorkspaceNote(planned.Session.Meta.SessionID, paperID, "Preserve this note before replace.")
	if err != nil {
		t.Fatalf("AddWorkspaceNote: %v", err)
	}
	if _, ok := findWorkspaceByPaperID(afterNote.Workspaces, paperID); !ok {
		t.Fatalf("expected workspace before replace, got %+v", afterNote.Workspaces)
	}

	_, err = svc.AttachSources(ctx, planned.Session.Meta.SessionID, []string{filepath.Join(t.TempDir(), "missing.pdf")}, true)
	if err == nil {
		t.Fatalf("expected replace failure")
	}

	snapshot, err := svc.LoadSession(planned.Session.Meta.SessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if len(snapshot.Sources) != 1 || snapshot.Sources[0].PaperID != paperID {
		t.Fatalf("expected original source preserved, got %+v", snapshot.Sources)
	}
	if snapshot.Plan == nil || snapshot.Execution == nil {
		t.Fatalf("expected saved plan/execution preserved, got plan=%+v execution=%+v", snapshot.Plan, snapshot.Execution)
	}
	workspace, ok := findWorkspaceByPaperID(snapshot.Workspaces, paperID)
	if !ok || len(workspace.Notes) != 1 {
		t.Fatalf("expected workspace note preserved, got %+v", snapshot.Workspaces)
	}
}

func TestTaskBoardApprovalGateRestrictsActions(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf1 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper1.pdf"), "Paper One")
	pdf2 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper2.pdf"), "Paper Two")
	pdf3 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper3.pdf"), "Paper Three")

	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "compare these papers",
		Sources:        []string{pdf1, pdf2, pdf3},
		PermissionMode: protocol.PermissionModeConfirm,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(confirm): %v", err)
	}
	if planned.Approval == nil || len(planned.Approval.PendingNodeIDs) != 4 {
		t.Fatalf("expected capped pending batch of 4, got %+v", planned.Approval)
	}

	pendingSet := make(map[string]struct{}, len(planned.Approval.PendingNodeIDs))
	for _, pendingID := range planned.Approval.PendingNodeIDs {
		pendingSet[pendingID] = struct{}{}
		task, ok := findTaskByID(planned.Session.TaskBoard, pendingID)
		if !ok || task.Status != protocol.TaskStatusAwaitingApproval {
			t.Fatalf("expected pending task awaiting approval, got %+v", task)
		}
		if !taskHasActions(task, protocol.TaskActionInspect, protocol.TaskActionApprove, protocol.TaskActionReject) {
			t.Fatalf("expected approve/reject actions for pending task, got %+v", task.AvailableActions)
		}
	}

	readyCount := 0
	nonPendingReadyID := ""
	for _, task := range planned.Session.TaskBoard.Tasks {
		if task.Status != protocol.TaskStatusReady {
			continue
		}
		readyCount++
		if _, ok := pendingSet[task.TaskID]; ok {
			t.Fatalf("non-pending ready task should not stay in approval batch: %+v", task)
		}
		if nonPendingReadyID == "" {
			nonPendingReadyID = task.TaskID
		}
		if !taskHasActions(task, protocol.TaskActionInspect) {
			t.Fatalf("expected ready task outside approval batch to expose inspect only, got %+v", task.AvailableActions)
		}
	}
	if readyCount == 0 || nonPendingReadyID == "" {
		t.Fatalf("expected ready tasks outside first approval batch, got %+v", planned.Session.TaskBoard.Tasks)
	}

	_, err = svc.RunTask(ctx, planned.Session.Meta.SessionID, nonPendingReadyID, "zh", "distill")
	if err == nil || !strings.Contains(err.Error(), "only the current pending batch can be approved or rejected") {
		t.Fatalf("expected approval gate error for non-pending task run, got %v", err)
	}
}

func TestApproveTaskRejectsOnlySelectedRequiredTask(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf1 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper1.pdf"), "Paper One")
	pdf2 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper2.pdf"), "Paper Two")

	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "compare these papers",
		Sources:        []string{pdf1, pdf2},
		PermissionMode: protocol.PermissionModeConfirm,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(confirm): %v", err)
	}
	if planned.Approval == nil || len(planned.Approval.PendingNodeIDs) < 2 {
		t.Fatalf("expected multiple pending tasks, got %+v", planned.Approval)
	}

	targetTaskID := planned.Approval.PendingNodeIDs[0]
	result, err := svc.ApproveTask(ctx, planned.Session.Meta.SessionID, targetTaskID, false, "user rejected this task")
	if err != nil {
		t.Fatalf("ApproveTask(reject): %v", err)
	}
	if result.Session.Meta.State != protocol.SessionStateAwaitingApproval {
		t.Fatalf("expected session to remain awaiting approval, got %s", result.Session.Meta.State)
	}
	if result.Approval == nil || len(result.Approval.PendingNodeIDs) != len(planned.Approval.PendingNodeIDs)-1 {
		t.Fatalf("expected only one task removed from pending batch, got %+v", result.Approval)
	}
	if contains(result.Approval.PendingNodeIDs, targetTaskID) {
		t.Fatalf("rejected task should be removed from pending batch, got %+v", result.Approval.PendingNodeIDs)
	}
	task, ok := findTaskByID(result.Session.TaskBoard, targetTaskID)
	if !ok || task.Status != protocol.TaskStatusFailed {
		t.Fatalf("expected rejected required task failed, got %+v", task)
	}
	if !strings.Contains(task.Error, "rejected by user") {
		t.Fatalf("expected rejection reason on task, got %+v", task)
	}
	if len(task.PaperIDs) != 1 {
		t.Fatalf("expected single-paper task, got %+v", task)
	}
	mergeTask, ok := findTaskByID(result.Session.TaskBoard, "merge_digest_"+task.PaperIDs[0])
	if !ok || mergeTask.Status != protocol.TaskStatusBlocked {
		t.Fatalf("expected dependent merge task blocked after required reject, got %+v", mergeTask)
	}
}

func TestRejectTaskSkipsOptionalNodeAndPlanCanContinue(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Optional Reject")
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "explain the key equation in this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModeConfirm,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(confirm): %v", err)
	}
	paperID := planned.Session.Sources[0].PaperID
	targetTaskID := "math_reasoner_" + paperID

	current, err := svc.RejectTask(ctx, planned.Session.Meta.SessionID, targetTaskID, "skip optional math pass")
	if err != nil {
		t.Fatalf("RejectTask: %v", err)
	}
	task, ok := findTaskByID(current.Session.TaskBoard, targetTaskID)
	if !ok || task.Status != protocol.TaskStatusSkipped {
		t.Fatalf("expected optional task skipped, got %+v", task)
	}
	if current.Session.Meta.State != protocol.SessionStateAwaitingApproval {
		t.Fatalf("expected remaining required tasks still awaiting approval, got %s", current.Session.Meta.State)
	}

	for current.Session.Meta.State == protocol.SessionStateAwaitingApproval {
		if current.Approval == nil || len(current.Approval.PendingNodeIDs) == 0 {
			t.Fatalf("expected pending approval payload, got %+v", current.Approval)
		}
		nextTaskID := current.Approval.PendingNodeIDs[0]
		current, err = svc.ApproveTask(ctx, planned.Session.Meta.SessionID, nextTaskID, true, "")
		if err != nil {
			t.Fatalf("ApproveTask(%s): %v", nextTaskID, err)
		}
	}
	if current.Session.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("expected completed state after rejecting optional task and approving required ones, got %s", current.Session.Meta.State)
	}
	if len(current.Digests) != 1 {
		t.Fatalf("expected digest after optional reject path, got %+v", current.Digests)
	}
}

func TestRunTaskConfigMismatchPreservesPlan(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Task Config")
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "distill this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}
	paperID := planned.Session.Sources[0].PaperID
	if _, err := svc.AddWorkspaceNote(planned.Session.Meta.SessionID, paperID, "Preserve workspace state across config mismatch."); err != nil {
		t.Fatalf("AddWorkspaceNote: %v", err)
	}

	_, err = svc.RunTask(ctx, planned.Session.Meta.SessionID, "merge_digest_"+paperID, "en", "distill")
	if err == nil || !strings.Contains(err.Error(), "re-run /plan") {
		t.Fatalf("expected config mismatch error, got %v", err)
	}

	snapshot, err := svc.LoadSession(planned.Session.Meta.SessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if snapshot.Meta.Language != "zh" || snapshot.Meta.Style != "distill" {
		t.Fatalf("expected session config preserved, got lang=%s style=%s", snapshot.Meta.Language, snapshot.Meta.Style)
	}
	if snapshot.Plan == nil || snapshot.Execution == nil {
		t.Fatalf("expected saved plan/execution preserved, got plan=%+v execution=%+v", snapshot.Plan, snapshot.Execution)
	}
	workspace, ok := findWorkspaceByPaperID(snapshot.Workspaces, paperID)
	if !ok || len(workspace.Notes) != 1 {
		t.Fatalf("expected workspace preserved, got %+v", snapshot.Workspaces)
	}
}

func TestRunPlannedConfigMismatchPreservesPlan(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Planned Config")
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "distill this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}

	_, err = svc.RunPlanned(ctx, planned.Session.Meta.SessionID, "en", "distill")
	if err == nil || !strings.Contains(err.Error(), "re-run /plan") {
		t.Fatalf("expected config mismatch error, got %v", err)
	}

	snapshot, err := svc.LoadSession(planned.Session.Meta.SessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if snapshot.Meta.Language != "zh" || snapshot.Meta.Style != "distill" {
		t.Fatalf("expected session config preserved, got lang=%s style=%s", snapshot.Meta.Language, snapshot.Meta.Style)
	}
	if snapshot.Plan == nil || snapshot.Execution == nil {
		t.Fatalf("expected saved plan/execution preserved, got plan=%+v execution=%+v", snapshot.Plan, snapshot.Execution)
	}
}

func TestListSkillsReturnsBuiltinDescriptors(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	meta, err := svc.NewSession(protocol.PermissionModePlan, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	descriptors, err := svc.ListSkills(meta.SessionID)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(descriptors) != 4 {
		t.Fatalf("expected 4 skills, got %d", len(descriptors))
	}
	if descriptors[0].Name != protocol.SkillNameReviewer {
		t.Fatalf("expected reviewer first, got %+v", descriptors)
	}
}

func TestRunReviewerSkillDefaultTargetHydratesWorkspaceAndTaskBoard(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Reviewer Skill")
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "inspect this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}

	result, err := svc.RunSkill(ctx, planned.Session.Meta.SessionID, string(protocol.SkillNameReviewer), "")
	if err != nil {
		t.Fatalf("RunSkill(reviewer): %v", err)
	}
	if result.Run.TargetID == "" {
		t.Fatalf("expected resolved paper target, got %+v", result.Run)
	}
	if len(result.Session.SkillRuns) != 1 || len(result.Session.SkillArtifacts) != 1 {
		t.Fatalf("expected hydrated skill run and artifact, got runs=%d artifacts=%d", len(result.Session.SkillRuns), len(result.Session.SkillArtifacts))
	}
	workspace, ok := findWorkspaceByPaperID(result.Session.Workspaces, result.Run.TargetID)
	if !ok || len(workspace.SkillRuns) != 1 {
		t.Fatalf("expected workspace skill run hydration, got %+v", workspace)
	}
	task, ok := findTaskByID(result.Session.TaskBoard, result.Run.RunID)
	if !ok {
		t.Fatalf("expected skill task card in task board")
	}
	if task.Kind != protocol.NodeKindReviewerSkill {
		t.Fatalf("expected reviewer skill node kind, got %+v", task)
	}
	if !taskHasActions(task, protocol.TaskActionInspect) {
		t.Fatalf("expected inspect-only skill task actions, got %+v", task.AvailableActions)
	}
	if result.Artifact == nil {
		t.Fatalf("expected skill artifact manifest")
	}
	if _, err := os.Stat(result.Artifact.Paths["markdown"]); err != nil {
		t.Fatalf("expected skill markdown artifact: %v", err)
	}
}

func TestRunPaperSkillRequiresExplicitTargetInMultiPaperSession(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf1 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper1.pdf"), "Paper One")
	pdf2 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper2.pdf"), "Paper Two")
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "compare these papers",
		Sources:        []string{pdf1, pdf2},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}

	_, err = svc.RunSkill(ctx, planned.Session.Meta.SessionID, string(protocol.SkillNameEquationExplain), "")
	if err == nil || !strings.Contains(err.Error(), "requires a paper_id target") {
		t.Fatalf("expected explicit target error, got %v", err)
	}
}

func TestCompareRefinementRequiresExistingComparison(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf1 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper1.pdf"), "Paper One")
	pdf2 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper2.pdf"), "Paper Two")
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "compare these papers",
		Sources:        []string{pdf1, pdf2},
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}

	_, err = svc.RunSkill(ctx, planned.Session.Meta.SessionID, string(protocol.SkillNameCompareRefinement), "")
	if err == nil || !strings.Contains(err.Error(), "existing comparison") {
		t.Fatalf("expected missing comparison error, got %v", err)
	}
}

func TestRunAllSkillsProduceArtifacts(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf1 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper1.pdf"), "Paper One")
	pdf2 := writeTestPDF(t, filepath.Join(t.TempDir(), "paper2.pdf"), "Paper Two")
	initial, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "compare these papers",
		Sources:        []string{pdf1, pdf2},
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(auto): %v", err)
	}
	if initial.Comparison == nil {
		t.Fatalf("expected comparison digest before compare-refinement")
	}

	firstPaper := initial.Session.Workspaces[0].PaperID
	secondPaper := initial.Session.Workspaces[1].PaperID
	results := make([]protocol.SkillRunResult, 0, 4)
	for _, spec := range []struct {
		name   protocol.SkillName
		target string
	}{
		{name: protocol.SkillNameReviewer, target: firstPaper},
		{name: protocol.SkillNameEquationExplain, target: firstPaper},
		{name: protocol.SkillNameRelatedWorkMap, target: secondPaper},
		{name: protocol.SkillNameCompareRefinement, target: ""},
	} {
		result, err := svc.RunSkill(ctx, initial.Session.Meta.SessionID, string(spec.name), spec.target)
		if err != nil {
			t.Fatalf("RunSkill(%s): %v", spec.name, err)
		}
		if result.Artifact == nil {
			t.Fatalf("expected artifact for %s", spec.name)
		}
		if _, err := os.Stat(result.Artifact.Paths["markdown"]); err != nil {
			t.Fatalf("expected markdown artifact for %s: %v", spec.name, err)
		}
		results = append(results, result)
	}

	final := results[len(results)-1]
	if len(final.Session.SkillRuns) != 4 {
		t.Fatalf("expected 4 visible skill runs, got %d", len(final.Session.SkillRuns))
	}
	reviewerTask, ok := findTaskByID(final.Session.TaskBoard, results[0].Run.RunID)
	if !ok || reviewerTask.Kind != protocol.NodeKindReviewerSkill {
		t.Fatalf("expected reviewer skill task, got %+v", reviewerTask)
	}
	compareTask, ok := findTaskByID(final.Session.TaskBoard, results[3].Run.RunID)
	if !ok || compareTask.Kind != protocol.NodeKindCompareRefinement {
		t.Fatalf("expected compare refinement skill task, got %+v", compareTask)
	}
}

func TestStyleReviewerRemainsLegacyStyle(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()

	pdf := writeTestPDF(t, filepath.Join(t.TempDir(), "paper.pdf"), "Paper Legacy Reviewer")
	result, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "distill this paper",
		Sources:        []string{pdf},
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "reviewer",
	})
	if err != nil {
		t.Fatalf("Execute(auto reviewer style): %v", err)
	}
	if len(result.Session.SkillRuns) != 0 {
		t.Fatalf("expected legacy reviewer style to not auto-run skills, got %+v", result.Session.SkillRuns)
	}
}

func newTestService(t *testing.T) (*Service, *testSink) {
	t.Helper()

	cfg := config.Default()
	cfg.BaseDir = t.TempDir()
	store := storage.New(cfg.BaseDir)
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	sink := &testSink{}
	p := pipeline.New(cfg)
	registry := tools.New(p)
	ag := agent.New(registry, cfg)
	return New(store, ag, sink), sink
}

func writeTestPDF(t *testing.T, path, title string) string {
	t.Helper()

	var lines []string
	lines = append(lines, title)
	paragraph := "Abstract We study paper distillation for long academic documents. Method We propose a two stage analysis workflow that extracts problem setup, method details, experiment settings, key numeric results, conclusions, and limitations without spending tokens on general domain background. Experiments We evaluate on Dataset A and Dataset B with consistent prompts and report accuracy 91.2 percent, macro F1 88.3 percent, and latency reductions of 17 percent. Results Our approach outperforms the baseline on all reported metrics and remains stable across ablation settings. Conclusion The method is practical for CLI based paper review. "
	for i := 0; i < 18; i++ {
		lines = append(lines, fmt.Sprintf("%s Section %d.", paragraph, i+1))
	}

	raw := minimalPDF(lines)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func findWorkspaceByPaperID(workspaces []protocol.PaperWorkspace, paperID string) (protocol.PaperWorkspace, bool) {
	for _, workspace := range workspaces {
		if workspace.PaperID == paperID {
			return workspace, true
		}
	}
	return protocol.PaperWorkspace{}, false
}

func findTaskByID(board *protocol.TaskBoard, taskID string) (protocol.TaskCard, bool) {
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

func statusCount(board *protocol.TaskBoard, status protocol.TaskStatus) int {
	if board == nil {
		return 0
	}
	total := 0
	for _, task := range board.Tasks {
		if task.Status == status {
			total++
		}
	}
	return total
}

func workspaceHasResource(workspace protocol.PaperWorkspace, uri string) bool {
	for _, resource := range workspace.Resources {
		if resource.URI == uri {
			return true
		}
	}
	return false
}

func taskHasActions(task protocol.TaskCard, expected ...protocol.TaskActionType) bool {
	if len(task.AvailableActions) != len(expected) {
		return false
	}
	for i, action := range task.AvailableActions {
		if action.Type != expected[i] {
			return false
		}
	}
	return true
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func minimalPDF(lines []string) []byte {
	type obj struct {
		id   int
		body string
	}
	var content bytes.Buffer
	content.WriteString("BT\n/F1 12 Tf\n72 760 Td\n")
	for i, line := range lines {
		if i > 0 {
			content.WriteString("0 -18 Td\n")
		}
		content.WriteString("(")
		content.WriteString(escapePDFText(line))
		content.WriteString(") Tj\n")
	}
	content.WriteString("ET")

	objs := []obj{
		{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>"},
		{4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", content.Len(), content.String())},
		{5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"},
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1)
	for _, object := range objs {
		offsets[object.id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", object.id, object.body)
	}
	xrefStart := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objs)+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xrefStart)
	return buf.Bytes()
}

func escapePDFText(in string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`)
	return replacer.Replace(in)
}
