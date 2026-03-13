package agent

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func init() {
	gob.Register(&executionPlan{})
	gob.Register(protocol.PlanStep{})
}

type executionPlan struct {
	Steps []protocol.PlanStep `json:"steps"`
}

func (p *executionPlan) FirstStep() string {
	if len(p.Steps) == 0 {
		return ""
	}
	raw, _ := json.Marshal(p.Steps[0])
	return string(raw)
}

func (p *executionPlan) MarshalJSON() ([]byte, error) {
	type alias executionPlan
	return json.Marshal((*alias)(p))
}

func (p *executionPlan) UnmarshalJSON(raw []byte) error {
	type alias executionPlan
	return json.Unmarshal(raw, (*alias)(p))
}

type plannerPayload struct {
	Mode    string              `json:"mode"`
	Goal    string              `json:"goal"`
	Sources []protocol.PaperRef `json:"sources"`
}

type executorPayload struct {
	Mode  string            `json:"mode"`
	Goal  string            `json:"goal"`
	Plan  *executionPlan    `json:"plan"`
	Step  protocol.PlanStep `json:"step"`
	Lang  string            `json:"lang"`
	Style string            `json:"style"`
}

type replannerPayload struct {
	Mode             string         `json:"mode"`
	Goal             string         `json:"goal"`
	Plan             *executionPlan `json:"plan"`
	AvailableDigests []string       `json:"available_digests,omitempty"`
	HasComparison    bool           `json:"has_comparison"`
	CompletedSteps   int            `json:"completed_steps"`
	LastExecutedStep string         `json:"last_executed_step,omitempty"`
	ExecutedStepIDs  []string       `json:"executed_step_ids,omitempty"`
}

type savedPlanPlannerAgent struct {
	plan *executionPlan
}

func (a *savedPlanPlannerAgent) Name(context.Context) string {
	return "saved_plan_planner"
}

func (a *savedPlanPlannerAgent) Description(context.Context) string {
	return "replays a previously generated plan"
}

func (a *savedPlanPlannerAgent) Run(ctx context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer gen.Close()
		adk.AddSessionValue(ctx, planexecute.PlanSessionKey, a.plan)
		if input != nil {
			adk.AddSessionValue(ctx, planexecute.UserInputSessionKey, input.Messages)
		}
		raw, err := json.Marshal(a.plan)
		if err != nil {
			gen.Send(&adk.AgentEvent{Err: err})
			return
		}
		gen.Send(adk.EventFromMessage(schema.AssistantMessage(string(raw), nil), nil, schema.Assistant, ""))
	}()
	return iter
}

func newExecutionPlan(steps []protocol.PlanStep) *executionPlan {
	cloned := make([]protocol.PlanStep, 0, len(steps))
	cloned = append(cloned, steps...)
	return &executionPlan{Steps: cloned}
}

func prependApprovalStep(plan *executionPlan, planID, summary string) *executionPlan {
	if plan == nil {
		return &executionPlan{}
	}
	steps := make([]protocol.PlanStep, 0, len(plan.Steps)+1)
	steps = append(steps, protocol.PlanStep{
		ID:               "approval_gate",
		Tool:             "approve_plan",
		Goal:             summary,
		ExpectedArtifact: planID,
	})
	steps = append(steps, plan.Steps...)
	return &executionPlan{Steps: steps}
}

func toProtocolPlan(goal string, refs []protocol.PaperRef, plan *executionPlan, approvalRequired bool) protocol.PlanResult {
	risks := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Inspection.FailureReason != "" {
			risks = append(risks, fmt.Sprintf("%s: %s", ref.PaperID, ref.Inspection.FailureReason))
			continue
		}
		if !ref.Inspection.ExtractableText {
			risks = append(risks, fmt.Sprintf("%s: pdf text extraction produced too little text", ref.PaperID))
		}
	}
	if len(risks) == 0 {
		risks = append(risks, "no major inspection risks detected")
	}
	steps := make([]protocol.PlanStep, 0, len(plan.Steps))
	steps = append(steps, plan.Steps...)
	return protocol.PlanResult{
		PlanID:           newPlanID(),
		Goal:             strings.TrimSpace(goal),
		SourceSummary:    refs,
		Steps:            steps,
		WillCompare:      hasToolStep(steps, "compare_papers"),
		Risks:            risks,
		ApprovalRequired: approvalRequired,
		CreatedAt:        time.Now().UTC(),
	}
}

func hasToolStep(steps []protocol.PlanStep, toolName string) bool {
	for _, step := range steps {
		if step.Tool == toolName {
			return true
		}
	}
	return false
}

func plannerInputMessages(goal string, refs []protocol.PaperRef) ([]adk.Message, error) {
	payload := plannerPayload{
		Mode:    "planner",
		Goal:    goal,
		Sources: refs,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []adk.Message{
		schema.SystemMessage("You are a paper workflow planner. For arXiv paper IDs, arXiv URLs, and AlphaXiv URLs, assume an AlphaXiv-first lookup policy exists behind the tools. Create only executable high-level tool steps. Never add download-PDF, background-writing, or literature-review steps."),
		schema.UserMessage(string(raw)),
	}, nil
}

func executorInputMessages(goal, lang, style string, in *planexecute.ExecutionContext) ([]adk.Message, error) {
	plan, _ := in.Plan.(*executionPlan)
	step := protocol.PlanStep{}
	if plan != nil && len(plan.Steps) > 0 {
		step = plan.Steps[0]
	}
	payload := executorPayload{
		Mode:  "executor",
		Goal:  goal,
		Plan:  plan,
		Step:  step,
		Lang:  lang,
		Style: style,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []adk.Message{
		schema.SystemMessage("You are a paper execution agent. Execute exactly one tool step at a time. When executing distill_paper, pass the original user goal to the tool. For arXiv-capable sources, the tool must prefer AlphaXiv overview, escalate to AlphaXiv full text for equation/table/figure/appendix/proof/derivation/section detail requests or insufficient overview, and only fall back to arXiv PDF when AlphaXiv content is unavailable. Final outputs must preserve provenance."),
		schema.UserMessage(string(raw)),
	}, nil
}

func replannerInputMessages(goal string, plan *executionPlan, digestIDs []string, hasComparison bool, completedSteps int, lastExecutedStep string, executedStepIDs []string) ([]adk.Message, error) {
	payload := replannerPayload{
		Mode:             "replanner",
		Goal:             goal,
		Plan:             plan,
		AvailableDigests: digestIDs,
		HasComparison:    hasComparison,
		CompletedSteps:   completedSteps,
		LastExecutedStep: lastExecutedStep,
		ExecutedStepIDs:  executedStepIDs,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []adk.Message{
		schema.SystemMessage("You are a replanner. Remove completed steps, prune impossible compare steps, and keep distill_paper steps even when AlphaXiv content is unavailable because they may still succeed through arXiv PDF fallback. Respond when no steps remain."),
		schema.UserMessage(string(raw)),
	}, nil
}

func newPlanID() string {
	return fmt.Sprintf("plan_%d", time.Now().UnixNano())
}

func lastExecutedStepID(executed []planexecute.ExecutedStep) string {
	if len(executed) == 0 {
		return ""
	}
	var step protocol.PlanStep
	if err := json.Unmarshal([]byte(executed[len(executed)-1].Step), &step); err != nil {
		return ""
	}
	return step.ID
}

func executedStepIDs(executed []planexecute.ExecutedStep) []string {
	out := make([]string, 0, len(executed))
	for _, item := range executed {
		var step protocol.PlanStep
		if err := json.Unmarshal([]byte(item.Step), &step); err != nil {
			continue
		}
		if strings.TrimSpace(step.ID) == "" {
			continue
		}
		out = append(out, step.ID)
	}
	return out
}

func executionPlanToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "plan",
		Desc: "Create an ordered list of executable paper-processing tool steps.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"steps": {
				Type: schema.Array,
				ElemInfo: &schema.ParameterInfo{
					Type: schema.Object,
					SubParams: map[string]*schema.ParameterInfo{
						"id": {
							Type:     schema.String,
							Desc:     "Stable step identifier.",
							Required: true,
						},
						"tool": {
							Type:     schema.String,
							Desc:     "Tool to call for this step.",
							Required: true,
							Enum:     []string{"distill_paper", "compare_papers", "export_artifact"},
						},
						"paper_ids": {
							Type:     schema.Array,
							ElemInfo: &schema.ParameterInfo{Type: schema.String},
							Desc:     "Paper IDs required by the step.",
						},
						"goal": {
							Type:     schema.String,
							Desc:     "Short goal for the step.",
							Required: true,
						},
						"expected_artifact": {
							Type:     schema.String,
							Desc:     "Artifact expected from the step.",
							Required: true,
						},
					},
				},
				Required: true,
			},
		}),
	}
}
