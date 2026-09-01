package model

import (
	"encoding/json"
	"time"
)

const (
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusCanceled  = "canceled"
	JobStatusWaiting   = "waiting_input"
)

const (
	StepStatusPending   = "pending"
	StepStatusRunning   = "running"
	StepStatusCompleted = "completed"
	StepStatusFailed    = "failed"
	StepStatusWaiting   = "waiting_input"
	StepStatusCanceled  = "canceled"
)

const (
	PipelineChat   = "chat"
	PipelineCoding = "coding"
	PipelineScrum  = "scrum"
)

const (
	MemoryKindEpisodic    = "episodic"
	MemoryKindProcedural  = "procedural"
	MemoryKindInstruction = "instruction"
	MemoryKindPreference  = "preference"
	MemoryKindReference   = "reference"
)

const (
	MemoryCandidateStatusCandidate = "candidate"
	MemoryCandidateStatusApproved  = "approved"
	MemoryCandidateStatusDurable   = "durable"
	MemoryCandidateStatusRejected  = "rejected"

	MemoryPromotionAuthorityCurrent    MemoryPromotionAuthority = "current_generation"
	MemoryPromotionAuthorityHistorical MemoryPromotionAuthority = "historical_generation"
	MemoryPromotionAuthorityGlobal     MemoryPromotionAuthority = "global"
)

type MemoryPromotionAuthority string

const (
	MemoryTrustTagApproved = "trust:approved"
	MemoryTrustTagDurable  = "trust:durable"
)

type Job struct {
	ID                int64           `json:"id"`
	Instruction       string          `json:"instruction"`
	Pipeline          string          `json:"pipeline"`
	Status            string          `json:"status"`
	Result            string          `json:"result,omitempty"`
	Error             string          `json:"error,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
	CurrentGeneration int64           `json:"current_generation"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty"`
}

type Step struct {
	ID                     int64      `json:"id"`
	JobID                  int64      `json:"job_id"`
	Action                 string     `json:"action"`
	SortIndex              int        `json:"sort_index"`
	Status                 string     `json:"status"`
	Generation             int64      `json:"generation"`
	SupersededAtGeneration *int64     `json:"superseded_at_generation,omitempty"`
	WorkerID               string     `json:"worker_id,omitempty"`
	Output                 string     `json:"output,omitempty"`
	Error                  string     `json:"error,omitempty"`
	StartedAt              *time.Time `json:"started_at,omitempty"`
	FinishedAt             *time.Time `json:"finished_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type JobDetails struct {
	Job   Job    `json:"job"`
	Steps []Step `json:"steps"`
}

type ClaimedStep struct {
	Job           Job
	Step          Step
	Authority     StepAttemptAuthority
	LeaseDeadline time.Time
}

type MemoryChunk struct {
	ID        int64        `json:"id"`
	Scope     MemoryScope  `json:"scope"`
	Source    MemorySource `json:"source"`
	Kind      MemoryKind   `json:"kind"`
	Content   string       `json:"content"`
	CreatedAt time.Time    `json:"created_at"`
}

type MemoryMatch struct {
	ID         int64            `json:"id"`
	Scope      MemoryScope      `json:"scope"`
	Kind       MemoryKind       `json:"kind"`
	Content    string           `json:"content"`
	Tags       []string         `json:"tags,omitempty"`
	Categories []MemoryCategory `json:"categories,omitempty"`
	Score      float64          `json:"score"`
	CreatedAt  time.Time        `json:"created_at"`
}

type MemoryFacet struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type MemoryCandidate struct {
	ID             int64           `json:"id"`
	Scope          MemoryScope     `json:"scope"`
	JobID          int64           `json:"job_id,omitempty"`
	Generation     *int64          `json:"generation,omitempty"`
	SourceMemoryID *int64          `json:"source_memory_id,omitempty"`
	CandidateKind  MemoryKind      `json:"candidate_kind"`
	Content        string          `json:"content"`
	Provenance     json.RawMessage `json:"provenance,omitempty"`
	Confidence     float64         `json:"confidence,omitempty"`
	Status         string          `json:"status,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type MemoryCandidatePromotionResult struct {
	Candidate MemoryCandidate `json:"candidate"`
	Memory    *MemoryChunk    `json:"memory,omitempty"`
}

type Channel struct {
	ID                           ChannelID           `json:"id"`
	Scope                        ChannelScope        `json:"scope"`
	Name                         string              `json:"name,omitempty"`
	Tags                         []string            `json:"tags,omitempty"`
	WorkspaceRoot                string              `json:"workspace_root"`
	DataSourceID                 DataSourceID        `json:"data_source_id,omitempty"`
	Mode                         ChannelMode         `json:"mode"`
	RoleplayViewpointCharacterID RoleplayCharacterID `json:"roleplay_viewpoint_character_id,omitempty"`
	CreatedAt                    time.Time           `json:"created_at"`
	UpdatedAt                    time.Time           `json:"updated_at"`
}

type ChannelID string
type DataSourceID string
type RoleplayCharacterID string
type ChannelMessageRole string

const (
	ChannelMessageRoleUser      ChannelMessageRole = "user"
	ChannelMessageRoleAssistant ChannelMessageRole = "assistant"
)

type ChannelMessage struct {
	ID          int64                            `json:"id"`
	ChannelID   ChannelID                        `json:"channel_id"`
	Role        ChannelMessageRole               `json:"role"`
	SpeakerName string                           `json:"speaker_name,omitempty"`
	Roleplay    *ChannelMessageRoleplayAuthority `json:"roleplay,omitempty"`
	Turn        *ChannelMessageTurnState         `json:"turn,omitempty"`
	Content     string                           `json:"content"`
	CreatedAt   time.Time                        `json:"created_at"`
}

type ChannelMessageRoleplayAuthority struct {
	PersonaKind      string                       `json:"persona_kind"`
	CharacterID      RoleplayCharacterID          `json:"character_id,omitempty"`
	ContributionKind string                       `json:"contribution_kind"`
	Parts            []ChannelMessageRoleplayPart `json:"parts,omitempty"`
}

type ChannelMessageRoleplayPart struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type ChannelMessageTurnState struct {
	JobID     int64     `json:"job_id"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ChannelMessagePage struct {
	Messages     []ChannelMessage `json:"messages"`
	NextBeforeID *int64           `json:"next_before_id,omitempty"`
	HasMore      bool             `json:"has_more"`
}

type DataSourceChannel struct {
	ID           string    `json:"id"`
	DataSourceID string    `json:"data_source_id"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
