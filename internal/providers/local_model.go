package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type LocalToolCallingChatModel struct {
	tools map[string]*schema.ToolInfo
}

type localEnvelope struct {
	Mode string `json:"mode"`
}

type localPlannerInput struct {
	Mode    string              `json:"mode"`
	Goal    string              `json:"goal"`
	Sources []protocol.PaperRef `json:"sources"`
}

type localExecutorInput struct {
	Mode  string            `json:"mode"`
	Goal  string            `json:"goal"`
	Plan  localPlanEnvelope `json:"plan"`
	Step  protocol.PlanStep `json:"step"`
	Lang  string            `json:"lang"`
	Style string            `json:"style"`
}

type localPlanEnvelope struct {
	Steps []protocol.PlanStep `json:"steps"`
}

type localReplannerInput struct {
	Mode             string            `json:"mode"`
	Goal             string            `json:"goal"`
	Plan             localPlanEnvelope `json:"plan"`
	AvailableDigests []string          `json:"available_digests,omitempty"`
	HasComparison    bool              `json:"has_comparison"`
	CompletedSteps   int               `json:"completed_steps"`
	LastExecutedStep string            `json:"last_executed_step,omitempty"`
	ExecutedStepIDs  []string          `json:"executed_step_ids,omitempty"`
}

func NewLocalToolCallingChatModel() model.ToolCallingChatModel {
	return &LocalToolCallingChatModel{tools: map[string]*schema.ToolInfo{}}
}

func (m *LocalToolCallingChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	cloned := make(map[string]*schema.ToolInfo, len(tools))
	for _, info := range tools {
		if info == nil {
			continue
		}
		cloned[info.Name] = info
	}
	return &LocalToolCallingChatModel{tools: cloned}, nil
}

func (m *LocalToolCallingChatModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return m.generate(input)
}

func (m *LocalToolCallingChatModel) Stream(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		msg, err := m.generate(input)
		if err != nil {
			sw.Send(nil, err)
			return
		}
		sw.Send(msg, nil)
	}()
	_ = ctx
	return sr, nil
}

func (m *LocalToolCallingChatModel) generate(input []*schema.Message) (*schema.Message, error) {
	if len(input) == 0 {
		return schema.AssistantMessage(`{"steps":[]}`, nil), nil
	}

	last := input[len(input)-1]
	if last.Role == schema.Tool {
		return schema.AssistantMessage(fmt.Sprintf("step completed: %s", last.ToolName), nil), nil
	}

	var env localEnvelope
	if err := json.Unmarshal([]byte(last.Content), &env); err != nil {
		return schema.AssistantMessage(`{"steps":[]}`, nil), nil
	}

	switch env.Mode {
	case "planner":
		return m.generatePlanner(last.Content)
	case "executor":
		return m.generateExecutor(last.Content)
	case "replanner":
		return m.generateReplanner(last.Content)
	default:
		return schema.AssistantMessage(`{"steps":[]}`, nil), nil
	}
}

func (m *LocalToolCallingChatModel) generatePlanner(raw string) (*schema.Message, error) {
	var in localPlannerInput
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil, err
	}
	steps := buildLocalPlanSteps(in.Goal, in.Sources)
	return assistantToolCall(planToolName(m.tools), map[string]any{
		"steps": steps,
	})
}

func (m *LocalToolCallingChatModel) generateExecutor(raw string) (*schema.Message, error) {
	var in localExecutorInput
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil, err
	}
	switch in.Step.Tool {
	case "approve_plan":
		return assistantToolCall("approve_plan", map[string]any{
			"plan_id": in.Step.ID,
			"summary": in.Step.Goal,
		})
	case "distill_paper":
		args := map[string]any{
			"paper_id": in.Step.PaperIDs[0],
			"goal":     in.Goal,
			"lang":     in.Lang,
			"style":    in.Style,
		}
		return assistantToolCall("distill_paper", args)
	case "compare_papers":
		return assistantToolCall("compare_papers", map[string]any{
			"paper_ids": in.Step.PaperIDs,
			"goal":      in.Goal,
			"lang":      in.Lang,
			"style":     in.Style,
		})
	case "export_artifact":
		return assistantToolCall("export_artifact", map[string]any{
			"artifact_id": in.Step.ExpectedArtifact,
			"format":      "md",
		})
	default:
		return schema.AssistantMessage("step completed", nil), nil
	}
}

func (m *LocalToolCallingChatModel) generateReplanner(raw string) (*schema.Message, error) {
	var in localReplannerInput
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil, err
	}
	remaining := append([]protocol.PlanStep(nil), in.Plan.Steps...)
	if len(in.ExecutedStepIDs) > 0 {
		seen := make(map[string]struct{}, len(in.ExecutedStepIDs))
		for _, id := range in.ExecutedStepIDs {
			seen[id] = struct{}{}
		}
		filteredByID := make([]protocol.PlanStep, 0, len(remaining))
		for _, step := range remaining {
			if _, ok := seen[step.ID]; ok {
				continue
			}
			filteredByID = append(filteredByID, step)
		}
		remaining = filteredByID
	} else if in.LastExecutedStep != "" {
		filteredByID := make([]protocol.PlanStep, 0, len(remaining))
		removed := false
		for _, step := range remaining {
			if !removed && step.ID == in.LastExecutedStep {
				removed = true
				continue
			}
			filteredByID = append(filteredByID, step)
		}
		remaining = filteredByID
	} else if in.CompletedSteps > 0 && in.CompletedSteps <= len(remaining) {
		remaining = remaining[in.CompletedSteps:]
	} else if in.CompletedSteps > len(remaining) {
		remaining = nil
	}
	filtered := make([]protocol.PlanStep, 0, len(remaining))
	for _, step := range remaining {
		if step.Tool == "compare_papers" {
			if comparablePaperCount(step.PaperIDs, in.AvailableDigests) < 2 {
				continue
			}
		}
		filtered = append(filtered, step)
	}
	if len(filtered) == 0 {
		return assistantToolCall(respondToolName(m.tools), map[string]any{
			"response": "Execution completed successfully.",
		})
	}
	return assistantToolCall(planToolName(m.tools), map[string]any{
		"steps": filtered,
	})
}

func buildLocalPlanSteps(goal string, sources []protocol.PaperRef) []protocol.PlanStep {
	extractable := make([]protocol.PaperRef, 0, len(sources))
	for _, source := range sources {
		if source.Inspection.ExtractableText {
			extractable = append(extractable, source)
		}
	}
	steps := make([]protocol.PlanStep, 0, len(extractable)+1)
	for idx, source := range extractable {
		steps = append(steps, protocol.PlanStep{
			ID:               fmt.Sprintf("step_%02d", idx+1),
			Tool:             "distill_paper",
			PaperIDs:         []string{source.PaperID},
			Goal:             fmt.Sprintf("提炼 %s 的核心贡献、实验和结果", source.PaperID),
			ExpectedArtifact: source.PaperID,
		})
	}
	if len(extractable) > 1 {
		paperIDs := make([]string, 0, len(extractable))
		for _, source := range extractable {
			paperIDs = append(paperIDs, source.PaperID)
		}
		steps = append(steps, protocol.PlanStep{
			ID:               fmt.Sprintf("step_%02d", len(steps)+1),
			Tool:             "compare_papers",
			PaperIDs:         paperIDs,
			Goal:             fallbackGoal(goal),
			ExpectedArtifact: "comparison",
		})
	}
	return steps
}

func comparablePaperCount(required []string, available []string) int {
	set := make(map[string]struct{}, len(available))
	for _, id := range available {
		set[id] = struct{}{}
	}
	count := 0
	for _, id := range required {
		if _, ok := set[id]; ok {
			count++
		}
	}
	return count
}

func assistantToolCall(name string, args map[string]any) (*schema.Message, error) {
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	return schema.AssistantMessage("", []schema.ToolCall{
		{
			ID:   fmt.Sprintf("call_%d", time.Now().UnixNano()),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      name,
				Arguments: string(raw),
			},
		},
	}), nil
}

func planToolName(tools map[string]*schema.ToolInfo) string {
	for _, candidate := range []string{"plan", "Plan"} {
		if _, ok := tools[candidate]; ok {
			return candidate
		}
	}
	return "plan"
}

func respondToolName(tools map[string]*schema.ToolInfo) string {
	for _, candidate := range []string{"respond", "Respond"} {
		if _, ok := tools[candidate]; ok {
			return candidate
		}
	}
	return "respond"
}

func fallbackGoal(goal string) string {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return "跨论文综合对比"
	}
	return goal
}
