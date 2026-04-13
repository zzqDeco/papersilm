package taskboard

import (
	"sort"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func Build(meta protocol.SessionMeta, plan *protocol.PlanResult, execution *protocol.ExecutionState, artifacts []protocol.ArtifactManifest, workspaces []protocol.PaperWorkspace) *protocol.TaskBoard {
	if plan == nil || len(plan.DAG.Nodes) == 0 {
		return nil
	}

	execByNodeID := make(map[string]protocol.NodeExecutionState, len(plan.DAG.Nodes))
	stale := make(map[string]struct{})
	pendingApproval := make(map[string]struct{})
	if execution != nil {
		for _, node := range execution.Nodes {
			execByNodeID[node.NodeID] = node
		}
		for _, nodeID := range execution.StaleNodeIDs {
			stale[nodeID] = struct{}{}
		}
		for _, nodeID := range execution.PendingNodeIDs {
			pendingApproval[nodeID] = struct{}{}
		}
	}

	workspaceByPaperID := make(map[string]protocol.PaperWorkspace, len(workspaces))
	for _, workspace := range workspaces {
		workspaceByPaperID[workspace.PaperID] = workspace
	}

	groupMap := map[string]*protocol.TaskGroup{}
	groupOrder := make([]string, 0, len(workspaces)+1)
	tasks := make([]protocol.TaskCard, 0, len(plan.DAG.Nodes))
	for _, nodeID := range topoSortedNodeIDs(plan.DAG) {
		node, ok := dagNode(plan.DAG, nodeID)
		if !ok {
			continue
		}
		groupID, groupKind := taskGroupID(node)
		if _, exists := groupMap[groupID]; !exists {
			title, paperIDs := taskGroupTitle(node, groupKind, workspaceByPaperID)
			groupMap[groupID] = &protocol.TaskGroup{
				GroupID:  groupID,
				Kind:     groupKind,
				Title:    title,
				PaperIDs: paperIDs,
			}
			groupOrder = append(groupOrder, groupID)
		}

		taskStatus := deriveTaskStatus(meta, plan.DAG, node, execByNodeID, stale, pendingApproval)
		task := protocol.TaskCard{
			TaskID:           node.ID,
			NodeID:           node.ID,
			Kind:             node.Kind,
			Title:            taskTitle(node, workspaceByPaperID),
			Description:      taskDescription(node),
			PaperIDs:         append([]string(nil), node.PaperIDs...),
			GroupID:          groupID,
			Status:           taskStatus,
			DependsOn:        append([]string(nil), node.DependsOn...),
			Produces:         append([]string(nil), node.Produces...),
			ArtifactIDs:      artifactIDsForNode(node, execByNodeID[node.ID], artifacts),
			Error:            strings.TrimSpace(execByNodeID[node.ID].Error),
			AvailableActions: availableActions(meta, node.ID, taskStatus, pendingApproval),
		}
		tasks = append(tasks, task)
		group := groupMap[groupID]
		group.TaskIDs = append(group.TaskIDs, task.TaskID)
	}

	groups := make([]protocol.TaskGroup, 0, len(groupOrder))
	for _, groupID := range groupOrder {
		group := groupMap[groupID]
		if group == nil {
			continue
		}
		groups = append(groups, *group)
	}

	updatedAt := meta.UpdatedAt
	if execution != nil && execution.UpdatedAt.After(updatedAt) {
		updatedAt = execution.UpdatedAt
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	return &protocol.TaskBoard{
		PlanID:    plan.PlanID,
		Goal:      plan.Goal,
		Groups:    groups,
		Tasks:     tasks,
		UpdatedAt: updatedAt,
	}
}

func taskGroupID(node protocol.PlanNode) (groupID string, kind string) {
	if len(node.PaperIDs) == 1 && !isComparisonNode(node.Kind) {
		return "paper:" + node.PaperIDs[0], "paper"
	}
	return "comparison", "comparison"
}

func taskGroupTitle(node protocol.PlanNode, kind string, workspaceByPaperID map[string]protocol.PaperWorkspace) (string, []string) {
	if kind == "comparison" {
		return "Comparison", append([]string(nil), node.PaperIDs...)
	}
	paperID := node.PaperIDs[0]
	if workspace, ok := workspaceByPaperID[paperID]; ok {
		if workspace.Digest != nil && strings.TrimSpace(workspace.Digest.Title) != "" {
			return workspace.Digest.Title, []string{paperID}
		}
		if workspace.Source != nil && strings.TrimSpace(workspace.Source.Inspection.Title) != "" {
			return workspace.Source.Inspection.Title, []string{paperID}
		}
		if workspace.Source != nil && strings.TrimSpace(workspace.Source.Label) != "" {
			return workspace.Source.Label, []string{paperID}
		}
	}
	return paperID, []string{paperID}
}

func taskTitle(node protocol.PlanNode, workspaceByPaperID map[string]protocol.PaperWorkspace) string {
	label := paperLabel(node.PaperIDs, workspaceByPaperID)
	switch node.Kind {
	case protocol.NodeKindPaperSummary:
		return "Summarize " + label
	case protocol.NodeKindExperiment:
		return "Extract experiments for " + label
	case protocol.NodeKindMathReasoner:
		return "Explain equations for " + label
	case protocol.NodeKindWebResearch:
		return "Collect external context for " + label
	case protocol.NodeKindMergeDigest:
		return "Assemble digest for " + label
	case protocol.NodeKindMethodCompare:
		return "Compare methods"
	case protocol.NodeKindExperimentCompare:
		return "Compare experiments"
	case protocol.NodeKindResultsCompare:
		return "Compare results"
	case protocol.NodeKindFinalSynthesis:
		return "Write final comparison"
	default:
		if trimmed := strings.TrimSpace(node.Goal); trimmed != "" {
			return trimmed
		}
		return string(node.Kind)
	}
}

func taskDescription(node protocol.PlanNode) string {
	switch node.Kind {
	case protocol.NodeKindPaperSummary:
		return "Extract the problem framing, method outline, and main claims."
	case protocol.NodeKindExperiment:
		return "Capture datasets, setup, and reported empirical evidence."
	case protocol.NodeKindMathReasoner:
		return "Focus on equations, derivations, and mathematically dense sections."
	case protocol.NodeKindWebResearch:
		return "Gather targeted external context that complements the paper itself."
	case protocol.NodeKindMergeDigest:
		return "Merge the single-paper analyses into the durable digest artifact."
	case protocol.NodeKindMethodCompare:
		return "Align methods across papers in a structured comparison row."
	case protocol.NodeKindExperimentCompare:
		return "Align experimental setup and evaluation design across papers."
	case protocol.NodeKindResultsCompare:
		return "Align the reported outcomes and comparative claims across papers."
	case protocol.NodeKindFinalSynthesis:
		return "Produce the final multi-paper comparison artifact."
	default:
		return strings.TrimSpace(node.Goal)
	}
}

func paperLabel(paperIDs []string, workspaceByPaperID map[string]protocol.PaperWorkspace) string {
	if len(paperIDs) == 0 {
		return "paper"
	}
	if len(paperIDs) > 1 {
		return "papers"
	}
	paperID := paperIDs[0]
	if workspace, ok := workspaceByPaperID[paperID]; ok {
		if workspace.Digest != nil && strings.TrimSpace(workspace.Digest.Title) != "" {
			return workspace.Digest.Title
		}
		if workspace.Source != nil && strings.TrimSpace(workspace.Source.Inspection.Title) != "" {
			return workspace.Source.Inspection.Title
		}
	}
	return paperID
}

func deriveTaskStatus(
	meta protocol.SessionMeta,
	dag protocol.PlanDAG,
	node protocol.PlanNode,
	execByNodeID map[string]protocol.NodeExecutionState,
	stale map[string]struct{},
	pendingApproval map[string]struct{},
) protocol.TaskStatus {
	status := node.Status
	if exec, ok := execByNodeID[node.ID]; ok && exec.Status != "" {
		status = exec.Status
	}
	switch status {
	case protocol.NodeStatusRunning:
		return protocol.TaskStatusRunning
	case protocol.NodeStatusCompleted:
		return protocol.TaskStatusCompleted
	case protocol.NodeStatusFailed:
		return protocol.TaskStatusFailed
	case protocol.NodeStatusSkipped:
		return protocol.TaskStatusSkipped
	}
	if _, ok := stale[node.ID]; ok {
		return protocol.TaskStatusStale
	}
	if meta.State == protocol.SessionStateAwaitingApproval {
		if _, ok := pendingApproval[node.ID]; ok && dependenciesSatisfied(dag, execByNodeID, stale, node) {
			return protocol.TaskStatusAwaitingApproval
		}
	}
	if dependenciesSatisfied(dag, execByNodeID, stale, node) {
		return protocol.TaskStatusReady
	}
	return protocol.TaskStatusBlocked
}

func dependenciesSatisfied(
	dag protocol.PlanDAG,
	execByNodeID map[string]protocol.NodeExecutionState,
	stale map[string]struct{},
	node protocol.PlanNode,
) bool {
	if len(node.DependsOn) == 0 {
		return true
	}
	for _, depID := range node.DependsOn {
		depNode, ok := dagNode(dag, depID)
		if !ok {
			return false
		}
		depStatus := depNode.Status
		if exec, ok := execByNodeID[depID]; ok && exec.Status != "" {
			depStatus = exec.Status
		}
		if _, ok := stale[depID]; ok {
			return false
		}
		switch depStatus {
		case protocol.NodeStatusCompleted, protocol.NodeStatusSkipped:
			continue
		case protocol.NodeStatusFailed:
			if depNode.Required {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func availableActions(meta protocol.SessionMeta, nodeID string, status protocol.TaskStatus, pendingApproval map[string]struct{}) []protocol.TaskAction {
	actions := []protocol.TaskAction{{Type: protocol.TaskActionInspect, Label: "Inspect"}}
	if meta.State == protocol.SessionStateAwaitingApproval {
		if _, ok := pendingApproval[nodeID]; ok && status == protocol.TaskStatusAwaitingApproval {
			actions = append(actions,
				protocol.TaskAction{Type: protocol.TaskActionApprove, Label: "Approve"},
				protocol.TaskAction{Type: protocol.TaskActionReject, Label: "Reject"},
			)
		}
		return actions
	}
	switch status {
	case protocol.TaskStatusReady, protocol.TaskStatusFailed, protocol.TaskStatusStale, protocol.TaskStatusCompleted:
		actions = append(actions, protocol.TaskAction{Type: protocol.TaskActionRun, Label: "Run"})
	case protocol.TaskStatusAwaitingApproval:
		actions = append(actions,
			protocol.TaskAction{Type: protocol.TaskActionApprove, Label: "Approve"},
			protocol.TaskAction{Type: protocol.TaskActionReject, Label: "Reject"},
		)
	}
	return actions
}

func artifactIDsForNode(node protocol.PlanNode, exec protocol.NodeExecutionState, artifacts []protocol.ArtifactManifest) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 2)
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, output := range exec.Outputs {
		add(output.ArtifactID)
		if value, ok := output.Data["artifact_id"].(string); ok {
			add(value)
		}
	}
	for _, manifest := range artifacts {
		switch node.Kind {
		case protocol.NodeKindMergeDigest, protocol.NodeKind("distill_paper"):
			if len(node.PaperIDs) == 1 && manifest.Kind == "paper_digest" && manifest.ArtifactID == node.PaperIDs[0] {
				add(manifest.ArtifactID)
			}
		case protocol.NodeKindFinalSynthesis, protocol.NodeKind("compare_papers"):
			if manifest.Kind == "comparison_digest" && manifest.ArtifactID == "comparison" {
				add(manifest.ArtifactID)
			}
		}
	}
	sort.Strings(out)
	return out
}

func isComparisonNode(kind protocol.NodeKind) bool {
	switch kind {
	case protocol.NodeKindMethodCompare, protocol.NodeKindExperimentCompare, protocol.NodeKindResultsCompare, protocol.NodeKindFinalSynthesis, protocol.NodeKind("compare_papers"):
		return true
	default:
		return false
	}
}

func dagNode(dag protocol.PlanDAG, nodeID string) (protocol.PlanNode, bool) {
	for _, node := range dag.Nodes {
		if node.ID == nodeID {
			return node, true
		}
	}
	return protocol.PlanNode{}, false
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
		nodeID := queue[0]
		queue = queue[1:]
		out = append(out, nodeID)
		for _, next := range adj[nodeID] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
				sort.Strings(queue)
			}
		}
	}
	if len(out) != len(dag.Nodes) {
		out = out[:0]
		for _, node := range dag.Nodes {
			out = append(out, node.ID)
		}
		sort.Strings(out)
	}
	return out
}
