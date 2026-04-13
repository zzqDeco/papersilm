package agent

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

const maxParallelNodes = 4

type taskKind string

const (
	taskKindDistill taskKind = "distill"
	taskKindCompare taskKind = "compare"
)

type taskSpec struct {
	ID       string
	Kind     taskKind
	Goal     string
	PaperIDs []string
}

func buildTaskSpecs(goal string, refs []protocol.PaperRef) []taskSpec {
	extractable := make([]protocol.PaperRef, 0, len(refs))
	for _, ref := range refs {
		if ref.Inspection.ExtractableText {
			extractable = append(extractable, ref)
		}
	}
	out := make([]taskSpec, 0, len(extractable)+1)
	for idx, ref := range extractable {
		out = append(out, taskSpec{
			ID:       fmt.Sprintf("task_%02d", idx+1),
			Kind:     taskKindDistill,
			Goal:     fmt.Sprintf("提炼 %s 的核心贡献、实验和结果", ref.PaperID),
			PaperIDs: []string{ref.PaperID},
		})
	}
	if len(extractable) > 1 {
		paperIDs := make([]string, 0, len(extractable))
		for _, ref := range extractable {
			paperIDs = append(paperIDs, ref.PaperID)
		}
		out = append(out, taskSpec{
			ID:       fmt.Sprintf("task_%02d", len(out)+1),
			Kind:     taskKindCompare,
			Goal:     fallbackGoal(goal),
			PaperIDs: paperIDs,
		})
	}
	return out
}

func compileDAG(goal string, refs []protocol.PaperRef, specs []taskSpec) protocol.PlanDAG {
	nodes := make([]protocol.PlanNode, 0, len(specs)*4)
	edges := make([]protocol.PlanEdge, 0, len(specs)*4)
	mathRequested := goalNeedsMath(goal)
	webRequested := goalNeedsWebResearch(goal)
	mergeNodes := make([]string, 0, len(refs))

	for _, spec := range specs {
		switch spec.Kind {
		case taskKindDistill:
			paperID := spec.PaperIDs[0]
			summaryID := "paper_summary_" + paperID
			experimentID := "experiment_" + paperID
			mergeID := "merge_digest_" + paperID
			nodes = append(nodes,
				newNode(summaryID, protocol.NodeKindPaperSummary, spec.Goal, []string{paperID}, protocol.WorkerProfilePaperSummary, true, "research"),
				newNode(experimentID, protocol.NodeKindExperiment, spec.Goal, []string{paperID}, protocol.WorkerProfileExperiment, true, "research"),
			)
			mergeDepends := []string{summaryID, experimentID}
			edges = append(edges,
				protocol.PlanEdge{From: summaryID, To: mergeID},
				protocol.PlanEdge{From: experimentID, To: mergeID},
			)
			if mathRequested {
				mathID := "math_reasoner_" + paperID
				nodes = append(nodes, newNode(mathID, protocol.NodeKindMathReasoner, spec.Goal, []string{paperID}, protocol.WorkerProfileMathReasoner, false, "research"))
				mergeDepends = append(mergeDepends, mathID)
				edges = append(edges, protocol.PlanEdge{From: mathID, To: mergeID})
			}
			if webRequested {
				webID := "web_research_" + paperID
				nodes = append(nodes, newNode(webID, protocol.NodeKindWebResearch, spec.Goal, []string{paperID}, protocol.WorkerProfileWebResearch, false, "research"))
				mergeDepends = append(mergeDepends, webID)
				edges = append(edges, protocol.PlanEdge{From: webID, To: mergeID})
			}
			mergeNode := newNode(mergeID, protocol.NodeKindMergeDigest, spec.Goal, []string{paperID}, protocol.WorkerProfileSupervisor, true, "merge")
			mergeNode.DependsOn = mergeDepends
			mergeNode.Produces = []string{paperID}
			nodes = append(nodes, mergeNode)
			mergeNodes = append(mergeNodes, mergeID)
		case taskKindCompare:
			if len(spec.PaperIDs) < 2 {
				continue
			}
			methodID := "method_compare"
			experimentID := "experiment_compare"
			resultsID := "results_compare"
			finalID := "final_synthesis"
			methodNode := newNode(methodID, protocol.NodeKindMethodCompare, spec.Goal, spec.PaperIDs, protocol.WorkerProfileMethodCompare, true, "compare")
			experimentNode := newNode(experimentID, protocol.NodeKindExperimentCompare, spec.Goal, spec.PaperIDs, protocol.WorkerProfileExperimentCompare, true, "compare")
			resultsNode := newNode(resultsID, protocol.NodeKindResultsCompare, spec.Goal, spec.PaperIDs, protocol.WorkerProfileResultsCompare, true, "compare")
			finalNode := newNode(finalID, protocol.NodeKindFinalSynthesis, spec.Goal, spec.PaperIDs, protocol.WorkerProfileSupervisor, true, "synthesis")
			finalNode.DependsOn = []string{methodID, experimentID, resultsID}
			finalNode.Produces = []string{"comparison"}
			for _, mergeID := range mergeNodes {
				methodNode.DependsOn = append(methodNode.DependsOn, mergeID)
				experimentNode.DependsOn = append(experimentNode.DependsOn, mergeID)
				resultsNode.DependsOn = append(resultsNode.DependsOn, mergeID)
				edges = append(edges,
					protocol.PlanEdge{From: mergeID, To: methodID},
					protocol.PlanEdge{From: mergeID, To: experimentID},
					protocol.PlanEdge{From: mergeID, To: resultsID},
				)
			}
			edges = append(edges,
				protocol.PlanEdge{From: methodID, To: finalID},
				protocol.PlanEdge{From: experimentID, To: finalID},
				protocol.PlanEdge{From: resultsID, To: finalID},
			)
			nodes = append(nodes, methodNode, experimentNode, resultsNode, finalNode)
		}
	}
	dag := protocol.PlanDAG{Nodes: nodes, Edges: edges}
	refreshReadyNodes(&dag)
	return dag
}

func newNode(id string, kind protocol.NodeKind, goal string, paperIDs []string, worker protocol.WorkerProfile, required bool, parallelGroup string) protocol.PlanNode {
	return protocol.PlanNode{
		ID:            id,
		Kind:          kind,
		Goal:          goal,
		PaperIDs:      append([]string(nil), paperIDs...),
		WorkerProfile: worker,
		Produces:      append([]string(nil), paperIDs...),
		Required:      required,
		Status:        protocol.NodeStatusPending,
		ParallelGroup: parallelGroup,
	}
}

func projectSteps(dag protocol.PlanDAG) []protocol.PlanStep {
	ordered := topoSortedNodeIDs(dag)
	steps := make([]protocol.PlanStep, 0, len(ordered))
	for _, nodeID := range ordered {
		node, ok := dagNode(dag, nodeID)
		if !ok {
			continue
		}
		tool := string(node.Kind)
		if node.Kind == protocol.NodeKindMergeDigest || node.Kind == protocol.NodeKind("distill_paper") {
			tool = "distill_paper"
		}
		if node.Kind == protocol.NodeKindFinalSynthesis || node.Kind == protocol.NodeKind("compare_papers") {
			tool = "compare_papers"
		}
		steps = append(steps, protocol.PlanStep{
			ID:               node.ID,
			Tool:             tool,
			PaperIDs:         append([]string(nil), node.PaperIDs...),
			Goal:             node.Goal,
			ExpectedArtifact: firstProduce(node),
		})
	}
	return steps
}

func firstProduce(node protocol.PlanNode) string {
	if len(node.Produces) == 0 {
		return node.ID
	}
	return node.Produces[0]
}

func topoSortedNodeIDs(dag protocol.PlanDAG) []string {
	indegree := make(map[string]int, len(dag.Nodes))
	adj := make(map[string][]string, len(dag.Nodes))
	for _, node := range dag.Nodes {
		indegree[node.ID] = 0
	}
	for _, edge := range dag.Edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
		indegree[edge.To]++
	}
	queue := make([]string, 0, len(dag.Nodes))
	for _, node := range dag.Nodes {
		if indegree[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}
	sort.Strings(queue)
	out := make([]string, 0, len(dag.Nodes))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		out = append(out, id)
		for _, next := range adj[id] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
				sort.Strings(queue)
			}
		}
	}
	if len(out) != len(dag.Nodes) {
		ids := make([]string, 0, len(dag.Nodes))
		for _, node := range dag.Nodes {
			ids = append(ids, node.ID)
		}
		sort.Strings(ids)
		return ids
	}
	return out
}

func buildExecutionState(planID string, dag protocol.PlanDAG) protocol.ExecutionState {
	state := protocol.ExecutionState{
		PlanID:    planID,
		UpdatedAt: time.Now().UTC(),
	}
	for _, node := range dag.Nodes {
		state.Nodes = append(state.Nodes, protocol.NodeExecutionState{
			NodeID:        node.ID,
			WorkerProfile: node.WorkerProfile,
			Status:        node.Status,
		})
	}
	return state
}

func refreshReadyNodes(dag *protocol.PlanDAG) {
	for i := range dag.Nodes {
		node := &dag.Nodes[i]
		if node.Status == protocol.NodeStatusCompleted || node.Status == protocol.NodeStatusRunning || node.Status == protocol.NodeStatusFailed || node.Status == protocol.NodeStatusSkipped {
			continue
		}
		if dependenciesSatisfied(*dag, *node) {
			node.Status = protocol.NodeStatusReady
		} else {
			node.Status = protocol.NodeStatusPending
		}
	}
}

func dependenciesSatisfied(dag protocol.PlanDAG, node protocol.PlanNode) bool {
	if len(node.DependsOn) == 0 {
		return true
	}
	for _, depID := range node.DependsOn {
		dep, ok := dagNode(dag, depID)
		if !ok {
			return false
		}
		switch dep.Status {
		case protocol.NodeStatusCompleted:
		case protocol.NodeStatusSkipped:
		case protocol.NodeStatusFailed:
			if dep.Required {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func dagNode(dag protocol.PlanDAG, nodeID string) (protocol.PlanNode, bool) {
	for _, node := range dag.Nodes {
		if node.ID == nodeID {
			return node, true
		}
	}
	return protocol.PlanNode{}, false
}

func setNodeStatus(dag *protocol.PlanDAG, nodeID string, status protocol.NodeStatus) bool {
	for i := range dag.Nodes {
		if dag.Nodes[i].ID == nodeID {
			dag.Nodes[i].Status = status
			return true
		}
	}
	return false
}

func updateExecutionNode(state *protocol.ExecutionState, nodeID string, status protocol.NodeStatus, errMsg string, outputs []protocol.NodeOutputRef) {
	now := time.Now().UTC()
	for i := range state.Nodes {
		if state.Nodes[i].NodeID != nodeID {
			continue
		}
		state.Nodes[i].Status = status
		state.Nodes[i].Error = errMsg
		if status == protocol.NodeStatusRunning && state.Nodes[i].StartedAt.IsZero() {
			state.Nodes[i].StartedAt = now
		}
		if status == protocol.NodeStatusCompleted || status == protocol.NodeStatusFailed || status == protocol.NodeStatusSkipped {
			state.Nodes[i].CompletedAt = now
		}
		if len(outputs) > 0 {
			state.Nodes[i].Outputs = append([]protocol.NodeOutputRef(nil), outputs...)
		}
		if status == protocol.NodeStatusRunning || status == protocol.NodeStatusCompleted || status == protocol.NodeStatusFailed || status == protocol.NodeStatusSkipped {
			removeStaleNodeID(state, nodeID)
		}
		state.UpdatedAt = now
		return
	}
	state.Nodes = append(state.Nodes, protocol.NodeExecutionState{
		NodeID:      nodeID,
		Status:      status,
		Error:       errMsg,
		Outputs:     append([]protocol.NodeOutputRef(nil), outputs...),
		StartedAt:   now,
		CompletedAt: now,
	})
	if status == protocol.NodeStatusRunning || status == protocol.NodeStatusCompleted || status == protocol.NodeStatusFailed || status == protocol.NodeStatusSkipped {
		removeStaleNodeID(state, nodeID)
	}
	state.UpdatedAt = now
}

func resetExecutionNode(state *protocol.ExecutionState, nodeID string) {
	now := time.Now().UTC()
	for i := range state.Nodes {
		if state.Nodes[i].NodeID != nodeID {
			continue
		}
		state.Nodes[i].Status = protocol.NodeStatusPending
		state.Nodes[i].Error = ""
		state.Nodes[i].Outputs = nil
		state.Nodes[i].StartedAt = time.Time{}
		state.Nodes[i].CompletedAt = time.Time{}
		state.UpdatedAt = now
		return
	}
	state.Nodes = append(state.Nodes, protocol.NodeExecutionState{
		NodeID: nodeID,
		Status: protocol.NodeStatusPending,
	})
	state.UpdatedAt = now
}

func readyNodeIDs(dag protocol.PlanDAG) []string {
	ids := make([]string, 0, len(dag.Nodes))
	for _, node := range dag.Nodes {
		if node.Status == protocol.NodeStatusReady {
			ids = append(ids, node.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func selectBatch(dag protocol.PlanDAG) []string {
	ready := readyNodeIDs(dag)
	if len(ready) > maxParallelNodes {
		ready = ready[:maxParallelNodes]
	}
	return ready
}

func selectBatchFromSet(dag protocol.PlanDAG, allowed map[string]struct{}) []string {
	if len(allowed) == 0 {
		return nil
	}
	ready := make([]string, 0, len(allowed))
	for _, node := range dag.Nodes {
		if node.Status != protocol.NodeStatusReady {
			continue
		}
		if _, ok := allowed[node.ID]; !ok {
			continue
		}
		ready = append(ready, node.ID)
	}
	sort.Strings(ready)
	if len(ready) > maxParallelNodes {
		ready = ready[:maxParallelNodes]
	}
	return ready
}

func addStaleNodeID(state *protocol.ExecutionState, nodeID string) {
	for _, existing := range state.StaleNodeIDs {
		if existing == nodeID {
			return
		}
	}
	state.StaleNodeIDs = append(state.StaleNodeIDs, nodeID)
	sort.Strings(state.StaleNodeIDs)
	state.UpdatedAt = time.Now().UTC()
}

func removeStaleNodeID(state *protocol.ExecutionState, nodeID string) {
	if len(state.StaleNodeIDs) == 0 {
		return
	}
	filtered := state.StaleNodeIDs[:0]
	removed := false
	for _, existing := range state.StaleNodeIDs {
		if existing == nodeID {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if removed {
		state.StaleNodeIDs = filtered
		state.UpdatedAt = time.Now().UTC()
	}
}

func applyDagPatch(dag *protocol.PlanDAG, state *protocol.ExecutionState, patch protocol.DagPatch) error {
	if patch.Finalize {
		state.Finalized = true
	}
	for _, node := range patch.AddNodes {
		if _, ok := dagNode(*dag, node.ID); ok {
			continue
		}
		if node.Status == "" {
			node.Status = protocol.NodeStatusPending
		}
		dag.Nodes = append(dag.Nodes, node)
		updateExecutionNode(state, node.ID, node.Status, "", nil)
	}
	for _, edge := range patch.AddEdges {
		if !hasEdge(*dag, edge) {
			dag.Edges = append(dag.Edges, edge)
		}
		addDependsOn(dag, edge.To, edge.From)
	}
	if len(patch.RemoveEdges) > 0 {
		filtered := dag.Edges[:0]
		for _, edge := range dag.Edges {
			if containsEdge(patch.RemoveEdges, edge) {
				removeDependsOn(dag, edge.To, edge.From)
				continue
			}
			filtered = append(filtered, edge)
		}
		dag.Edges = filtered
	}
	if len(patch.RemoveNodes) > 0 {
		filtered := dag.Nodes[:0]
		removeSet := make(map[string]struct{}, len(patch.RemoveNodes))
		for _, id := range patch.RemoveNodes {
			removeSet[id] = struct{}{}
		}
		for _, node := range dag.Nodes {
			if _, ok := removeSet[node.ID]; ok {
				if node.Status == protocol.NodeStatusCompleted {
					return fmt.Errorf("cannot remove completed node: %s", node.ID)
				}
				continue
			}
			filtered = append(filtered, node)
		}
		dag.Nodes = filtered
		filteredEdges := dag.Edges[:0]
		for _, edge := range dag.Edges {
			if _, ok := removeSet[edge.From]; ok {
				continue
			}
			if _, ok := removeSet[edge.To]; ok {
				continue
			}
			filteredEdges = append(filteredEdges, edge)
		}
		dag.Edges = filteredEdges
	}
	for _, id := range patch.MarkSkipped {
		setNodeStatus(dag, id, protocol.NodeStatusSkipped)
		updateExecutionNode(state, id, protocol.NodeStatusSkipped, "", nil)
	}
	for _, id := range patch.MarkComplete {
		setNodeStatus(dag, id, protocol.NodeStatusCompleted)
		updateExecutionNode(state, id, protocol.NodeStatusCompleted, "", nil)
	}
	if err := validateDAG(*dag); err != nil {
		return err
	}
	refreshReadyNodes(dag)
	state.UpdatedAt = time.Now().UTC()
	return nil
}

func validateDAG(dag protocol.PlanDAG) error {
	ids := make(map[string]struct{}, len(dag.Nodes))
	for _, node := range dag.Nodes {
		if _, ok := ids[node.ID]; ok {
			return fmt.Errorf("duplicate node id: %s", node.ID)
		}
		ids[node.ID] = struct{}{}
	}
	for _, edge := range dag.Edges {
		if _, ok := ids[edge.From]; !ok {
			return fmt.Errorf("missing edge source: %s", edge.From)
		}
		if _, ok := ids[edge.To]; !ok {
			return fmt.Errorf("missing edge target: %s", edge.To)
		}
	}
	if len(topoSortedNodeIDs(dag)) != len(dag.Nodes) {
		return fmt.Errorf("dag contains a cycle")
	}
	return nil
}

func hasEdge(dag protocol.PlanDAG, target protocol.PlanEdge) bool {
	for _, edge := range dag.Edges {
		if edge == target {
			return true
		}
	}
	return false
}

func containsEdge(edges []protocol.PlanEdge, target protocol.PlanEdge) bool {
	for _, edge := range edges {
		if edge == target {
			return true
		}
	}
	return false
}

func addDependsOn(dag *protocol.PlanDAG, nodeID, depID string) {
	for i := range dag.Nodes {
		if dag.Nodes[i].ID != nodeID {
			continue
		}
		for _, existing := range dag.Nodes[i].DependsOn {
			if existing == depID {
				return
			}
		}
		dag.Nodes[i].DependsOn = append(dag.Nodes[i].DependsOn, depID)
		sort.Strings(dag.Nodes[i].DependsOn)
		return
	}
}

func removeDependsOn(dag *protocol.PlanDAG, nodeID, depID string) {
	for i := range dag.Nodes {
		if dag.Nodes[i].ID != nodeID {
			continue
		}
		filtered := dag.Nodes[i].DependsOn[:0]
		for _, existing := range dag.Nodes[i].DependsOn {
			if existing == depID {
				continue
			}
			filtered = append(filtered, existing)
		}
		dag.Nodes[i].DependsOn = filtered
		return
	}
}

func goalNeedsMath(goal string) bool {
	goal = strings.ToLower(strings.TrimSpace(goal))
	if goal == "" {
		return false
	}
	for _, token := range []string{"equation", "proof", "derivation", "formula", "appendix", "section", "公式", "证明", "推导", "附录", "某一节"} {
		if strings.Contains(goal, token) {
			return true
		}
	}
	return false
}

func goalNeedsWebResearch(goal string) bool {
	goal = strings.ToLower(strings.TrimSpace(goal))
	if goal == "" {
		return false
	}
	for _, token := range []string{"related work", "landscape", "latest", "trend", "external", "outside", "最新", "相关工作", "外部", "行业", "社区"} {
		if strings.Contains(goal, token) {
			return true
		}
	}
	return false
}

func fallbackGoal(goal string) string {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return "跨论文综合对比"
	}
	return goal
}
