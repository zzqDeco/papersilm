package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/pipeline"
	runtimeinput "github.com/zzqDeco/papersilm/internal/runtime/input"
	"github.com/zzqDeco/papersilm/internal/runtime/turnloop"
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

type failingResultSink struct{}

func (s failingResultSink) Emit(event protocol.StreamEvent) error {
	if event.Type == protocol.EventResult {
		return errors.New("sink result failure")
	}
	return nil
}

func TestExecutePlanCreatesWorkspacePlanWithoutSources(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	result, err := svc.Execute(context.Background(), protocol.ClientRequest{
		Task:           "总结当前工作区",
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(plan): %v", err)
	}
	if result.Plan == nil || len(result.Plan.DAG.Nodes) != 1 {
		t.Fatalf("expected workspace plan, got %+v", result.Plan)
	}
	if result.Plan.DAG.Nodes[0].Kind != protocol.NodeKindWorkspaceInspect {
		t.Fatalf("expected workspace inspect node, got %+v", result.Plan.DAG.Nodes[0])
	}
	if len(result.Session.Sources) != 0 {
		t.Fatalf("workspace-first task should not require sources, got %+v", result.Session.Sources)
	}
}

func TestWorkspaceTaskRunsWithoutAttachedSources(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	readmePath := filepath.Join(svc.store.WorkspaceRoot(), "README.md")
	if err := os.WriteFile(readmePath, []byte("# papersilm\n\nworkspace first agent\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", readmePath, err)
	}
	if err := svc.store.RefreshWorkspaceState(); err != nil {
		t.Fatalf("RefreshWorkspaceState: %v", err)
	}
	result, err := svc.Execute(context.Background(), protocol.ClientRequest{
		Task:           "总结当前工作区的结构和关键文件",
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(workspace): %v", err)
	}
	if strings.TrimSpace(result.Response) == "" {
		t.Fatalf("expected workspace response, got %+v", result)
	}
	if result.Session.Workspace == nil || result.Session.Workspace.FileCount == 0 {
		t.Fatalf("expected hydrated workspace summary, got %+v", result.Session.Workspace)
	}
}

func TestConfirmCommandCreatesToolScopedPermissionAndAcceptRuns(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	ctx := context.Background()
	planned, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           "run command `printf %s command-smoke`",
		PermissionMode: protocol.PermissionModeConfirm,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(confirm): %v", err)
	}
	if planned.Approval == nil || len(planned.Approval.Requests) != 1 {
		t.Fatalf("expected one permission request, got %+v", planned.Approval)
	}
	request := planned.Approval.Requests[0]
	if request.Tool != string(protocol.NodeKindWorkspaceCommand) || request.Command != "printf %s command-smoke" {
		t.Fatalf("unexpected command request: %+v", request)
	}
	result, err := svc.DecidePermission(ctx, planned.Session.Meta.SessionID, protocol.PermissionDecision{
		RequestID: request.RequestID,
		Value:     "accept-once",
	})
	if err != nil {
		t.Fatalf("DecidePermission: %v", err)
	}
	if result.Session.Meta.State != protocol.SessionStateCompleted {
		t.Fatalf("expected completed session, got %s", result.Session.Meta.State)
	}
	execState, err := svc.store.LoadExecutionState(result.Session.Meta.SessionID)
	if err != nil {
		t.Fatalf("LoadExecutionState: %v", err)
	}
	if execState == nil || !executionOutputContains(execState.Outputs, "command-smoke") {
		t.Fatalf("expected command output, got %+v", execState)
	}
}

func TestConfirmWorkspaceInspectRequiresPlanApproval(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	result, err := svc.Execute(context.Background(), protocol.ClientRequest{
		Task:           "总结当前工作区",
		PermissionMode: protocol.PermissionModeConfirm,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(confirm workspace inspect): %v", err)
	}
	if result.Approval == nil || result.Approval.ActiveRequestID == "" {
		t.Fatalf("expected plan approval before confirm-mode inspect, got %+v", result.Approval)
	}
	if len(result.Approval.Requests) != 1 || result.Approval.Requests[0].Tool != "plan_checkpoint" {
		t.Fatalf("expected plan checkpoint request, got %+v", result.Approval.Requests)
	}
}

func TestWorkspaceIntentWinsWithAttachedSources(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	meta, err := svc.NewSession(protocol.PermissionModePlan, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.store.WorkspaceRoot(), "README.md"), []byte("workspace readme"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md): %v", err)
	}
	if err := svc.store.RefreshWorkspaceState(); err != nil {
		t.Fatalf("RefreshWorkspaceState: %v", err)
	}
	if err := svc.store.SaveSources(meta.SessionID, []protocol.PaperRef{
		{PaperID: "paper_a", URI: "/tmp/paper-a.pdf", LocalPath: "/tmp/paper-a.pdf", SourceType: protocol.SourceTypeLocalPDF, Status: protocol.SourceStatusAttached},
	}); err != nil {
		t.Fatalf("SaveSources: %v", err)
	}
	result, err := svc.Execute(context.Background(), protocol.ClientRequest{
		SessionID:      meta.SessionID,
		Task:           "search README for workspace",
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(workspace intent with sources): %v", err)
	}
	if result.Plan == nil || len(result.Plan.DAG.Nodes) != 1 {
		t.Fatalf("expected one workspace plan node, got %+v", result.Plan)
	}
	if got := result.Plan.DAG.Nodes[0].Kind; got != protocol.NodeKindWorkspaceSearch {
		t.Fatalf("expected workspace search despite attached sources, got %s", got)
	}
}

func TestAcceptSessionAllowsMatchingCommandPrefix(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	sessionID := seedCommandPlan(t, svc, []string{"printf %s first", "printf %s second"})
	approval, err := svc.store.LoadPendingApproval(sessionID)
	if err != nil {
		t.Fatalf("LoadPendingApproval: %v", err)
	}
	result, err := svc.DecidePermission(context.Background(), sessionID, protocol.PermissionDecision{
		RequestID: approval.ActiveRequestID,
		Value:     "accept-session",
		Scope:     "command-prefix",
	})
	if err != nil {
		t.Fatalf("DecidePermission(accept-session): %v", err)
	}
	if result.Session.Approval != nil {
		t.Fatalf("expected all matching commands auto-allowed, got %+v", result.Session.Approval)
	}
	if !executionOutputContains(result.Session.Execution.Outputs, "first") || !executionOutputContains(result.Session.Execution.Outputs, "second") {
		t.Fatalf("expected both commands to run, got %+v", result.Session.Execution.Outputs)
	}
}

func TestRunTaskHonorsSelectedTaskID(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	sessionID := seedCommandPlan(t, svc, []string{"printf %s first", "printf %s second"})
	if err := svc.store.DeletePendingApproval(sessionID); err != nil {
		t.Fatalf("DeletePendingApproval: %v", err)
	}
	meta, err := svc.store.LoadMeta(sessionID)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	meta.State = protocol.SessionStatePlanned
	meta.ApprovalPending = false
	meta.PermissionMode = protocol.PermissionModeAuto
	if err := svc.store.SaveMeta(meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}

	result, err := svc.RunTask(context.Background(), sessionID, "cmd_2", "zh", "distill")
	if err != nil {
		t.Fatalf("RunTask(cmd_2): %v", err)
	}
	if executionOutputContains(result.Session.Execution.Outputs, "first") {
		t.Fatalf("selected task run should not execute cmd_1, got %+v", result.Session.Execution.Outputs)
	}
	if !executionOutputContains(result.Session.Execution.Outputs, "second") {
		t.Fatalf("selected task run should execute cmd_2, got %+v", result.Session.Execution.Outputs)
	}
}

func TestApproveTaskValidatesTaskTarget(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	sessionID := seedCommandPlan(t, svc, []string{"printf %s first", "printf %s second"})
	if _, err := svc.runtime.HandleTaskAction(context.Background(), sessionID, runtimeinput.TaskActionPayload{Action: "approve", TaskID: "cmd_2", Approved: true}, "turn_bad_approval"); err == nil {
		t.Fatalf("expected approve of non-pending task to fail")
	}
	result, err := svc.runtime.HandleTaskAction(context.Background(), sessionID, runtimeinput.TaskActionPayload{Action: "approve", TaskID: "cmd_1", Approved: true}, "turn_good_approval")
	if err != nil {
		t.Fatalf("ApproveTask(cmd_1): %v", err)
	}
	if !executionOutputContains(result.Session.Execution.Outputs, "first") {
		t.Fatalf("expected cmd_1 to run after targeted approval, got %+v", result.Session.Execution.Outputs)
	}
}

func TestRunPlannedDoesNotBypassPendingApproval(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	sessionID := seedCommandPlan(t, svc, []string{"printf %s first"})
	if _, err := svc.RunPlanned(context.Background(), sessionID, "zh", "distill"); err == nil {
		t.Fatalf("expected /run to respect pending approval")
	}
}

func TestRunTaskDoesNotBypassPendingApproval(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	sessionID := seedCommandPlan(t, svc, []string{"printf %s first"})
	if _, err := svc.RunTask(context.Background(), sessionID, "cmd_1", "zh", "distill"); err == nil {
		t.Fatalf("expected /task run to respect pending approval")
	}
}

func TestRunTaskConfirmSideEffectCreatesApproval(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	sessionID := seedCommandPlan(t, svc, []string{"printf %s first"})
	if err := svc.store.DeletePendingApproval(sessionID); err != nil {
		t.Fatalf("DeletePendingApproval: %v", err)
	}
	meta, err := svc.store.LoadMeta(sessionID)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	meta.State = protocol.SessionStatePlanned
	meta.ApprovalPending = false
	meta.PermissionMode = protocol.PermissionModeConfirm
	if err := svc.store.SaveMeta(meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	result, err := svc.RunTask(context.Background(), sessionID, "cmd_1", "zh", "distill")
	if err != nil {
		t.Fatalf("RunTask(confirm side effect): %v", err)
	}
	if result.Session.Approval == nil || result.Session.Approval.ActiveRequestID == "" {
		t.Fatalf("expected task-scoped approval, got %+v", result.Session.Approval)
	}
	if result.Session.Meta.PermissionMode != protocol.PermissionModeConfirm {
		t.Fatalf("expected confirm mode to be preserved, got %s", result.Session.Meta.PermissionMode)
	}
	if executionOutputContains(result.Session.Execution.Outputs, "first") {
		t.Fatalf("confirm task run should not execute before approval, got %+v", result.Session.Execution.Outputs)
	}
}

func TestTaskScopedApprovalDoesNotContinuePlan(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	sessionID := seedCommandPlan(t, svc, []string{"printf %s first", "printf %s second"})
	if err := svc.store.DeletePendingApproval(sessionID); err != nil {
		t.Fatalf("DeletePendingApproval: %v", err)
	}
	meta, err := svc.store.LoadMeta(sessionID)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	meta.State = protocol.SessionStatePlanned
	meta.ApprovalPending = false
	meta.PermissionMode = protocol.PermissionModeConfirm
	if err := svc.store.SaveMeta(meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	result, err := svc.RunTask(context.Background(), sessionID, "cmd_1", "zh", "distill")
	if err != nil {
		t.Fatalf("RunTask(confirm side effect): %v", err)
	}
	approval := result.Session.Approval
	if approval == nil || approval.Mode != "task" {
		t.Fatalf("expected task-scoped approval, got %+v", approval)
	}
	result, err = svc.DecidePermission(context.Background(), sessionID, protocol.PermissionDecision{
		RequestID: approval.ActiveRequestID,
		Value:     "accept-once",
	})
	if err != nil {
		t.Fatalf("DecidePermission(task scoped): %v", err)
	}
	if !executionOutputContains(result.Session.Execution.Outputs, "first") {
		t.Fatalf("expected approved task output, got %+v", result.Session.Execution.Outputs)
	}
	if executionOutputContains(result.Session.Execution.Outputs, "second") {
		t.Fatalf("task-scoped approval should not continue unrelated plan steps, got %+v", result.Session.Execution.Outputs)
	}
	if result.Session.Approval != nil || result.Session.Meta.ApprovalPending {
		t.Fatalf("task-scoped approval should not enqueue another approval, got approval=%+v meta=%+v", result.Session.Approval, result.Session.Meta)
	}
	if result.Session.Meta.State != protocol.SessionStatePlanned {
		t.Fatalf("expected unfinished plan to return to planned, got %s", result.Session.Meta.State)
	}
}

func TestRunTaskRejectsBlockedDependencies(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	meta, err := svc.NewSession(protocol.PermissionModeAuto, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	now := time.Now().UTC()
	plan := protocol.PlanResult{
		PlanID: "plan_" + meta.SessionID,
		Goal:   "compare papers",
		DAG: protocol.PlanDAG{Nodes: []protocol.PlanNode{
			{ID: "distill_1", Kind: protocol.NodeKindPaperSummary, Goal: "distill", WorkerProfile: protocol.WorkerProfilePaperSummary, Required: true, Status: protocol.NodeStatusReady},
			{ID: "compare_papers", Kind: protocol.NodeKindFinalSynthesis, Goal: "compare", WorkerProfile: protocol.WorkerProfileMethodCompare, Required: true, Status: protocol.NodeStatusPending, DependsOn: []string{"distill_1"}},
		}},
		Steps: []protocol.PlanStep{
			{ID: "distill_1", Tool: "distill_paper", PaperIDs: []string{"paper_1"}, Goal: "distill", ExpectedArtifact: "paper_1"},
			{ID: "compare_papers", Tool: "compare_papers", PaperIDs: []string{"paper_1", "paper_2"}, Goal: "compare", ExpectedArtifact: "comparison"},
		},
		CreatedAt: now,
	}
	board := protocol.TaskBoard{PlanID: plan.PlanID, Goal: plan.Goal, Tasks: []protocol.TaskCard{
		{TaskID: "distill_1", NodeID: "distill_1", Kind: protocol.NodeKindPaperSummary, Status: protocol.TaskStatusReady},
		{TaskID: "compare_papers", NodeID: "compare_papers", Kind: protocol.NodeKindFinalSynthesis, Status: protocol.TaskStatusBlocked, DependsOn: []string{"distill_1"}},
	}, UpdatedAt: now}
	plan.TaskBoard = &board
	if err := svc.store.SavePlan(meta.SessionID, plan); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := svc.store.SaveExecutionState(meta.SessionID, protocol.ExecutionState{PlanID: plan.PlanID, Nodes: []protocol.NodeExecutionState{
		{NodeID: "distill_1", WorkerProfile: protocol.WorkerProfilePaperSummary, Status: protocol.NodeStatusReady},
		{NodeID: "compare_papers", WorkerProfile: protocol.WorkerProfileMethodCompare, Status: protocol.NodeStatusPending},
	}, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveExecutionState: %v", err)
	}
	meta.State = protocol.SessionStatePlanned
	meta.ActivePlanID = plan.PlanID
	if err := svc.store.SaveMeta(meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	if _, err := svc.RunTask(context.Background(), meta.SessionID, "compare_papers", "zh", "distill"); err == nil || !strings.Contains(err.Error(), "blocked by unmet dependencies") {
		t.Fatalf("expected blocked dependency error, got %v", err)
	}
}

func TestExecuteInvalidatesStalePendingApproval(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	sessionID := seedCommandPlan(t, svc, []string{"printf %s stale"})
	result, err := svc.Execute(context.Background(), protocol.ClientRequest{
		SessionID:      sessionID,
		Task:           "总结当前工作区",
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(new task): %v", err)
	}
	if result.Session.Approval != nil || result.Session.Meta.ApprovalPending {
		t.Fatalf("expected stale approval cleared, got approval=%+v meta=%+v", result.Session.Approval, result.Session.Meta)
	}
	if approval, err := svc.store.LoadPendingApproval(sessionID); err != nil || approval != nil {
		t.Fatalf("expected pending approval file removed, approval=%+v err=%v", approval, err)
	}
}

func TestFinishCompletedPropagatesResultEmitFailure(t *testing.T) {
	t.Parallel()

	svc := newTestServiceWithSink(t, failingResultSink{})
	sessionID := seedCommandPlan(t, svc, []string{"printf %s first"})
	approval, err := svc.store.LoadPendingApproval(sessionID)
	if err != nil {
		t.Fatalf("LoadPendingApproval: %v", err)
	}
	_, err = svc.DecidePermission(context.Background(), sessionID, protocol.PermissionDecision{
		RequestID: approval.ActiveRequestID,
		Value:     "accept-once",
	})
	if err == nil || !strings.Contains(err.Error(), "sink result failure") {
		t.Fatalf("expected final result emit failure, got %v", err)
	}
}

func TestWorkspaceEditRunsInAutoMode(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	result, err := svc.Execute(context.Background(), protocol.ClientRequest{
		Task:           "create `notes.md` with a short workspace note",
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(auto edit): %v", err)
	}
	content, err := os.ReadFile(filepath.Join(svc.store.WorkspaceRoot(), "notes.md"))
	if err != nil {
		t.Fatalf("ReadFile(notes.md): %v", err)
	}
	if !strings.Contains(string(content), "workspace note") {
		t.Fatalf("expected generated edit content, got %q", string(content))
	}
	if !executionOutputContains(result.Session.Execution.Outputs, "notes.md") {
		t.Fatalf("expected edit output in execution state, got %+v", result.Session.Execution.Outputs)
	}
}

func TestWorkspaceEditRequiresExplicitTarget(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	_, err := svc.Execute(context.Background(), protocol.ClientRequest{
		Task:           "create a short workspace note",
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
	})
	if err == nil || !strings.Contains(err.Error(), "explicit target file path") {
		t.Fatalf("expected explicit target error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(svc.store.WorkspaceRoot(), "notes.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("notes.md should not be created by default target fallback, stat err=%v", statErr)
	}
	sessionID, loadErr := svc.store.LatestSessionID()
	if loadErr != nil {
		t.Fatalf("LatestSessionID: %v", loadErr)
	}
	meta, loadErr := svc.store.LoadMeta(sessionID)
	if loadErr != nil {
		t.Fatalf("LoadMeta: %v", loadErr)
	}
	if meta.State != protocol.SessionStateFailed {
		t.Fatalf("expected failed session after edit target error, got %s", meta.State)
	}
}

func TestWorkspaceEditDoesNotOverwriteWithInstructionText(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	readmePath := filepath.Join(svc.store.WorkspaceRoot(), "README.md")
	original := "hello typo\n"
	if err := os.WriteFile(readmePath, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md): %v", err)
	}
	if err := svc.store.RefreshWorkspaceState(); err != nil {
		t.Fatalf("RefreshWorkspaceState: %v", err)
	}
	_, err := svc.Execute(context.Background(), protocol.ClientRequest{
		Task:           "fix typo in `README.md`",
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
	})
	if err == nil || !strings.Contains(err.Error(), "explicit content or a replacement") {
		t.Fatalf("expected safe edit content error, got %v", err)
	}
	content, readErr := os.ReadFile(readmePath)
	if readErr != nil {
		t.Fatalf("ReadFile(README.md): %v", readErr)
	}
	if string(content) != original {
		t.Fatalf("instruction-only edit should not overwrite file, got %q", string(content))
	}
}

func TestWorkspaceEditReplacementUsesCurrentContent(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	readmePath := filepath.Join(svc.store.WorkspaceRoot(), "README.md")
	if err := os.WriteFile(readmePath, []byte("hello typo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md): %v", err)
	}
	if err := svc.store.RefreshWorkspaceState(); err != nil {
		t.Fatalf("RefreshWorkspaceState: %v", err)
	}
	_, err := svc.Execute(context.Background(), protocol.ClientRequest{
		Task:           "update `README.md` replace `typo` with `type`",
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute(replacement edit): %v", err)
	}
	content, readErr := os.ReadFile(readmePath)
	if readErr != nil {
		t.Fatalf("ReadFile(README.md): %v", readErr)
	}
	if string(content) != "hello type\n" {
		t.Fatalf("expected replacement against current content, got %q", string(content))
	}
}

func TestApprovedCommandFailureMarksNodeFailed(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	sessionID := seedCommandPlan(t, svc, []string{"sh -c 'exit 7'"})
	approval, err := svc.store.LoadPendingApproval(sessionID)
	if err != nil {
		t.Fatalf("LoadPendingApproval: %v", err)
	}
	if _, err := svc.DecidePermission(context.Background(), sessionID, protocol.PermissionDecision{
		RequestID: approval.ActiveRequestID,
		Value:     "accept-once",
	}); err == nil {
		t.Fatalf("expected command failure")
	}
	execState, err := svc.store.LoadExecutionState(sessionID)
	if err != nil {
		t.Fatalf("LoadExecutionState: %v", err)
	}
	if execState == nil || len(execState.Nodes) == 0 || execState.Nodes[0].Status != protocol.NodeStatusFailed {
		t.Fatalf("expected failed node after command error, got %+v", execState)
	}
}

func TestExecutePlanFailureMarksSessionFailed(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	_, err := svc.Execute(context.Background(), protocol.ClientRequest{
		Task:           "run command `sh -c 'exit 7'`",
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
	})
	if err == nil {
		t.Fatalf("expected command failure")
	}
	sessionID, loadErr := svc.store.LatestSessionID()
	if loadErr != nil {
		t.Fatalf("LatestSessionID: %v", loadErr)
	}
	meta, loadErr := svc.store.LoadMeta(sessionID)
	if loadErr != nil {
		t.Fatalf("LoadMeta: %v", loadErr)
	}
	if meta.State != protocol.SessionStateFailed {
		t.Fatalf("expected failed session state, got %s", meta.State)
	}
}

func TestExecuteValidationFailureDoesNotLeaveSessionRunning(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	meta, err := svc.NewSession(protocol.PermissionModePlan, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := svc.Execute(context.Background(), protocol.ClientRequest{
		SessionID:      meta.SessionID,
		Task:           "   ",
		PermissionMode: protocol.PermissionModePlan,
		Sources:        []string{filepath.Join(svc.store.WorkspaceRoot(), "missing.pdf")},
		Language:       "zh",
		Style:          "distill",
	}); err == nil {
		t.Fatalf("expected attach/validation failure")
	}
	loaded, err := svc.store.LoadMeta(meta.SessionID)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if loaded.State == protocol.SessionStateRunning {
		t.Fatalf("session should not remain running after validation failure")
	}
}

func TestExecutePlanningFailureDoesNotLeaveSessionRunning(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	meta, err := svc.NewSession(protocol.PermissionModeAuto, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	_, err = svc.Execute(context.Background(), protocol.ClientRequest{
		SessionID:      meta.SessionID,
		Task:           "summarize paper https://example.com/not-a-pdf",
		PermissionMode: protocol.PermissionModeAuto,
		Language:       "zh",
		Style:          "distill",
	})
	if err == nil {
		t.Fatalf("expected planning failure")
	}
	loaded, err := svc.store.LoadMeta(meta.SessionID)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if loaded.State != protocol.SessionStateFailed {
		t.Fatalf("expected failed session state after planning failure, got %s", loaded.State)
	}
}

func TestTurnLoopReceivesServiceInputs(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	var got turnloop.TurnEnvelope
	svc.loop = turnloop.New(turnloop.Config{
		Handler: func(_ context.Context, envelope turnloop.TurnEnvelope) (protocol.RunResult, error) {
			got = envelope
			return protocol.RunResult{}, nil
		},
	})
	_, err := svc.Execute(context.Background(), protocol.ClientRequest{
		Task:           "inspect the workspace",
		PermissionMode: protocol.PermissionModePlan,
		Language:       "zh",
		Style:          "distill",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.SessionID == "" || len(got.Items) != 1 || got.Items[0].Kind != runtimeinput.KindUserMessage {
		t.Fatalf("expected user input pushed into turnloop, got %+v", got)
	}
}

func TestNativeHandleTurnProcessesAllBufferedItems(t *testing.T) {
	t.Parallel()

	svc, sink := newTestService(t)
	meta, err := svc.NewSession(protocol.PermissionModePlan, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	_, err = svc.runtime.HandleTurn(context.Background(), turnloop.TurnEnvelope{
		TurnID:    "turn_multi",
		SessionID: meta.SessionID,
		Items: []runtimeinput.Item{
			runtimeinput.FromClientRequest(protocol.ClientRequest{SessionID: meta.SessionID, Task: "总结当前工作区", PermissionMode: protocol.PermissionModePlan, Language: "zh", Style: "distill"}),
			runtimeinput.FromClientRequest(protocol.ClientRequest{SessionID: meta.SessionID, Task: "搜索 workspace", PermissionMode: protocol.PermissionModePlan, Language: "zh", Style: "distill"}),
		},
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("HandleTurn(multi): %v", err)
	}
	planEvents := 0
	for _, event := range sink.events {
		if event.Type == protocol.EventPlan {
			planEvents++
		}
	}
	if planEvents != 2 {
		t.Fatalf("expected both buffered inputs to execute, saw %d plan events: %+v", planEvents, sink.events)
	}
}

func TestNewSessionCopiesActiveProviderProfileAndModel(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.BaseDir = t.TempDir()
	cfg.ActiveProvider = "local-openai"
	cfg.Providers = map[string]config.ProviderConfig{
		"local-openai": {
			Provider: config.ProviderOpenAI,
			Model:    "gpt-5.4",
			BaseURL:  "http://127.0.0.1:8317/v1",
			APIKey:   "local-test",
			Timeout:  "2m",
		},
	}
	cfg.Provider = cfg.Providers[cfg.ActiveProvider]
	store := storage.New(cfg.BaseDir)
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	sink := &testSink{}
	registry := tools.New(pipeline.New(cfg))
	svc := New(cfg, store, registry, sink)
	meta, err := svc.NewSession(protocol.PermissionModePlan, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if meta.ProviderProfile != "local-openai" || meta.Model != "gpt-5.4" {
		t.Fatalf("expected provider/model copied from config, got %+v", meta)
	}
}

func TestListAndRunSkillsUseNativeRuntime(t *testing.T) {
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
	if len(descriptors) != 4 || descriptors[0].Name != protocol.SkillNameReviewer {
		t.Fatalf("unexpected descriptors: %+v", descriptors)
	}
	digest := protocol.PaperDigest{PaperID: "paper_a", Title: "Paper A", OneLineSummary: "summary", Language: "zh", Style: "distill", GeneratedAt: time.Now().UTC()}
	if err := svc.store.SaveDigest(meta.SessionID, digest); err != nil {
		t.Fatalf("SaveDigest: %v", err)
	}
	result, err := svc.RunSkill(context.Background(), meta.SessionID, string(protocol.SkillNameReviewer), "paper_a")
	if err != nil {
		t.Fatalf("RunSkill: %v", err)
	}
	if result.Run.Status != protocol.SkillRunStatusCompleted || result.Artifact == nil {
		t.Fatalf("expected completed skill artifact, got %+v", result)
	}
	if result.Artifact.Kind != "reviewer_skill" {
		t.Fatalf("expected descriptor artifact kind, got %q", result.Artifact.Kind)
	}
}

func TestPaperSkillRequiresSessionPaperTarget(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	meta, err := svc.NewSession(protocol.PermissionModePlan, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := svc.RunSkill(context.Background(), meta.SessionID, string(protocol.SkillNameReviewer), "missing_paper"); err == nil {
		t.Fatalf("expected paper skill with unknown target to fail")
	}
	if err := svc.store.SaveSources(meta.SessionID, []protocol.PaperRef{
		{PaperID: "paper_a", URI: "/tmp/paper-a.pdf", LocalPath: "/tmp/paper-a.pdf", SourceType: protocol.SourceTypeLocalPDF, Status: protocol.SourceStatusAttached},
	}); err != nil {
		t.Fatalf("SaveSources: %v", err)
	}
	if _, err := svc.RunSkill(context.Background(), meta.SessionID, string(protocol.SkillNameReviewer), "paper_b"); err == nil {
		t.Fatalf("expected paper skill with non-session target to fail")
	}
}

func TestComparisonSkillRequiresComparisonAndPersistsPaperIDs(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)
	meta, err := svc.NewSession(protocol.PermissionModePlan, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := svc.RunSkill(context.Background(), meta.SessionID, string(protocol.SkillNameCompareRefinement), "comparison"); err == nil {
		t.Fatalf("expected comparison skill without comparison to fail")
	}
	if err := svc.store.SaveSources(meta.SessionID, []protocol.PaperRef{
		{PaperID: "paper_1", URI: "/tmp/paper1.pdf", LocalPath: "/tmp/paper1.pdf", SourceType: protocol.SourceTypeLocalPDF, Status: protocol.SourceStatusAttached},
		{PaperID: "paper_2", URI: "/tmp/paper2.pdf", LocalPath: "/tmp/paper2.pdf", SourceType: protocol.SourceTypeLocalPDF, Status: protocol.SourceStatusAttached},
	}); err != nil {
		t.Fatalf("SaveSources: %v", err)
	}
	if err := svc.store.SaveComparison(meta.SessionID, protocol.ComparisonDigest{PaperIDs: []string{"paper_1", "paper_2"}, Goal: "compare", GeneratedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("SaveComparison: %v", err)
	}
	result, err := svc.RunSkill(context.Background(), meta.SessionID, string(protocol.SkillNameCompareRefinement), "comparison")
	if err != nil {
		t.Fatalf("RunSkill(comparison): %v", err)
	}
	if strings.Join(result.Run.PaperIDs, ",") != "paper_1,paper_2" {
		t.Fatalf("expected comparison paper IDs on run, got %+v", result.Run.PaperIDs)
	}
	raw, err := os.ReadFile(result.Artifact.Paths["json"])
	if err != nil {
		t.Fatalf("ReadFile(skill json): %v", err)
	}
	if !strings.Contains(string(raw), "paper_1") || !strings.Contains(string(raw), "paper_2") {
		t.Fatalf("expected paper_ids in artifact json, got %s", string(raw))
	}
}

func newTestService(t *testing.T) (*Service, *testSink) {
	t.Helper()
	sink := &testSink{}
	return newTestServiceWithSink(t, sink), sink
}

func newTestServiceWithSink(t *testing.T, sink EventSink) *Service {
	t.Helper()
	cfg := config.Default()
	cfg.BaseDir = t.TempDir()
	store := storage.New(cfg.BaseDir)
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	registry := tools.New(pipeline.New(cfg))
	return New(cfg, store, registry, sink)
}

func seedCommandPlan(t *testing.T, svc *Service, commands []string) string {
	t.Helper()
	meta, err := svc.NewSession(protocol.PermissionModeConfirm, "zh", "distill")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	now := time.Now().UTC()
	nodes := make([]protocol.PlanNode, 0, len(commands))
	steps := make([]protocol.PlanStep, 0, len(commands))
	execNodes := make([]protocol.NodeExecutionState, 0, len(commands))
	for i, command := range commands {
		id := fmt.Sprintf("cmd_%d", i+1)
		goal := fmt.Sprintf("run command `%s`", command)
		nodes = append(nodes, protocol.PlanNode{ID: id, Kind: protocol.NodeKindWorkspaceCommand, Goal: goal, WorkerProfile: protocol.WorkerProfileSupervisor, Required: true, Status: protocol.NodeStatusReady})
		steps = append(steps, protocol.PlanStep{ID: id, Tool: string(protocol.NodeKindWorkspaceCommand), Goal: goal, ExpectedArtifact: "workspace_response"})
		execNodes = append(execNodes, protocol.NodeExecutionState{NodeID: id, WorkerProfile: protocol.WorkerProfileSupervisor, Status: protocol.NodeStatusReady})
	}
	plan := protocol.PlanResult{PlanID: "plan_" + meta.SessionID, Goal: "run commands", DAG: protocol.PlanDAG{Nodes: nodes}, Steps: steps, ApprovalRequired: true, CreatedAt: now}
	board := protocol.TaskBoard{PlanID: plan.PlanID, Goal: plan.Goal, UpdatedAt: now}
	for _, node := range nodes {
		board.Tasks = append(board.Tasks, protocol.TaskCard{TaskID: node.ID, NodeID: node.ID, Kind: node.Kind, Title: node.Goal, Status: protocol.TaskStatusReady})
	}
	plan.TaskBoard = &board
	if err := svc.store.SavePlan(meta.SessionID, plan); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := svc.store.SaveExecutionState(meta.SessionID, protocol.ExecutionState{PlanID: plan.PlanID, Nodes: execNodes, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveExecutionState: %v", err)
	}
	meta.State = protocol.SessionStateAwaitingApproval
	meta.ApprovalPending = true
	meta.ActivePlanID = plan.PlanID
	if err := svc.store.SaveMeta(meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	request := protocol.PermissionRequest{
		RequestID: "req_first",
		SessionID: meta.SessionID,
		PlanID:    plan.PlanID,
		NodeID:    "cmd_1",
		Tool:      string(protocol.NodeKindWorkspaceCommand),
		Operation: "shell",
		Title:     "Run command",
		Question:  "Do you want to run this command?",
		Command:   commands[0],
		Preview:   protocol.PermissionPreview{Kind: "command", CommandPrefix: "printf %s"},
		Options: []protocol.PermissionOption{
			{Value: "accept-once", Label: "Yes", Scope: "node"},
			{Value: "accept-session", Label: "Yes, during this session", Scope: "command-prefix"},
			{Value: "reject", Label: "No", Scope: "node"},
		},
		CreatedAt: now,
	}
	approval := protocol.ApprovalRequest{PlanID: plan.PlanID, CheckpointID: "checkpoint_" + plan.PlanID, InterruptID: "permission_req_first", PendingNodeIDs: []string{"cmd_1"}, Summary: request.Question, RequiresInput: true, CreatedAt: now, Mode: "plan", ActiveRequestID: request.RequestID, Requests: []protocol.PermissionRequest{request}}
	if err := svc.store.SavePendingApproval(meta.SessionID, approval); err != nil {
		t.Fatalf("SavePendingApproval: %v", err)
	}
	return meta.SessionID
}

func executionOutputContains(outputs []protocol.NodeOutputRef, needle string) bool {
	for _, output := range outputs {
		for _, value := range output.Data {
			if strings.Contains(fmt.Sprint(value), needle) {
				return true
			}
		}
	}
	return false
}
