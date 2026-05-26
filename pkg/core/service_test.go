package core

import (
	"context"
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
	registry := tools.New(pipeline.New(cfg))
	return New(cfg, store, registry, sink), sink
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
	approval := protocol.ApprovalRequest{PlanID: plan.PlanID, CheckpointID: "checkpoint_" + plan.PlanID, InterruptID: "permission_req_first", PendingNodeIDs: []string{"cmd_1"}, Summary: request.Question, RequiresInput: true, CreatedAt: now, Mode: "task", ActiveRequestID: request.RequestID, Requests: []protocol.PermissionRequest{request}}
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
