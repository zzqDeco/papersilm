package einoagent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestEinoStableResumeUsesCheckpointStreamingMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpoints := stableCheckpointStore(t)
	agent := &stableInterruptAgent{name: "stable_streaming_agent", description: "streaming checkpoint test"}
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: checkpoints,
	})

	interrupt := drainToInterrupt(t, runner.Query(ctx, "interrupt", adk.WithCheckPointID("stable-streaming")))
	interruptID := rootInterruptID(t, interrupt)

	resumeRunner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{
		Agent:           agent,
		EnableStreaming: false,
		CheckPointStore: checkpoints,
	})
	resumeIter, err := resumeRunner.ResumeWithParams(ctx, "stable-streaming", &adk.ResumeParams{
		Targets: map[string]any{interruptID: "approved"},
	})
	drainWithoutError(t, mustResume(t, resumeIter, err))

	if len(agent.resumeInfos) != 1 {
		t.Fatalf("expected one resume callback, got %d", len(agent.resumeInfos))
	}
	resumeInfo := agent.resumeInfos[0]
	if !resumeInfo.EnableStreaming {
		t.Fatalf("expected v0.9.9 resume to preserve streaming mode from checkpoint")
	}
	if !resumeInfo.IsResumeTarget || resumeInfo.ResumeData != "approved" {
		t.Fatalf("expected targeted resume data, got target=%v data=%v", resumeInfo.IsResumeTarget, resumeInfo.ResumeData)
	}
}

func TestEinoStableResumeWithWrongTargetReinterruptsLeaf(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpoints := stableCheckpointStore(t)
	agent := &stableInterruptAgent{
		name:                       "stable_reinterrupt_agent",
		description:                "reinterrupt test",
		reinterruptWhenNotTargeted: true,
	}
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: checkpoints,
	})

	firstInterrupt := drainToInterrupt(t, runner.Query(ctx, "interrupt", adk.WithCheckPointID("stable-reinterrupt")))
	firstID := rootInterruptID(t, firstInterrupt)
	if firstID == "" {
		t.Fatalf("expected interrupt id")
	}

	resumeIter, err := runner.ResumeWithParams(ctx, "stable-reinterrupt", &adk.ResumeParams{
		Targets: map[string]any{"not-" + firstID: "approved"},
	})
	resumed := mustResume(t, resumeIter, err)
	secondInterrupt := drainToInterrupt(t, resumed)
	if secondInterrupt == nil {
		t.Fatalf("expected non-targeted leaf interrupt to be re-raised")
	}
	if len(agent.resumeInfos) != 1 || agent.resumeInfos[0].IsResumeTarget {
		t.Fatalf("expected non-target resume info, got %+v", agent.resumeInfos)
	}
}

func TestEinoStableAgentToolInterruptBubblesAsCompositeInterrupt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	inner := &stableInterruptAgent{name: "stable_inner_agent", description: "inner interrupting agent"}
	baseTool := adk.NewTypedAgentTool(ctx, adk.TypedAgent[*schema.AgenticMessage](inner))
	invokable, ok := baseTool.(einotool.InvokableTool)
	if !ok {
		t.Fatalf("expected agent tool to be invokable")
	}

	output, err := invokable.InvokableRun(ctx, `{"request":"need approval"}`)
	if err == nil {
		t.Fatalf("expected agent tool interrupt")
	}
	if output != "" {
		t.Fatalf("expected no output when interrupting, got %q", output)
	}
	errText := err.Error()
	if !strings.Contains(errText, "agent tool interrupt") || !strings.Contains(errText, "SubsLen=1") {
		t.Fatalf("expected composite interrupt from agent tool, got %v", err)
	}
}

type stableInterruptAgent struct {
	name                       string
	description                string
	reinterruptWhenNotTargeted bool
	resumeInfos                []*adk.ResumeInfo
}

func (a *stableInterruptAgent) Name(context.Context) string {
	return a.name
}

func (a *stableInterruptAgent) Description(context.Context) string {
	return a.description
}

func (a *stableInterruptAgent) Run(ctx context.Context, _ *adk.TypedAgentInput[*schema.AgenticMessage], _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]] {
	iter, generator := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()
	go func() {
		defer generator.Close()
		event := adk.TypedStatefulInterrupt[*schema.AgenticMessage](ctx, protocol.PermissionRequest{
			Tool:     "stable_test_tool",
			Title:    "Stable interrupt",
			Question: "Allow stable interrupt?",
		}, "stable-state")
		event.AgentName = a.name
		generator.Send(event)
	}()
	return iter
}

func (a *stableInterruptAgent) Resume(ctx context.Context, info *adk.ResumeInfo, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]] {
	a.resumeInfos = append(a.resumeInfos, info)
	iter, generator := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()
	go func() {
		defer generator.Close()
		if a.reinterruptWhenNotTargeted && (info == nil || !info.IsResumeTarget) {
			event := adk.TypedStatefulInterrupt[*schema.AgenticMessage](ctx, protocol.PermissionRequest{
				Tool:     "stable_test_tool",
				Title:    "Still pending",
				Question: "Allow stable interrupt?",
			}, "stable-state")
			event.AgentName = a.name
			generator.Send(event)
			return
		}
		generator.Send(&adk.TypedAgentEvent[*schema.AgenticMessage]{
			AgentName: a.name,
			Output: &adk.TypedAgentOutput[*schema.AgenticMessage]{
				MessageOutput: &adk.TypedMessageVariant[*schema.AgenticMessage]{
					Message: stableAgenticText("resumed"),
				},
			},
		})
	}()
	return iter
}

func stableCheckpointStore(t *testing.T) adk.CheckPointStore {
	t.Helper()
	store := storage.New(filepath.Join(t.TempDir(), ".papersilm"))
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure store: %v", err)
	}
	sessionID := fmt.Sprintf("sess_stable_%d", time.Now().UnixNano())
	if err := store.CreateSession(protocol.SessionMeta{
		SessionID:      sessionID,
		State:          protocol.SessionStateIdle,
		WorkspaceID:    protocol.DefaultWorkspaceID,
		WorkspaceRoot:  store.WorkspaceRoot(),
		PermissionMode: protocol.PermissionModeConfirm,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return store.CheckPointStore(sessionID)
}

func drainToInterrupt(t *testing.T, iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]) *adk.InterruptInfo {
	t.Helper()
	var interrupt *adk.InterruptInfo
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			t.Fatalf("unexpected agent event error: %v", event.Err)
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			interrupt = event.Action.Interrupted
		}
	}
	if interrupt == nil {
		t.Fatalf("expected interrupt event")
	}
	return interrupt
}

func drainWithoutError(t *testing.T, iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]) {
	t.Helper()
	for {
		event, ok := iter.Next()
		if !ok {
			return
		}
		if event != nil && event.Err != nil {
			t.Fatalf("unexpected agent event error: %v", event.Err)
		}
	}
}

func mustResume(t *testing.T, iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]], err error) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]] {
	t.Helper()
	if err != nil {
		t.Fatalf("ResumeWithParams: %v", err)
	}
	return iter
}

func rootInterruptID(t *testing.T, info *adk.InterruptInfo) string {
	t.Helper()
	for _, ctx := range info.InterruptContexts {
		if ctx != nil && ctx.IsRootCause {
			return ctx.ID
		}
	}
	t.Fatalf("expected root interrupt context, got %+v", info)
	return ""
}

func stableAgenticText(text string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: text}),
		},
	}
}
