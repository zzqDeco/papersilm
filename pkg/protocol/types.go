package protocol

import "time"

type SourceType string

const (
	SourceTypeLocalPDF         SourceType = "local_pdf"
	SourceTypePaperID          SourceType = "paper_id"
	SourceTypeArxivAbs         SourceType = "arxiv_abs"
	SourceTypeArxivPDF         SourceType = "arxiv_pdf"
	SourceTypeAlphaXivOverview SourceType = "alphaxiv_overview"
	SourceTypeAlphaXivAbs      SourceType = "alphaxiv_abs"
)

type SourceStatus string

const (
	SourceStatusAttached  SourceStatus = "attached"
	SourceStatusInspected SourceStatus = "inspected"
	SourceStatusDistilled SourceStatus = "distilled"
	SourceStatusFailed    SourceStatus = "failed"
)

type PermissionMode string

const (
	PermissionModePlan    PermissionMode = "plan"
	PermissionModeConfirm PermissionMode = "confirm"
	PermissionModeAuto    PermissionMode = "auto"
)

type OutputFormat string

const (
	OutputFormatText       OutputFormat = "text"
	OutputFormatJSON       OutputFormat = "json"
	OutputFormatStreamJSON OutputFormat = "stream-json"
)

type ArtifactFormat string

const (
	ArtifactFormatMarkdown ArtifactFormat = "md"
	ArtifactFormatJSON     ArtifactFormat = "json"
)

type ContentSource string

const (
	ContentSourceUnknown          ContentSource = "unknown"
	ContentSourceAlphaXivOverview ContentSource = "alphaxiv_overview"
	ContentSourceAlphaXivFullText ContentSource = "alphaxiv_full_text"
	ContentSourceArxivPDFFallback ContentSource = "arxiv_pdf_fallback"
)

type WorkerProfile string

const (
	WorkerProfileSupervisor        WorkerProfile = "supervisor"
	WorkerProfilePaperSummary      WorkerProfile = "paper_summary_worker"
	WorkerProfileExperiment        WorkerProfile = "experiment_worker"
	WorkerProfileMathReasoner      WorkerProfile = "math_reasoner_worker"
	WorkerProfileWebResearch       WorkerProfile = "web_research_worker"
	WorkerProfileMethodCompare     WorkerProfile = "method_compare_worker"
	WorkerProfileExperimentCompare WorkerProfile = "experiment_compare_worker"
	WorkerProfileResultsCompare    WorkerProfile = "results_compare_worker"
)

type NodeKind string

const (
	NodeKindPaperSummary      NodeKind = "paper_summary"
	NodeKindExperiment        NodeKind = "experiment"
	NodeKindMathReasoner      NodeKind = "math_reasoner"
	NodeKindWebResearch       NodeKind = "web_research"
	NodeKindMergeDigest       NodeKind = "merge_digest"
	NodeKindMethodCompare     NodeKind = "method_compare"
	NodeKindExperimentCompare NodeKind = "experiment_compare"
	NodeKindResultsCompare    NodeKind = "results_compare"
	NodeKindFinalSynthesis    NodeKind = "final_synthesis"
	NodeKindWorkspaceInspect  NodeKind = "workspace_inspect"
	NodeKindWorkspaceSearch   NodeKind = "workspace_search"
	NodeKindWorkspaceEdit     NodeKind = "workspace_edit"
	NodeKindWorkspaceCommand  NodeKind = "workspace_command"
	NodeKindReviewerSkill     NodeKind = "reviewer_skill"
	NodeKindEquationExplain   NodeKind = "equation_explain_skill"
	NodeKindRelatedWorkMap    NodeKind = "related_work_map_skill"
	NodeKindCompareRefinement NodeKind = "compare_refinement_skill"
)

type NodeStatus string

const (
	NodeStatusPending   NodeStatus = "pending"
	NodeStatusReady     NodeStatus = "ready"
	NodeStatusRunning   NodeStatus = "running"
	NodeStatusCompleted NodeStatus = "completed"
	NodeStatusFailed    NodeStatus = "failed"
	NodeStatusSkipped   NodeStatus = "skipped"
)

type SourceInspection struct {
	PageCount          int      `json:"page_count"`
	Title              string   `json:"title,omitempty"`
	SectionHints       []string `json:"section_hints,omitempty"`
	ExtractableText    bool     `json:"extractable_text"`
	Comparable         bool     `json:"comparable"`
	FailureReason      string   `json:"failure_reason,omitempty"`
	SampleIntroduction string   `json:"sample_introduction,omitempty"`
}

type PaperRef struct {
	PaperID                string           `json:"paper_id"`
	URI                    string           `json:"uri"`
	LocalPath              string           `json:"local_path,omitempty"`
	ResolvedPaperID        string           `json:"resolved_paper_id,omitempty"`
	SourceType             SourceType       `json:"source_type"`
	Label                  string           `json:"label,omitempty"`
	Status                 SourceStatus     `json:"status"`
	PreferredContentSource ContentSource    `json:"preferred_content_source,omitempty"`
	ContentProvenance      ContentSource    `json:"content_provenance,omitempty"`
	Inspection             SourceInspection `json:"inspection,omitempty"`
}

type AnchorKind string

const (
	AnchorKindPage    AnchorKind = "page"
	AnchorKindSnippet AnchorKind = "snippet"
	AnchorKindSection AnchorKind = "section"
)

type AnchorRef struct {
	Kind    AnchorKind `json:"kind"`
	Page    int        `json:"page,omitempty"`
	Snippet string     `json:"snippet,omitempty"`
	Section string     `json:"section,omitempty"`
}

type Citation struct {
	Page    int    `json:"page"`
	Snippet string `json:"snippet,omitempty"`
}

type PaperNote struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PaperAnnotation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Anchor    AnchorRef `json:"anchor"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PaperResource struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	URI   string `json:"uri"`
}

type SimilarPaperRef struct {
	PaperID string `json:"paper_id"`
	Title   string `json:"title,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Status  string `json:"status,omitempty"`
}

type SkillName string

const (
	SkillNameReviewer          SkillName = "reviewer"
	SkillNameEquationExplain   SkillName = "equation-explain"
	SkillNameRelatedWorkMap    SkillName = "related-work-map"
	SkillNameCompareRefinement SkillName = "compare-refinement"
)

type SkillTargetKind string

const (
	SkillTargetKindPaper      SkillTargetKind = "paper"
	SkillTargetKindComparison SkillTargetKind = "comparison"
)

type SkillRunStatus string

const (
	SkillRunStatusRunning   SkillRunStatus = "running"
	SkillRunStatusCompleted SkillRunStatus = "completed"
	SkillRunStatusFailed    SkillRunStatus = "failed"
)

type SkillDescriptor struct {
	Name         SkillName       `json:"name"`
	Title        string          `json:"title"`
	Summary      string          `json:"summary"`
	TargetKind   SkillTargetKind `json:"target_kind"`
	ArtifactKind string          `json:"artifact_kind"`
}

type SkillRunRecord struct {
	RunID      string          `json:"run_id"`
	SessionID  string          `json:"session_id"`
	SkillName  SkillName       `json:"skill_name"`
	TargetKind SkillTargetKind `json:"target_kind"`
	TargetID   string          `json:"target_id"`
	PaperIDs   []string        `json:"paper_ids,omitempty"`
	ArtifactID string          `json:"artifact_id,omitempty"`
	Status     SkillRunStatus  `json:"status"`
	Title      string          `json:"title"`
	Summary    string          `json:"summary,omitempty"`
	Error      string          `json:"error,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type SkillRunResult struct {
	Session    SessionSnapshot   `json:"session"`
	Descriptor SkillDescriptor   `json:"descriptor"`
	Run        SkillRunRecord    `json:"run"`
	Artifact   *ArtifactManifest `json:"artifact,omitempty"`
}

type KeyResult struct {
	Claim     string     `json:"claim"`
	Value     string     `json:"value,omitempty"`
	Dataset   string     `json:"dataset,omitempty"`
	Metric    string     `json:"metric,omitempty"`
	Baseline  string     `json:"baseline,omitempty"`
	Citations []Citation `json:"citations,omitempty"`
}

type PaperDigest struct {
	PaperID              string        `json:"paper_id"`
	Title                string        `json:"title"`
	Problem              string        `json:"problem"`
	OneLineSummary       string        `json:"one_line_summary"`
	MethodSummary        []string      `json:"method_summary"`
	ExperimentSummary    []string      `json:"experiment_summary"`
	KeyResults           []KeyResult   `json:"key_results"`
	Conclusions          []string      `json:"conclusions"`
	Limitations          []string      `json:"limitations"`
	Citations            []Citation    `json:"citations"`
	Markdown             string        `json:"markdown,omitempty"`
	ArtifactID           string        `json:"artifact_id,omitempty"`
	Language             string        `json:"language"`
	Style                string        `json:"style"`
	ContentProvenance    ContentSource `json:"content_provenance"`
	GeneratedAt          time.Time     `json:"generated_at"`
	HasBackgroundOmitted bool          `json:"has_background_omitted"`
}

type ComparisonMatrixRow struct {
	Dimension string            `json:"dimension"`
	Summary   string            `json:"summary,omitempty"`
	Values    map[string]string `json:"values"`
}

type ComparisonDigest struct {
	PaperIDs         []string              `json:"paper_ids"`
	Goal             string                `json:"goal"`
	PaperSummaries   []PaperDigest         `json:"paper_summaries"`
	MethodMatrix     []ComparisonMatrixRow `json:"method_matrix"`
	ExperimentMatrix []ComparisonMatrixRow `json:"experiment_matrix"`
	ResultMatrix     []ComparisonMatrixRow `json:"result_matrix"`
	Synthesis        []string              `json:"synthesis"`
	Limitations      []string              `json:"limitations"`
	Markdown         string                `json:"markdown,omitempty"`
	ArtifactID       string                `json:"artifact_id,omitempty"`
	Language         string                `json:"language"`
	Style            string                `json:"style"`
	GeneratedAt      time.Time             `json:"generated_at"`
}

type PaperWorkspace struct {
	PaperID     string            `json:"paper_id"`
	Source      *PaperRef         `json:"source,omitempty"`
	Digest      *PaperDigest      `json:"digest,omitempty"`
	Notes       []PaperNote       `json:"notes,omitempty"`
	Annotations []PaperAnnotation `json:"annotations,omitempty"`
	Resources   []PaperResource   `json:"resources,omitempty"`
	Similar     []SimilarPaperRef `json:"similar,omitempty"`
	SkillRuns   []SkillRunRecord  `json:"skill_runs,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type WorkspaceFileKind string

const (
	WorkspaceFileKindText   WorkspaceFileKind = "text"
	WorkspaceFileKindCode   WorkspaceFileKind = "code"
	WorkspaceFileKindPaper  WorkspaceFileKind = "paper"
	WorkspaceFileKindConfig WorkspaceFileKind = "config"
	WorkspaceFileKindOther  WorkspaceFileKind = "other"
)

type WorkspaceFile struct {
	Path           string            `json:"path"`
	AbsolutePath   string            `json:"absolute_path,omitempty"`
	Kind           WorkspaceFileKind `json:"kind"`
	SizeBytes      int64             `json:"size_bytes"`
	ModifiedAt     time.Time         `json:"modified_at"`
	PaperCandidate bool              `json:"paper_candidate,omitempty"`
}

type WorkspaceSummary struct {
	WorkspaceID    string    `json:"workspace_id"`
	Root           string    `json:"root"`
	Name           string    `json:"name"`
	FileCount      int       `json:"file_count"`
	TextFileCount  int       `json:"text_file_count"`
	PaperFileCount int       `json:"paper_file_count"`
	SessionCount   int       `json:"session_count"`
	IndexedAt      time.Time `json:"indexed_at"`
}

type WorkspaceSearchHit struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

type WorkspaceCommandRecord struct {
	Command     string    `json:"command"`
	Cwd         string    `json:"cwd"`
	ExitCode    int       `json:"exit_code"`
	Stdout      string    `json:"stdout,omitempty"`
	Stderr      string    `json:"stderr,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

const DefaultWorkspaceID = "workspace"

type TaskStatus string

const (
	TaskStatusBlocked          TaskStatus = "blocked"
	TaskStatusReady            TaskStatus = "ready"
	TaskStatusAwaitingApproval TaskStatus = "awaiting_approval"
	TaskStatusRunning          TaskStatus = "running"
	TaskStatusCompleted        TaskStatus = "completed"
	TaskStatusFailed           TaskStatus = "failed"
	TaskStatusStale            TaskStatus = "stale"
	TaskStatusSkipped          TaskStatus = "skipped"
)

type TaskActionType string

const (
	TaskActionInspect TaskActionType = "inspect"
	TaskActionRun     TaskActionType = "run"
	TaskActionApprove TaskActionType = "approve"
	TaskActionReject  TaskActionType = "reject"
)

type TaskAction struct {
	Type  TaskActionType `json:"type"`
	Label string         `json:"label,omitempty"`
}

type TaskCard struct {
	TaskID           string       `json:"task_id"`
	NodeID           string       `json:"node_id"`
	Kind             NodeKind     `json:"kind"`
	Title            string       `json:"title"`
	Description      string       `json:"description,omitempty"`
	PaperIDs         []string     `json:"paper_ids,omitempty"`
	GroupID          string       `json:"group_id"`
	Status           TaskStatus   `json:"status"`
	DependsOn        []string     `json:"depends_on,omitempty"`
	Produces         []string     `json:"produces,omitempty"`
	ArtifactIDs      []string     `json:"artifact_ids,omitempty"`
	Error            string       `json:"error,omitempty"`
	AvailableActions []TaskAction `json:"available_actions,omitempty"`
}

type TaskGroup struct {
	GroupID  string   `json:"group_id"`
	Kind     string   `json:"kind"`
	Title    string   `json:"title"`
	PaperIDs []string `json:"paper_ids,omitempty"`
	TaskIDs  []string `json:"task_ids,omitempty"`
}

type TaskBoard struct {
	PlanID    string      `json:"plan_id"`
	Goal      string      `json:"goal"`
	Groups    []TaskGroup `json:"groups,omitempty"`
	Tasks     []TaskCard  `json:"tasks,omitempty"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type PlanNode struct {
	ID            string        `json:"id"`
	Kind          NodeKind      `json:"kind"`
	Goal          string        `json:"goal"`
	PaperIDs      []string      `json:"paper_ids,omitempty"`
	WorkerProfile WorkerProfile `json:"worker_profile"`
	DependsOn     []string      `json:"depends_on,omitempty"`
	Produces      []string      `json:"produces,omitempty"`
	Required      bool          `json:"required"`
	Status        NodeStatus    `json:"status"`
	ParallelGroup string        `json:"parallel_group,omitempty"`
}

type PlanEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type PlanDAG struct {
	Nodes []PlanNode `json:"nodes"`
	Edges []PlanEdge `json:"edges,omitempty"`
}

type NodeOutputRef struct {
	NodeID     string         `json:"node_id"`
	Kind       string         `json:"kind"`
	Ref        string         `json:"ref,omitempty"`
	ArtifactID string         `json:"artifact_id,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type DagPatch struct {
	AddNodes     []PlanNode `json:"add_nodes,omitempty"`
	AddEdges     []PlanEdge `json:"add_edges,omitempty"`
	RemoveNodes  []string   `json:"remove_nodes,omitempty"`
	RemoveEdges  []PlanEdge `json:"remove_edges,omitempty"`
	MarkSkipped  []string   `json:"mark_skipped,omitempty"`
	MarkComplete []string   `json:"mark_complete,omitempty"`
	Finalize     bool       `json:"finalize,omitempty"`
	Reason       string     `json:"reason,omitempty"`
}

type BatchStatus string

const (
	BatchStatusPending   BatchStatus = "pending"
	BatchStatusRunning   BatchStatus = "running"
	BatchStatusCompleted BatchStatus = "completed"
)

type ExecutionBatch struct {
	BatchID     string      `json:"batch_id"`
	NodeIDs     []string    `json:"node_ids"`
	Status      BatchStatus `json:"status"`
	StartedAt   time.Time   `json:"started_at"`
	CompletedAt time.Time   `json:"completed_at,omitempty"`
}

type NodeExecutionState struct {
	NodeID        string          `json:"node_id"`
	WorkerProfile WorkerProfile   `json:"worker_profile"`
	Status        NodeStatus      `json:"status"`
	Error         string          `json:"error,omitempty"`
	Outputs       []NodeOutputRef `json:"outputs,omitempty"`
	StartedAt     time.Time       `json:"started_at,omitempty"`
	CompletedAt   time.Time       `json:"completed_at,omitempty"`
}

type ExecutionState struct {
	PlanID         string               `json:"plan_id"`
	CurrentBatchID string               `json:"current_batch_id,omitempty"`
	PendingNodeIDs []string             `json:"pending_node_ids,omitempty"`
	StaleNodeIDs   []string             `json:"stale_node_ids,omitempty"`
	Nodes          []NodeExecutionState `json:"nodes,omitempty"`
	Outputs        []NodeOutputRef      `json:"outputs,omitempty"`
	BatchHistory   []ExecutionBatch     `json:"batch_history,omitempty"`
	Finalized      bool                 `json:"finalized"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

type PlanStep struct {
	ID               string   `json:"id"`
	Tool             string   `json:"tool"`
	PaperIDs         []string `json:"paper_ids,omitempty"`
	Goal             string   `json:"goal"`
	ExpectedArtifact string   `json:"expected_artifact"`
}

type PlanResult struct {
	PlanID           string     `json:"plan_id"`
	Goal             string     `json:"goal"`
	SourceSummary    []PaperRef `json:"source_summary"`
	DAG              PlanDAG    `json:"dag"`
	Steps            []PlanStep `json:"steps"`
	WillCompare      bool       `json:"will_compare"`
	Risks            []string   `json:"risks"`
	ApprovalRequired bool       `json:"approval_required"`
	TaskBoard        *TaskBoard `json:"task_board,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type ApprovalRequest struct {
	PlanID          string              `json:"plan_id"`
	CheckpointID    string              `json:"checkpoint_id"`
	InterruptID     string              `json:"interrupt_id"`
	PendingNodeIDs  []string            `json:"pending_node_ids,omitempty"`
	Summary         string              `json:"summary"`
	RequiresInput   bool                `json:"requires_input"`
	CreatedAt       time.Time           `json:"created_at"`
	Mode            string              `json:"mode,omitempty"`
	ActiveRequestID string              `json:"active_request_id,omitempty"`
	Requests        []PermissionRequest `json:"requests,omitempty"`
}

type PermissionPreview struct {
	Kind            string `json:"kind,omitempty"`
	Summary         string `json:"summary,omitempty"`
	Diff            string `json:"diff,omitempty"`
	OldContentHash  string `json:"old_content_hash,omitempty"`
	NewContent      string `json:"new_content,omitempty"`
	CommandPrefix   string `json:"command_prefix,omitempty"`
	ConflictMessage string `json:"conflict_message,omitempty"`
}

type PermissionOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Feedback    string `json:"feedback,omitempty"`
}

type PermissionRequest struct {
	RequestID  string             `json:"request_id"`
	SessionID  string             `json:"session_id,omitempty"`
	PlanID     string             `json:"plan_id,omitempty"`
	NodeID     string             `json:"node_id,omitempty"`
	Tool       string             `json:"tool"`
	Operation  string             `json:"operation,omitempty"`
	Title      string             `json:"title"`
	Subtitle   string             `json:"subtitle,omitempty"`
	Question   string             `json:"question"`
	Summary    string             `json:"summary,omitempty"`
	TargetPath string             `json:"target_path,omitempty"`
	Command    string             `json:"command,omitempty"`
	Preview    PermissionPreview  `json:"preview,omitempty"`
	Options    []PermissionOption `json:"options"`
	CreatedAt  time.Time          `json:"created_at"`
}

type PermissionDecision struct {
	RequestID string `json:"request_id"`
	Value     string `json:"value"`
	Feedback  string `json:"feedback,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

type PermissionRule struct {
	RuleID        string    `json:"rule_id"`
	Tool          string    `json:"tool"`
	Operation     string    `json:"operation,omitempty"`
	Scope         string    `json:"scope"`
	TargetPath    string    `json:"target_path,omitempty"`
	Directory     string    `json:"directory,omitempty"`
	CommandPrefix string    `json:"command_prefix,omitempty"`
	NodeKind      string    `json:"node_kind,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type PlanProgressStatus string

const (
	PlanProgressStarted   PlanProgressStatus = "started"
	PlanProgressCompleted PlanProgressStatus = "completed"
	PlanProgressFailed    PlanProgressStatus = "failed"
	PlanProgressReplanned PlanProgressStatus = "replanned"
)

type PlanProgress struct {
	PlanID        string             `json:"plan_id"`
	StepID        string             `json:"step_id,omitempty"`
	NodeID        string             `json:"node_id,omitempty"`
	Tool          string             `json:"tool,omitempty"`
	WorkerProfile WorkerProfile      `json:"worker_profile,omitempty"`
	BatchID       string             `json:"batch_id,omitempty"`
	Status        PlanProgressStatus `json:"status"`
	Message       string             `json:"message,omitempty"`
	Error         string             `json:"error,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
}

type ArtifactManifest struct {
	ArtifactID string                 `json:"artifact_id"`
	SessionID  string                 `json:"session_id"`
	Kind       string                 `json:"kind"`
	Source     string                 `json:"source,omitempty"`
	Language   string                 `json:"language"`
	Format     ArtifactFormat         `json:"format"`
	Paths      map[string]string      `json:"paths"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

type SessionState string

const (
	SessionStateIdle             SessionState = "idle"
	SessionStateSourceAttached   SessionState = "source_attached"
	SessionStatePlanned          SessionState = "planned"
	SessionStateAwaitingApproval SessionState = "awaiting_approval"
	SessionStateRunning          SessionState = "running"
	SessionStateCompleted        SessionState = "completed"
	SessionStateFailed           SessionState = "failed"
)

type SessionMeta struct {
	SessionID          string         `json:"session_id"`
	Name               string         `json:"name,omitempty"`
	State              SessionState   `json:"state"`
	PermissionMode     PermissionMode `json:"permission_mode"`
	WorkspaceRoot      string         `json:"workspace_root,omitempty"`
	WorkspaceID        string         `json:"workspace_id,omitempty"`
	ProviderProfile    string         `json:"provider_profile,omitempty"`
	Model              string         `json:"model,omitempty"`
	Language           string         `json:"language"`
	Style              string         `json:"style"`
	LastTask           string         `json:"last_task,omitempty"`
	ApprovalPending    bool           `json:"approval_pending"`
	ActivePlanID       string         `json:"active_plan_id,omitempty"`
	ActiveCheckpointID string         `json:"active_checkpoint_id,omitempty"`
	PendingInterruptID string         `json:"pending_interrupt_id,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type SessionSnapshot struct {
	Meta           SessionMeta        `json:"meta"`
	Workspace      *WorkspaceSummary  `json:"workspace,omitempty"`
	Sources        []PaperRef         `json:"sources"`
	Plan           *PlanResult        `json:"plan,omitempty"`
	Approval       *ApprovalRequest   `json:"approval,omitempty"`
	TaskBoard      *TaskBoard         `json:"task_board,omitempty"`
	Execution      *ExecutionState    `json:"execution,omitempty"`
	Digests        []PaperDigest      `json:"digests,omitempty"`
	Compare        *ComparisonDigest  `json:"comparison,omitempty"`
	Artifacts      []ArtifactManifest `json:"artifacts,omitempty"`
	SkillRuns      []SkillRunRecord   `json:"skill_runs,omitempty"`
	SkillArtifacts []ArtifactManifest `json:"skill_artifacts,omitempty"`
	Workspaces     []PaperWorkspace   `json:"workspaces,omitempty"`
}

type ClientRequest struct {
	Task           string         `json:"task"`
	Sources        []string       `json:"sources,omitempty"`
	PermissionMode PermissionMode `json:"permission_mode"`
	Language       string         `json:"language"`
	Style          string         `json:"style"`
	SessionID      string         `json:"session_id,omitempty"`
}

type RunResult struct {
	Session    SessionSnapshot    `json:"session"`
	Plan       *PlanResult        `json:"plan,omitempty"`
	Approval   *ApprovalRequest   `json:"approval,omitempty"`
	Digests    []PaperDigest      `json:"digests,omitempty"`
	Comparison *ComparisonDigest  `json:"comparison,omitempty"`
	Artifacts  []ArtifactManifest `json:"artifacts,omitempty"`
	Response   string             `json:"response,omitempty"`
}
