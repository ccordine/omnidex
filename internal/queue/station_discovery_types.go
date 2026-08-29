package queue

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

type StationDiscoveryOpenRecord struct {
	Authority model.StepAttemptAuthority
	Gap       StationGapOpening
	Selection llm.ProviderIdentitySelection
}

// StationGapDiscoveryOpenRecord is the sole production opening authority. The
// typed gap and its provider-discovery boundary commit in one transaction, so
// a local database failure cannot strand a gap before provider discovery.
type StationGapDiscoveryOpenRecord struct {
	Gap       StationGapOpenRecord
	Selection llm.ProviderIdentitySelection
}

type StationGapDiscoveryOpening struct {
	Gap       StationGapOpening
	Discovery StationDiscoveryOpening
}

type StationDiscoveryOpening struct {
	ID              int64           `json:"id"`
	GapOpeningID    int64           `json:"gap_opening_id"`
	JobID           int64           `json:"job_id"`
	Generation      int64           `json:"generation"`
	StepID          int64           `json:"step_id"`
	StepAttempt     int64           `json:"step_attempt"`
	WorkerID        string          `json:"worker_id"`
	GapID           string          `json:"gap_id"`
	Selection       json.RawMessage `json:"selection"`
	SelectionSHA256 string          `json:"selection_sha256"`
	Challenge       string          `json:"challenge"`
	CreatedAt       time.Time       `json:"created_at"`
}

type StationDiscoveryReceiptRecord struct {
	Authority     model.StepAttemptAuthority
	OpeningID     int64
	GapID         string
	Observed      llm.ObservedProviderIdentity
	FailureReason StationDiscoveryFailureReason
	Error         string
}

type StationDiscoveryFailureReason string

const (
	StationDiscoveryFailureEvidenceRejected    StationDiscoveryFailureReason = "evidence_rejected"
	StationDiscoveryFailureObservationRejected StationDiscoveryFailureReason = "observation_rejected"
	StationDiscoveryFailureProviderRejected    StationDiscoveryFailureReason = "provider_contract_rejected"
)

func (reason StationDiscoveryFailureReason) Validate() error {
	switch reason {
	case StationDiscoveryFailureEvidenceRejected,
		StationDiscoveryFailureObservationRejected,
		StationDiscoveryFailureProviderRejected:
		return nil
	default:
		return fmt.Errorf("station discovery failure reason %q is not registered", reason)
	}
}

// StationDiscoveryCallOpenRecord commits a successful discovery receipt and
// the exact provider-call opening together. Provider inference is illegal
// until this transition commits.
type StationDiscoveryCallOpenRecord struct {
	Authority model.StepAttemptAuthority
	Gap       StationGapOpening
	Discovery StationDiscoveryOpening
	Observed  llm.ObservedProviderIdentity
	Prepared  llm.PreparedModel
}

type StationDiscoveryCallOpening struct {
	Discovery StationDiscoveryReceipt
	Call      StationCallOpening
	Attempt   model.StepAttemptStatus
}

type StationDiscoveryFailureRecord struct {
	Authority     model.StepAttemptAuthority
	Gap           StationGapOpening
	Discovery     StationDiscoveryOpening
	Observed      llm.ObservedProviderIdentity
	FailureReason StationDiscoveryFailureReason
	Error         string
}

type StationDiscoveryFailure struct {
	Discovery StationDiscoveryReceipt
	Outcome   StationGapOutcome
}

type StationDiscoveryReceipt struct {
	ID                int64           `json:"id"`
	OpeningID         int64           `json:"opening_id"`
	JobID             int64           `json:"job_id"`
	Generation        int64           `json:"generation"`
	StepID            int64           `json:"step_id"`
	StepAttempt       int64           `json:"step_attempt"`
	WorkerID          string          `json:"worker_id"`
	GapID             string          `json:"gap_id"`
	Status            string          `json:"status"`
	Observation       json.RawMessage `json:"observation"`
	ObservationSHA256 string          `json:"observation_sha256"`
	Expectation       json.RawMessage `json:"expectation,omitempty"`
	ExpectationSHA256 string          `json:"expectation_sha256,omitempty"`
	Error             string          `json:"error,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}
