package objectiveadvisory

import (
	"context"
	"time"
)

const (
	TriggerPostGroundingObjective = "post_grounding_objective_advisory"
	TriggerVersionV1              = "v1"
	AuthorityNonAuthoritative     = "non_authoritative_advisory"
	CapsuleLabel                  = "ADVISORY — NON-AUTHORITATIVE"

	MaxProjectionBytes   = 24 * 1024
	MaxRawTextBytes      = 8 * 1024
	MaxChunkBytes        = 1024
	MaxCapsuleBytes      = 1024
	MaxChunksPerArtifact = 16
	MaxConfiguredSources = 2
)

type Status string

const (
	StatusNotRun    Status = "not_run"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusInvalid   Status = "invalid"
	StatusTruncated Status = "truncated"
)

type TextAuthority struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type EvidenceSummary struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	SHA256  string `json:"sha256"`
}

type ProjectionInput struct {
	ObjectiveID         string            `json:"objective_id"`
	Generation          int64             `json:"generation"`
	Objective           string            `json:"objective"`
	UserAuthorities     []TextAuthority   `json:"user_authorities"`
	Constraints         []string          `json:"constraints"`
	GroundedEvidence    []EvidenceSummary `json:"grounded_evidence"`
	Decisions           []string          `json:"decisions"`
	Invariants          []string          `json:"invariants"`
	UnresolvedQuestions []string          `json:"unresolved_questions"`
	UsefulAdvice        string            `json:"useful_advice"`
}

type Projection struct {
	Schema          string          `json:"schema"`
	ID              string          `json:"id"`
	TriggerID       string          `json:"trigger_id"`
	TriggerVersion  string          `json:"trigger_version"`
	Input           ProjectionInput `json:"input"`
	Rendered        string          `json:"rendered"`
	RenderedSHA256  string          `json:"rendered_sha256"`
	RenderedBytes   int             `json:"rendered_bytes"`
	EstimatedTokens int             `json:"estimated_tokens"`
}

type SemanticGap struct {
	ObjectiveID string            `json:"objective_id"`
	Generation  int64             `json:"generation"`
	Requirement string            `json:"requirement"`
	Candidate   string            `json:"candidate"`
	Evidence    []EvidenceSummary `json:"evidence"`
}

type SamplingConfig struct {
	Temperature float64  `json:"temperature"`
	TopP        *float64 `json:"top_p,omitempty"`
	Seed        *int64   `json:"seed,omitempty"`
}

type Budget struct {
	MaxInputBytes   int `json:"max_input_bytes"`
	MaxOutputBytes  int `json:"max_output_bytes"`
	MaxOutputTokens int `json:"max_output_tokens"`
}

type SourceConfig struct {
	ID       string         `json:"id"`
	Provider string         `json:"provider"`
	Model    string         `json:"model"`
	Sampling SamplingConfig `json:"sampling"`
	Budget   Budget         `json:"budget"`
}

type Config struct {
	Mode                Mode           `json:"mode"`
	Sources             []SourceConfig `json:"sources"`
	MinimumRelevance    float64        `json:"minimum_relevance"`
	MaxSelectedCapsules int            `json:"max_selected_capsules"`
}

type GenerateRequest struct {
	TriggerID      string       `json:"trigger_id"`
	TriggerVersion string       `json:"trigger_version"`
	Projection     Projection   `json:"projection"`
	Source         SourceConfig `json:"source"`
}

type Generation struct {
	FinalText         string        `json:"final_text"`
	EffectiveProvider string        `json:"effective_provider"`
	EffectiveModel    string        `json:"effective_model"`
	ModelDigest       string        `json:"model_digest"`
	Quantization      string        `json:"quantization"`
	PromptTokens      int           `json:"prompt_tokens"`
	OutputTokens      int           `json:"output_tokens"`
	Duration          time.Duration `json:"duration"`
	FinishReason      string        `json:"finish_reason"`
}

type Provider interface {
	Generate(context.Context, GenerateRequest) (Generation, error)
}

type Embedder interface {
	Embedding(context.Context, string) ([]float64, error)
}

type Clock func() time.Time

type Artifact struct {
	ID                string         `json:"id"`
	ObjectiveID       string         `json:"objective_id"`
	Generation        int64          `json:"generation"`
	TriggerID         string         `json:"trigger_id"`
	TriggerVersion    string         `json:"trigger_version"`
	ProjectionID      string         `json:"projection_id"`
	ProjectionSHA256  string         `json:"projection_sha256"`
	SourceID          string         `json:"source_id"`
	Provider          string         `json:"provider"`
	RequestedModel    string         `json:"requested_model"`
	EffectiveProvider string         `json:"effective_provider"`
	EffectiveModel    string         `json:"effective_model"`
	ModelDigest       string         `json:"model_digest"`
	Quantization      string         `json:"quantization"`
	Sampling          SamplingConfig `json:"sampling"`
	RawText           string         `json:"raw_text,omitempty"`
	RawTextSHA256     string         `json:"raw_text_sha256,omitempty"`
	RawBytes          int            `json:"raw_bytes"`
	PromptTokens      int            `json:"prompt_tokens"`
	OutputTokens      int            `json:"output_tokens"`
	Duration          time.Duration  `json:"duration"`
	FinishReason      string         `json:"finish_reason,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	Status            Status         `json:"status"`
	Failure           string         `json:"failure,omitempty"`
	Authority         string         `json:"authority"`
}

type Chunk struct {
	ID               string   `json:"id"`
	AdvisoryID       string   `json:"advisory_id"`
	Index            int      `json:"index"`
	StartByte        int      `json:"start_byte"`
	EndByte          int      `json:"end_byte"`
	SourceTextSHA256 string   `json:"source_text_sha256"`
	Content          string   `json:"content"`
	ContentSHA256    string   `json:"content_sha256"`
	Tags             []string `json:"tags"`
	ByteCost         int      `json:"byte_cost"`
}

type Capsule struct {
	ID                string `json:"id"`
	SourceAdvisoryID  string `json:"source_advisory_id"`
	SourceChunkID     string `json:"source_chunk_id"`
	ObjectiveID       string `json:"objective_id"`
	Generation        int64  `json:"generation"`
	SemanticGapSHA256 string `json:"semantic_gap_sha256"`
	Content           string `json:"content"`
	Provider          string `json:"provider"`
	RequestedModel    string `json:"requested_model"`
	EffectiveModel    string `json:"effective_model"`
	Authority         string `json:"authority"`
	RelevanceBasis    string `json:"relevance_basis"`
	Label             string `json:"label"`
	ByteCost          int    `json:"byte_cost"`
	EstimatedTokens   int    `json:"estimated_tokens"`
}

type Metrics struct {
	AdvisoryCalls                 int           `json:"advisory_calls"`
	EmbeddingCalls                int           `json:"embedding_calls"`
	RawBytes                      int           `json:"raw_bytes"`
	ChunksProduced                int           `json:"chunks_produced"`
	CandidateCapsules             int           `json:"candidate_capsules"`
	SelectedCapsules              int           `json:"selected_capsules"`
	UnselectedChunks              int           `json:"unselected_chunks"`
	PotentialCapsuleContentBytes  int           `json:"potential_capsule_content_bytes"`
	PotentialCapsuleContentTokens int           `json:"potential_capsule_content_tokens"`
	SelectedCapsuleContentBytes   int           `json:"selected_capsule_content_bytes"`
	SelectedCapsuleContentTokens  int           `json:"selected_capsule_content_tokens"`
	PromptTokens                  int           `json:"prompt_tokens"`
	OutputTokens                  int           `json:"output_tokens"`
	WallTime                      time.Duration `json:"wall_time"`
}

type Report struct {
	Mode              Mode       `json:"mode"`
	TriggerID         string     `json:"trigger_id"`
	TriggerVersion    string     `json:"trigger_version"`
	SemanticGapSHA256 string     `json:"semantic_gap_sha256"`
	Projection        Projection `json:"projection"`
	Artifacts         []Artifact `json:"artifacts"`
	Chunks            []Chunk    `json:"chunks"`
	CandidateCapsules []Capsule  `json:"candidate_capsules"`
	ActiveCapsules    []Capsule  `json:"active_capsules"`
	ReductionStatus   Status     `json:"reduction_status"`
	ReductionError    string     `json:"reduction_error,omitempty"`
	Metrics           Metrics    `json:"metrics"`
}

type Runner interface {
	Run(context.Context, ProjectionInput, SemanticGap) (Report, error)
}
