package queue

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

// One row can retain roughly 69 MiB across the generation receipt, raw
// provider capture, candidate, request, and prompt. Keep server-side reads at
// one exact call per page even though this journal is not publicly exposed.
const MaxLLMCallEvidencePageSize = 1

var ErrLLMCallTerminalizedByAttempt = errors.New("LLM call was terminalized by its step attempt")

type LLMCallStatus string

const (
	LLMCallSucceeded LLMCallStatus = "succeeded"
	LLMCallFailed    LLMCallStatus = "failed"
)

type LLMCallOutcomeStatus string

const (
	LLMCallAccepted       LLMCallOutcomeStatus = "accepted"
	LLMCallRejected       LLMCallOutcomeStatus = "rejected"
	LLMCallProviderFailed LLMCallOutcomeStatus = "provider_failed"
	LLMCallInterrupted    LLMCallOutcomeStatus = "interrupted"
)

type LLMCallOpeningRecord struct {
	Authority      model.StepAttemptAuthority
	Scope          string
	WorkID         string
	WorkKind       assemblyline.WorkKind
	RequestedModel string
	Prepared       llm.PreparedModel
}

type LLMCallReceiptRecord struct {
	Authority      model.StepAttemptAuthority
	CallEvidenceID int64
	Prepared       llm.PreparedModel
	Generation     llm.PreparedGeneration
	CallError      string
	Elapsed        time.Duration
}

// LLMCallOutcomeRecord appends the code-owned semantic validation result for
// one successful provider call. It never changes the immutable call receipt.
type LLMCallOutcomeRecord struct {
	Authority       model.StepAttemptAuthority
	CallEvidenceID  int64
	Candidate       string
	Projection      *assemblyline.PortableResultProjection
	ValidationError string
}

type LLMCallOutcome struct {
	CallEvidenceID        int64                `json:"call_evidence_id"`
	Status                LLMCallOutcomeStatus `json:"status"`
	CandidateSHA256       string               `json:"candidate_sha256"`
	Projection            json.RawMessage      `json:"projection,omitempty"`
	ValidationError       string               `json:"validation_error,omitempty"`
	ValidationErrorSHA256 string               `json:"validation_error_sha256,omitempty"`
	CreatedAt             time.Time            `json:"created_at"`
}

// LLMCallProjectionEvidence records the code-owned accepted span without
// duplicating the potentially multi-megabyte source. Candidate remains the
// immutable source of bytes on the call receipt; these fields bind the exact
// accepted artifact to that source by kind, hashes, and byte boundaries.
type LLMCallProjectionEvidence struct {
	Kind                 assemblyline.PortableResultProjectionKind `json:"kind"`
	SourceResponseSHA256 string                                    `json:"source_response_sha256"`
	SourceSHA256         string                                    `json:"source_sha256"`
	StartByte            int                                       `json:"start_byte"`
	EndByte              int                                       `json:"end_byte"`
	RawBytes             int                                       `json:"raw_bytes"`
	DiscardedBytes       int                                       `json:"discarded_bytes"`
}

type LLMCallEvidence struct {
	ID                       int64           `json:"id"`
	JobID                    int64           `json:"job_id"`
	Generation               int64           `json:"generation"`
	StepID                   int64           `json:"step_id"`
	StepAttempt              int64           `json:"step_attempt"`
	WorkerID                 string          `json:"worker_id"`
	Scope                    string          `json:"scope"`
	WorkID                   string          `json:"work_id"`
	WorkKind                 string          `json:"work_kind"`
	RequestedModel           string          `json:"requested_model"`
	Model                    string          `json:"model"`
	Protocol                 string          `json:"protocol"`
	SystemEnvelope           string          `json:"system_envelope"`
	PromptHint               string          `json:"prompt_hint"`
	ModelInput               string          `json:"model_input"`
	ModelInputSHA256         string          `json:"model_input_sha256"`
	ModelInputBytes          int             `json:"model_input_bytes"`
	ProviderRequest          []byte          `json:"provider_request"`
	ProviderRequestSHA256    string          `json:"provider_request_sha256"`
	ProviderRequestBytes     int             `json:"provider_request_bytes"`
	ContextTokens            int             `json:"context_tokens"`
	MaxOutputTokens          int             `json:"max_output_tokens"`
	OutputLimitMode          string          `json:"output_limit_mode"`
	ProviderReceiptPresent   bool            `json:"provider_receipt_present"`
	GenerationReceipt        json.RawMessage `json:"generation_receipt"`
	GenerationReceiptSHA256  string          `json:"generation_receipt_sha256"`
	RawResponsePresent       bool            `json:"raw_response_present"`
	RawResponse              []byte          `json:"raw_response,omitempty"`
	RawResponseSHA256        string          `json:"raw_response_sha256,omitempty"`
	RawResponseBytes         int             `json:"raw_response_bytes"`
	Candidate                string          `json:"candidate,omitempty"`
	CandidateSHA256          string          `json:"candidate_sha256,omitempty"`
	PromptTokens             int             `json:"prompt_tokens"`
	OutputTokens             int             `json:"output_tokens"`
	ProviderDurationNanos    int64           `json:"provider_duration_nanos"`
	Status                   LLMCallStatus   `json:"status"`
	Error                    string          `json:"error,omitempty"`
	ErrorSHA256              string          `json:"error_sha256,omitempty"`
	ElapsedNanos             int64           `json:"elapsed_nanos"`
	CreatedAt                time.Time       `json:"created_at"`
	ProviderReceiptCreatedAt *time.Time      `json:"provider_receipt_created_at,omitempty"`
	Outcome                  *LLMCallOutcome `json:"outcome,omitempty"`
}
