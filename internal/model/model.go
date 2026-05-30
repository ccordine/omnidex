package model

import (
	"encoding/json"
	"time"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
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
	PipelineAssistant = "assistant"
	PipelineChat      = "chat"
	PipelineCoding    = "coding"
	PipelineStory     = "story"
	PipelineDataQuery       = "data_query"
	PipelineDataExplore     = "data_explore"
	PipelineProjectDebugger = "project_debugger"
	PipelineScrumCardLLM    = "scrum_card_llm"
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
)

const (
	MemoryTrustTagApproved = "trust:approved"
	MemoryTrustTagDurable  = "trust:durable"
)

type Job struct {
	ID          int64           `json:"id"`
	Instruction string          `json:"instruction"`
	Pipeline    string          `json:"pipeline"`
	Status      string          `json:"status"`
	Result      string          `json:"result,omitempty"`
	Error       string          `json:"error,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

type Step struct {
	ID         int64      `json:"id"`
	JobID      int64      `json:"job_id"`
	Action     string     `json:"action"`
	SortIndex  int        `json:"sort_index"`
	Status     string     `json:"status"`
	WorkerID   string     `json:"worker_id,omitempty"`
	Output     string     `json:"output,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type StepContext struct {
	ID        int64     `json:"id"`
	StepID    int64     `json:"step_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

type JobDetails struct {
	Job      Job           `json:"job"`
	Steps    []Step        `json:"steps"`
	Contexts []StepContext `json:"contexts"`
}

type ClaimedStep struct {
	Job      Job
	Step     Step
	Contexts []StepContext
}

type MemoryChunk struct {
	ID        int64     `json:"id"`
	Source    string    `json:"source"`
	Kind      string    `json:"kind"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type MemoryMatch struct {
	ID         int64     `json:"id"`
	Kind       string    `json:"kind"`
	Content    string    `json:"content"`
	Tags       []string  `json:"tags,omitempty"`
	Categories []string  `json:"categories,omitempty"`
	Score      float64   `json:"score"`
	CreatedAt  time.Time `json:"created_at"`
}

type MemoryFacet struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type MemoryCandidate struct {
	ID             int64           `json:"id"`
	JobID          int64           `json:"job_id,omitempty"`
	SourceMemoryID *int64          `json:"source_memory_id,omitempty"`
	CandidateKind  string          `json:"candidate_kind"`
	Content        string          `json:"content"`
	Provenance     json.RawMessage `json:"provenance,omitempty"`
	Confidence     float64         `json:"confidence,omitempty"`
	Status         string          `json:"status,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type ClaimRecord struct {
	ID             int64     `json:"id"`
	JobID          int64     `json:"job_id,omitempty"`
	StepID         int64     `json:"step_id,omitempty"`
	Text           string    `json:"text"`
	NormalizedText string    `json:"normalized_text,omitempty"`
	Status         string    `json:"status,omitempty"`
	Confidence     float64   `json:"confidence,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type ClaimSupportRecord struct {
	ID           int64     `json:"id"`
	ClaimID      int64     `json:"claim_id"`
	EvidenceID   int64     `json:"evidence_id"`
	SupportScore float64   `json:"support_score,omitempty"`
	Rationale    string    `json:"rationale,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type ClaimSupportDetail struct {
	ID                int64     `json:"id"`
	ClaimID           int64     `json:"claim_id"`
	ClaimText         string    `json:"claim_text"`
	ClaimStatus       string    `json:"claim_status,omitempty"`
	EvidenceID        int64     `json:"evidence_id"`
	EvidenceKind      string    `json:"evidence_kind,omitempty"`
	EvidenceSourceRef string    `json:"evidence_source_ref,omitempty"`
	SupportScore      float64   `json:"support_score,omitempty"`
	Rationale         string    `json:"rationale,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type JobInspection struct {
	Job              Job                  `json:"job"`
	JobID            int64                `json:"job_id"`
	Artifacts        []artifacts.Envelope `json:"artifacts,omitempty"`
	Evidence         []evidence.Record    `json:"evidence,omitempty"`
	Claims           []ClaimRecord        `json:"claims,omitempty"`
	ClaimSupport     []ClaimSupportDetail `json:"claim_support,omitempty"`
	MemoryCandidates []MemoryCandidate    `json:"memory_candidates,omitempty"`
}

type MemoryCandidatePromotionResult struct {
	Candidate MemoryCandidate `json:"candidate"`
	Memory    *MemoryChunk    `json:"memory,omitempty"`
}

type Channel struct {
	ID        string          `json:"id"`
	Name      string          `json:"name,omitempty"`
	Persona   string          `json:"persona"`
	System    string          `json:"system,omitempty"`
	Provider  string          `json:"provider,omitempty"`
	Model     string          `json:"model,omitempty"`
	Context   json.RawMessage `json:"context,omitempty"`
	Tags      []string        `json:"tags,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type ChannelMessage struct {
	ID        int64     `json:"id"`
	ChannelID string    `json:"channel_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type DataSourceChannel struct {
	ID           string    `json:"id"`
	DataSourceID string    `json:"data_source_id"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type DataSourceChannelMessage struct {
	ID        int64           `json:"id"`
	ChannelID string          `json:"channel_id"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	JobID     *int64          `json:"job_id,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}
