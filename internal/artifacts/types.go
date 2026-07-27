package artifacts

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	KindIntent               = "intent"
	KindCapabilityAudit      = "capability_audit"
	KindPlan                 = "plan"
	KindWorkspace            = "workspace"
	KindRetrieval            = "retrieval"
	KindWebEvidence          = "web_evidence"
	KindAnalysis             = "analysis"
	KindSubtaskResult        = "subtask_result"
	KindResponseDraft        = "response_draft"
	KindVerification         = "verification"
	KindMemoryCandidate      = "memory_candidate"
	KindToolCall             = "tool_call"
	KindToolResult           = "tool_result"
	KindImplementationLedger = "implementation_ledger"
)

const (
	MemoryModeOff            = "off"
	MemoryModeRelevantOnly   = "relevant_only"
	MemoryModeExplicitRecall = "explicit_recall"
)

const (
	SubtaskKindResearch     = "research"
	SubtaskKindAnalyze      = "analyze"
	SubtaskKindExecute      = "execute"
	SubtaskKindRespond      = "respond"
	SubtaskKindVerify       = "verify"
	SubtaskKindMemoryReview = "memory_review"
)

const (
	VerificationVerdictPass    = "pass"
	VerificationVerdictRevise  = "revise"
	VerificationVerdictBlocked = "blocked"
)

type Envelope struct {
	ID        string          `json:"id,omitempty"`
	JobID     int64           `json:"job_id,omitempty"`
	StepID    int64           `json:"step_id,omitempty"`
	Kind      string          `json:"kind"`
	Version   string          `json:"version"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at,omitempty"`
}

func (e Envelope) Validate() error {
	if e.Kind == "" {
		return errors.New("artifact kind is required")
	}
	if e.Version == "" {
		return errors.New("artifact version is required")
	}
	if len(e.Payload) == 0 || string(e.Payload) == "null" {
		return errors.New("artifact payload is required")
	}
	if !json.Valid(e.Payload) {
		return fmt.Errorf("artifact payload for %s is not valid JSON", e.Kind)
	}
	return nil
}

type IntentArtifact struct {
	UserGoal             string      `json:"user_goal"`
	Mode                 string      `json:"mode"`
	Objectives           []Objective `json:"objectives"`
	Constraints          []string    `json:"constraints"`
	RequiresAction       bool        `json:"requires_action"`
	RequiredCapabilities []string    `json:"required_capabilities"`
	CompletionCriteria   []string    `json:"completion_criteria"`
	MemoryMode           string      `json:"memory_mode"`
	UnresolvedReferences []string    `json:"unresolved_references"`
	Ambiguities          []string    `json:"ambiguities,omitempty"`
}

type Objective struct {
	ID                   string   `json:"id"`
	Description          string   `json:"description"`
	Priority             int      `json:"priority"`
	RequiresAction       bool     `json:"requires_action"`
	RequiredCapabilities []string `json:"required_capabilities"`
	AcceptanceCriteria   []string `json:"acceptance_criteria"`
}

type CapabilityAuditArtifact struct {
	AllowedTools          []string `json:"allowed_tools,omitempty"`
	AvailableTools        []string `json:"available_tools,omitempty"`
	MissingTools          []string `json:"missing_tools,omitempty"`
	AvailableCapabilities []string `json:"available_capabilities,omitempty"`
	MissingCapabilities   []string `json:"missing_capabilities,omitempty"`
	WorkspaceOK           bool     `json:"workspace_ok,omitempty"`
	WebSearchOK           bool     `json:"web_search_ok,omitempty"`
	Notes                 []string `json:"notes,omitempty"`
}

type PlanArtifact struct {
	Goal        string         `json:"goal"`
	Constraints map[string]any `json:"constraints,omitempty"`
	Subtasks    []Subtask      `json:"subtasks,omitempty"`
}

type Subtask struct {
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	RoleID               string   `json:"role_id"`
	ObjectiveID          string   `json:"objective_id"`
	Objective            string   `json:"objective"`
	Priority             int      `json:"priority"`
	RequiredCapabilities []string `json:"required_capabilities"`
	Constraints          []string `json:"constraints,omitempty"`
	SuccessCriteria      []string `json:"success_criteria"`
}

type WorkspaceFileExcerpt struct {
	Path     string   `json:"path"`
	Reason   string   `json:"reason,omitempty"`
	Excerpt  string   `json:"excerpt,omitempty"`
	Score    float64  `json:"score,omitempty"`
	Language string   `json:"language,omitempty"`
	Symbols  []string `json:"symbols,omitempty"`
}

type WorkspaceArtifact struct {
	Root            string                 `json:"root,omitempty"`
	FilesConsidered int                    `json:"files_considered,omitempty"`
	RelevantFiles   []WorkspaceFileExcerpt `json:"relevant_files,omitempty"`
	Languages       []string               `json:"languages,omitempty"`
	Summary         string                 `json:"summary,omitempty"`
	MissingContext  []string               `json:"missing_context,omitempty"`
}

type RetrievalItem struct {
	ID         int64    `json:"id,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Content    string   `json:"content,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Score      float64  `json:"score,omitempty"`
}

type RetrievalArtifact struct {
	Summary         string          `json:"summary,omitempty"`
	Authority       string          `json:"authority,omitempty"`
	Items           []RetrievalItem `json:"items,omitempty"`
	Omitted         int             `json:"omitted,omitempty"`
	OmittedByReason map[string]int  `json:"omitted_by_reason,omitempty"`
}

type WebDocument struct {
	Provider  string `json:"provider,omitempty"`
	SearchURL string `json:"search_url,omitempty"`
	URL       string `json:"url,omitempty"`
	Title     string `json:"title,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
	Content   string `json:"content,omitempty"`
}

type WebEvidenceArtifact struct {
	Query     string        `json:"query,omitempty"`
	Summary   string        `json:"summary,omitempty"`
	Documents []WebDocument `json:"documents,omitempty"`
}

type MemoryCandidateArtifact struct {
	CandidateKind string         `json:"candidate_kind"`
	Content       string         `json:"content"`
	Confidence    float64        `json:"confidence,omitempty"`
	Status        string         `json:"status,omitempty"`
	Provenance    map[string]any `json:"provenance,omitempty"`
}

type SubtaskResultArtifact struct {
	SubtaskID            string   `json:"subtask_id"`
	Kind                 string   `json:"kind"`
	RoleID               string   `json:"role_id"`
	ObjectiveID          string   `json:"objective_id"`
	Objective            string   `json:"objective"`
	Priority             int      `json:"priority"`
	RequiredCapabilities []string `json:"required_capabilities"`
	Summary              string   `json:"summary"`
	EvidenceIDs          []int64  `json:"evidence_ids,omitempty"`
	Sources              []string `json:"sources"`
}

type ToolCallArtifact struct {
	Tool        string         `json:"tool"`
	Skill       string         `json:"skill,omitempty"`
	Input       map[string]any `json:"input,omitempty"`
	Allowed     bool           `json:"allowed,omitempty"`
	AllowedBy   []string       `json:"allowed_by,omitempty"`
	Forbidden   []string       `json:"forbidden,omitempty"`
	RequestedBy string         `json:"requested_by,omitempty"`
}

type ToolResultArtifact struct {
	Tool     string         `json:"tool"`
	Skill    string         `json:"skill,omitempty"`
	Accepted bool           `json:"accepted"`
	Summary  string         `json:"summary,omitempty"`
	Output   map[string]any `json:"output,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type AnalysisArtifact struct {
	Summary     string   `json:"summary"`
	Blockers    []string `json:"blockers"`
	Assumptions []string `json:"assumptions"`
}

type ResponseDraftArtifact struct {
	Response string `json:"response"`
}

type VerificationArtifact struct {
	Verdict              string              `json:"verdict"`
	IndependentChallenge bool                `json:"independent_challenge"`
	SupportedClaims      []string            `json:"supported_claims"`
	UnsupportedClaims    []string            `json:"unsupported_claims"`
	MissingEvidence      []string            `json:"missing_evidence"`
	Contradictions       []string            `json:"contradictions"`
	RoleViolations       []string            `json:"role_violations"`
	ObjectiveCoverage    []ObjectiveCoverage `json:"objective_coverage"`
	AdviceOnly           bool                `json:"advice_only"`
	RecommendedActions   []string            `json:"recommended_actions"`
}

type ObjectiveCoverage struct {
	ObjectiveID string   `json:"objective_id"`
	Satisfied   bool     `json:"satisfied"`
	EvidenceIDs []int64  `json:"evidence_ids"`
	Gaps        []string `json:"gaps"`
}
