package queue

import (
	"encoding/json"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

const (
	maxStationCallErrorBytes = 8 * 1024
	maxStationCallInputBytes = 128 * 1024
)

type StationCallOpenRecord struct {
	Authority model.StepAttemptAuthority
	Gap       StationGapOpening
	Discovery StationDiscoveryReceipt
	Prepared  llm.PreparedModel
}

type StationCallOpening struct {
	ID                     int64           `json:"id"`
	GapOpeningID           int64           `json:"gap_opening_id"`
	DiscoveryReceiptID     int64           `json:"discovery_receipt_id"`
	JobID                  int64           `json:"job_id"`
	Generation             int64           `json:"generation"`
	StepID                 int64           `json:"step_id"`
	StepAttempt            int64           `json:"step_attempt"`
	WorkerID               string          `json:"worker_id"`
	GapID                  string          `json:"gap_id"`
	Protocol               string          `json:"protocol"`
	TokenizerProfile       string          `json:"tokenizer_profile"`
	ProviderMethod         string          `json:"provider_method"`
	ProviderEndpoint       string          `json:"provider_endpoint"`
	WireRequest            []byte          `json:"-"`
	WireRequestSHA256      string          `json:"wire_request_sha256"`
	WireRequestBytes       int             `json:"wire_request_bytes"`
	Expectation            json.RawMessage `json:"expectation"`
	ExpectationSHA256      string          `json:"expectation_sha256"`
	ObservationChallenge   string          `json:"observation_challenge"`
	Model                  string          `json:"model"`
	ContextTokens          int             `json:"context_tokens"`
	MaxInputTokens         int             `json:"max_input_tokens"`
	MaxOutputTokens        int             `json:"max_output_tokens"`
	ModelInput             string          `json:"model_input"`
	ModelInputSHA256       string          `json:"model_input_sha256"`
	ModelInputBytes        int             `json:"model_input_bytes"`
	ModelInputTokenCeiling int             `json:"model_input_token_ceiling"`
	CreatedAt              time.Time       `json:"created_at"`
}

type StationCallReceiptRecord struct {
	Authority model.StepAttemptAuthority
	OpeningID int64
	GapID     string
	Result    llm.PreparedGeneration
	Error     string
}

// StationCallReceiptEvidenceRecord is the sole production terminal-call
// command. The provider receipt and its immutable audit projection commit in
// one transaction; neither record may exist without the other.
type StationCallReceiptEvidenceRecord struct {
	Receipt         StationCallReceiptRecord
	RequestedModel  string
	EvidenceAttempt int
	LatencyMS       int64
}

type StationCallReceiptEvidence struct {
	Receipt  StationCallReceipt
	Evidence LLMCallEvidence
}

type StationCallReceipt struct {
	ID               int64           `json:"id"`
	OpeningID        int64           `json:"opening_id"`
	JobID            int64           `json:"job_id"`
	Generation       int64           `json:"generation"`
	StepID           int64           `json:"step_id"`
	StepAttempt      int64           `json:"step_attempt"`
	WorkerID         string          `json:"worker_id"`
	GapID            string          `json:"gap_id"`
	Status           string          `json:"status"`
	GenerationJSON   json.RawMessage `json:"generation_json"`
	GenerationSHA256 string          `json:"generation_sha256"`
	Error            string          `json:"error,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

// LLMCallEvidenceRecord derives the legacy trace projection from the durable
// call opening and terminal receipt. Callers cannot independently restate it.
func (receipt StationCallReceipt) LLMCallEvidenceRecord(
	authority model.StepAttemptAuthority,
	gap StationGapOpening,
	opening StationCallOpening,
	requestedModel string,
	attempt int,
	latencyMS int64,
) (LLMCallEvidenceRecord, error) {
	return llmEvidenceForStationCall(authority, gap, opening, receipt, requestedModel, attempt, latencyMS)
}
