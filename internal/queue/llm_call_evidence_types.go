package queue

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

// One row can retain the bounded generation receipt, raw provider capture,
// candidate, request including native context, and prompt. Keep reads at
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
	Authority              model.StepAttemptAuthority
	Scope                  string
	WorkInput              json.RawMessage
	WorkKind               assemblyline.WorkKind
	Iteration              int
	OutputContinuation     int
	DispatchAttempt        int
	ParentCallEvidenceID   int64
	ReplacesCallEvidenceID int64
	SourceCorrection       *assemblyline.SourceBodyCorrectionEvidence
	RequestedModel         string
	Prepared               llm.PreparedModel
}

type LLMCallReceiptRecord struct {
	Authority          model.StepAttemptAuthority
	CallEvidenceID     int64
	Prepared           llm.PreparedModel
	Generation         llm.PreparedGeneration
	OutputLimitReached bool
	CallError          string
	Elapsed            time.Duration
}

// LLMCallOutcomeRecord appends the code-owned semantic validation result for
// one successful provider call.
type LLMCallOutcomeRecord struct {
	Authority       model.StepAttemptAuthority
	CallEvidenceID  int64
	Candidate       string
	ValidationError string
}

type LLMCallOutcome struct {
	CallEvidenceID  int64                `json:"call_evidence_id"`
	Status          LLMCallOutcomeStatus `json:"status"`
	ValidationError string               `json:"validation_error,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
}

type LLMCallEvidence struct {
	ID                       int64           `json:"id"`
	JobID                    int64           `json:"job_id"`
	Generation               int64           `json:"generation"`
	StepID                   int64           `json:"step_id"`
	StepAttempt              int64           `json:"step_attempt"`
	WorkerID                 string          `json:"worker_id"`
	Scope                    string          `json:"scope"`
	WorkInput                json.RawMessage `json:"work_input,omitempty"`
	WorkKind                 string          `json:"work_kind"`
	Iteration                int             `json:"iteration"`
	OutputContinuation       int             `json:"output_continuation"`
	DispatchAttempt          int             `json:"dispatch_attempt"`
	ParentCallEvidenceID     int64           `json:"parent_call_evidence_id,omitempty"`
	ReplacesCallEvidenceID   int64           `json:"replaces_call_evidence_id,omitempty"`
	SourceBaseCandidate      string          `json:"source_base_candidate,omitempty"`
	SourceStartByte          int             `json:"source_start_byte,omitempty"`
	SourceEndByte            int             `json:"source_end_byte,omitempty"`
	SourceQuestion           string          `json:"source_question,omitempty"`
	RequestedModel           string          `json:"requested_model"`
	Model                    string          `json:"model"`
	Protocol                 string          `json:"protocol"`
	ModelInput               string          `json:"model_input"`
	ModelInputBytes          int             `json:"model_input_bytes"`
	ProviderRequest          []byte          `json:"provider_request"`
	ProviderRequestBytes     int             `json:"provider_request_bytes"`
	ContextTokens            int             `json:"context_tokens"`
	MaxOutputTokens          int             `json:"max_output_tokens"`
	OutputLimitMode          string          `json:"output_limit_mode"`
	ProviderReceiptPresent   bool            `json:"provider_receipt_present"`
	GenerationReceipt        json.RawMessage `json:"generation_receipt"`
	RawResponsePresent       bool            `json:"raw_response_present"`
	RawResponse              []byte          `json:"raw_response,omitempty"`
	RawResponseBytes         int             `json:"raw_response_bytes"`
	Candidate                string          `json:"candidate,omitempty"`
	PromptTokens             int             `json:"prompt_tokens"`
	OutputTokens             int             `json:"output_tokens"`
	ProviderDurationNanos    int64           `json:"provider_duration_nanos"`
	OutputLimitReached       bool            `json:"output_limit_reached"`
	Status                   LLMCallStatus   `json:"status"`
	Error                    string          `json:"error,omitempty"`
	ElapsedNanos             int64           `json:"elapsed_nanos"`
	CreatedAt                time.Time       `json:"created_at"`
	ProviderReceiptCreatedAt *time.Time      `json:"provider_receipt_created_at,omitempty"`
	Outcome                  *LLMCallOutcome `json:"outcome,omitempty"`
}
