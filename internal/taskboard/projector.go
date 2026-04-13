package taskboard

import (
	"sort"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func Build(meta protocol.SessionMeta, plan *protocol.PlanResult, execution *protocol.ExecutionState, artifacts []protocol.ArtifactManifest, workspaces []protocol.PaperWorkspace, skillRuns []protocol.SkillRunRecord) *protocol.TaskBoard {
	if (plan == nil || len(plan.DAG.Nodes) == 0) && len(skillRuns) == 0 {
		return nil
	}

	nodeCount := 0
	if plan != nil {
		nodeCount = len(plan.DAG.Nodes)
	}
	execByNodeID := make(map[string]protocol.NodeExecutionState, nodeCount)
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
	tasks := make([]protocol.TaskCard, 0, nodeCount+len(skillRuns))
	if plan != nil {
		for _, nodeID := range topoSortedNodeIDs(plan.DAG) {
			node, ok := dagNode(plan.DAG, nodeID)
			if !ok {
				continue
			}
			groupID, groupKind := taskGroupID(node)
			groupTitle, paperIDs := taskGroupTitle(node, groupKind, workspaceByPaperID)
			ensureGroup(groupMap, &groupOrder, groupID, groupKind, groupTitle, paperIDs)

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
			groupMap[groupID].TaskIDs = append(groupMap[groupID].TaskIDs, task.TaskID)
		}
	}
	for _, run := range skillRuns {
		groupID, groupKind, groupTitle, paperIDs := skillGroup(run, workspaceByPaperID)
		ensureGroup(groupMap, &groupOrder, groupID, groupKind, groupTitle, paperIDs)
		task := protocol.TaskCard{
			TaskID:           run.RunID,
			NodeID:           run.RunID,
			Kind:             skillNodeKind(run.SkillName),
			Title:            skillTaskTitle(run, workspaceByPaperID),
			Description:      skillTaskDescription(run),
			PaperIDs:         append([]string(nil), paperIDs...),
			GroupID:          groupID,
			Status:           skillTaskStatus(run.Status),
			ArtifactIDs:      skillArtifactIDs(run),
			Error:            strings.TrimSpace(run.Error),
			AvailableActions: []protocol.TaskAction{{Type: protocol.TaskActionInspect, Label: "Inspect"}},
		}
		tasks = append(tasks, task)
		groupMap[groupID].TaskIDs = append(groupMap[groupID].TaskIDs, task.TaskID)
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
	for _, run := range skillRuns {
		if run.UpdatedAt.After(updatedAt) {
			updatedAt = run.UpdatedAt
		}
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	planID := "skills"
	goal := skillsOnlyGoal(meta)
	if plan != nil {
		planID = plan.PlanID
		goal = plan.Goal
	}

	return &protocol.TaskBoard{
		PlanID:    planID,
		Goal:      goal,
		Groups:    groups,
		Tasks:     tasks,
		UpdatedAt: updatedAt,
	}
}

func ensureGroup(groupMap map[string]*protocol.TaskGroup, groupOrder *[]string, groupID, kind, title string, paperIDs []string) {
	if _, exists := groupMap[groupID]; exists {
		return
	}
	groupMap[groupID] = &protocol.TaskGroup{
		GroupID:  groupID,
		Kind:     kind,
		Title:    title,
		PaperIDs: append([]string(nil), paperIDs...),
	}
	*groupOrder = append(*groupOrder, groupID)
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

func skillGroup(run protocol.SkillRunRecord, workspaceByPaperID map[string]protocol.PaperWorkspace) (groupID string, kind string, title string, paperIDs []string) {
	if run.TargetKind == protocol.SkillTargetKindComparison {
		return "comparison", "comparison", "Comparison", nil
	}
	groupID = "paper:" + run.TargetID
	if workspace, ok := workspaceByPaperID[run.TargetID]; ok {
		if workspace.Digest != nil && strings.TrimSpace(workspace.Digest.Title) != "" {
			return groupID, "paper", workspace.Digest.Title, []string{run.TargetID}
		}
		if workspace.Source != nil && strings.TrimSpace(workspace.Source.Inspection.Title) != "" {
			return groupID, "paper", workspace.Source.Inspection.Title, []string{run.TargetID}
		}
	}
	return groupID, "paper", run.TargetID, []string{run.TargetID}
}

func skillNodeKind(name protocol.SkillName) protocol.NodeKind {
	switch name {
	case protocol.SkillNameReviewer:
		return protocol.NodeKindReviewerSkill
	case protocol.SkillNameEquationExplain:
		return protocol.NodeKindEquationExplain
	case protocol.SkillNameRelatedWorkMap:
		return protocol.NodeKindRelatedWorkMap
	case protocol.SkillNameCompareRefinement:
		return protocol.NodeKindCompareRefinement
	default:
		return protocol.NodeKindReviewerSkill
	}
}

func skillTaskTitle(run protocol.SkillRunRecord, workspaceByPaperID map[string]protocol.PaperWorkspace) string {
	if strings.TrimSpace(run.Title) != "" {
		return run.Title
	}
	target := run.TargetID
	if run.TargetKind == protocol.SkillTargetKindComparison {
		target = "comparison"
	} else if workspace, ok := workspaceByPaperID[run.TargetID]; ok {
		if workspace.Digest != nil && strings.TrimSpace(workspace.Digest.Title) != "" {
			target = workspace.Digest.Title
		} else if workspace.Source != nil && strings.TrimSpace(workspace.Source.Inspection.Title) != "" {
			target = workspace.Source.Inspection.Title
		}
	}
	switch run.SkillName {
	case protocol.SkillNameReviewer:
		return "Reviewer pass for " + target
	case protocol.SkillNameEquationExplain:
		return "Equation explain for " + target
	case protocol.SkillNameRelatedWorkMap:
		return "Related work map for " + target
	case protocol.SkillNameCompareRefinement:
		return "Compare refinement"
	default:
		return run.Title
	}
}

func skillTaskDescription(run protocol.SkillRunRecord) string {
	if trimmed := strings.TrimSpace(run.Summary); trimmed != "" {
		return trimmed
	}
	switch run.SkillName {
	case protocol.SkillNameReviewer:
		return "Structured reviewer-style assessment for a single paper."
	case protocol.SkillNameEquationExplain:
		return "Focused explanation of equations, assumptions, and failure modes."
	case protocol.SkillNameRelatedWorkMap:
		return "Document-grounded map of related methods, comparison axes, and gaps."
	case protocol.SkillNameCompareRefinement:
		return "Refined multi-paper decision frame and follow-up checks."
	default:
		return "Research skill run."
	}
}

func skillTaskStatus(status protocol.SkillRunStatus) protocol.TaskStatus {
	switch status {
	case protocol.SkillRunStatusRunning:
		return protocol.TaskStatusRunning
	case protocol.SkillRunStatusFailed:
		return protocol.TaskStatusFailed
	default:
		return protocol.TaskStatusCompleted
	}
}

func skillArtifactIDs(run protocol.SkillRunRecord) []string {
	if strings.TrimSpace(run.ArtifactID) == "" {
		return nil
	}
	return []string{run.ArtifactID}
}

func skillsOnlyGoal(meta protocol.SessionMeta) string {
	if trimmed := strings.TrimSpace(meta.LastTask); trimmed != "" {
		return trimmed
	}
	return "Research skills"
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
