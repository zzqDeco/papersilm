package protocol

import "time"

type SourceType string

const (
	SourceTypeLocalPDF SourceType = "local_pdf"
	SourceTypeArxivAbs SourceType = "arxiv_abs"
	SourceTypeArxivPDF SourceType = "arxiv_pdf"
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
	PaperID    string           `json:"paper_id"`
	URI        string           `json:"uri"`
	LocalPath  string           `json:"local_path,omitempty"`
	SourceType SourceType       `json:"source_type"`
	Label      string           `json:"label,omitempty"`
	Status     SourceStatus     `json:"status"`
	Inspection SourceInspection `json:"inspection,omitempty"`
}

type Citation struct {
	Page    int    `json:"page"`
	Snippet string `json:"snippet,omitempty"`
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
	PaperID             string      `json:"paper_id"`
	Title               string      `json:"title"`
	Problem             string      `json:"problem"`
	OneLineSummary      string      `json:"one_line_summary"`
	MethodSummary       []string    `json:"method_summary"`
	ExperimentSummary   []string    `json:"experiment_summary"`
	KeyResults          []KeyResult `json:"key_results"`
	Conclusions         []string    `json:"conclusions"`
	Limitations         []string    `json:"limitations"`
	Citations           []Citation  `json:"citations"`
	Markdown            string      `json:"markdown,omitempty"`
	Language            string      `json:"language"`
	Style               string      `json:"style"`
	GeneratedAt         time.Time   `json:"generated_at"`
	HasBackgroundOmitted bool       `json:"has_background_omitted"`
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
	Language         string                `json:"language"`
	Style            string                `json:"style"`
	GeneratedAt      time.Time             `json:"generated_at"`
}

type PlanResult struct {
	Goal              string     `json:"goal"`
	SourceSummary     []PaperRef `json:"source_summary"`
	ExtractionStrategy []string  `json:"extraction_strategy"`
	ExpectedSections  []string   `json:"expected_sections"`
	Risks             []string   `json:"risks"`
	ToolPlan          []string   `json:"tool_plan"`
	WillCompare       bool       `json:"will_compare"`
	ApprovalRequired  bool       `json:"approval_required"`
	CreatedAt         time.Time  `json:"created_at"`
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
	SessionID       string         `json:"session_id"`
	Name            string         `json:"name,omitempty"`
	State           SessionState   `json:"state"`
	PermissionMode  PermissionMode `json:"permission_mode"`
	Language        string         `json:"language"`
	Style           string         `json:"style"`
	LastTask        string         `json:"last_task,omitempty"`
	ApprovalPending bool           `json:"approval_pending"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type SessionSnapshot struct {
	Meta      SessionMeta         `json:"meta"`
	Sources   []PaperRef          `json:"sources"`
	Plan      *PlanResult         `json:"plan,omitempty"`
	Digests   []PaperDigest       `json:"digests,omitempty"`
	Compare   *ComparisonDigest   `json:"comparison,omitempty"`
	Artifacts []ArtifactManifest  `json:"artifacts,omitempty"`
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
	Session    SessionSnapshot     `json:"session"`
	Plan       *PlanResult         `json:"plan,omitempty"`
	Digests    []PaperDigest       `json:"digests,omitempty"`
	Comparison *ComparisonDigest   `json:"comparison,omitempty"`
	Artifacts  []ArtifactManifest  `json:"artifacts,omitempty"`
}

