package queue

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

func validateStationDiscoveryOpening(
	record StationDiscoveryOpenRecord,
) (StationDiscoveryOpening, error) {
	if err := validateStepAttemptAuthority(record.Authority); err != nil {
		return StationDiscoveryOpening{}, err
	}
	if err := record.Selection.Validate(); err != nil {
		return StationDiscoveryOpening{}, err
	}
	if record.Gap.ID < 1 || record.Gap.JobID != record.Authority.JobID ||
		record.Gap.Generation != record.Authority.Generation ||
		record.Gap.StepID != record.Authority.StepID ||
		record.Gap.StepAttempt != record.Authority.Attempt ||
		record.Gap.WorkerID != record.Authority.WorkerID ||
		record.Selection.NativeContextLimit != record.Gap.ContextTokens {
		return StationDiscoveryOpening{}, fmt.Errorf("station discovery differs from its exact gap authority")
	}
	selection, err := exactjson.Canonical(record.Selection)
	if err != nil {
		return StationDiscoveryOpening{}, err
	}
	challenge, err := llm.DeriveProviderIdentityDiscoveryChallenge(
		"station-gap:"+record.Gap.GapID, record.Selection,
	)
	if err != nil {
		return StationDiscoveryOpening{}, err
	}
	return StationDiscoveryOpening{
		GapOpeningID: record.Gap.ID, JobID: record.Authority.JobID,
		Generation: record.Authority.Generation, StepID: record.Authority.StepID,
		StepAttempt: record.Authority.Attempt, WorkerID: record.Authority.WorkerID,
		GapID: record.Gap.GapID, Selection: append(json.RawMessage(nil), selection...),
		SelectionSHA256: stationGapSHA256(string(selection)), Challenge: challenge,
	}, nil
}

func validateStationDiscoveryReceipt(
	record StationDiscoveryReceiptRecord,
	opening StationDiscoveryOpening,
) ([]byte, []byte, error) {
	if err := validateStepAttemptAuthority(record.Authority); err != nil {
		return nil, nil, err
	}
	if record.OpeningID != opening.ID || record.GapID != opening.GapID ||
		opening.JobID != record.Authority.JobID || opening.Generation != record.Authority.Generation ||
		opening.StepID != record.Authority.StepID || opening.StepAttempt != record.Authority.Attempt ||
		opening.WorkerID != record.Authority.WorkerID ||
		len(record.Error) > maxStationCallErrorBytes || record.Error != strings.TrimSpace(record.Error) {
		return nil, nil, fmt.Errorf("station discovery receipt differs from its exact original attempt")
	}
	var selection llm.ProviderIdentitySelection
	if err := json.Unmarshal(opening.Selection, &selection); err != nil {
		return nil, nil, err
	}
	if record.Error != "" {
		if strings.TrimSpace(record.Error) == "" {
			return nil, nil, fmt.Errorf("failed station discovery requires one non-whitespace error")
		}
		if err := record.FailureReason.Validate(); err != nil {
			return nil, nil, err
		}
		expected, deriveErr := llm.DeriveExactProviderIdentityExpectation(record.Observed.Evidence, selection)
		switch record.FailureReason {
		case StationDiscoveryFailureEvidenceRejected:
			if record.Observed.Attestation != (llm.ProviderIdentityAttestation{}) ||
				record.Observed.Observation != (llm.ProviderIdentityObservation{}) {
				return nil, nil, fmt.Errorf("evidence-rejected station discovery cannot claim an observation")
			}
			if err := record.Observed.Evidence.ValidateFailure(selection, nil); err != nil {
				return nil, nil, fmt.Errorf("station discovery failure lacks exact rejected evidence: %w", err)
			}
			observation, err := exactjson.Canonical(struct {
				FailureReason StationDiscoveryFailureReason `json:"failure_reason"`
				Evidence      llm.ProviderIdentityEvidence  `json:"evidence"`
			}{record.FailureReason, record.Observed.Evidence})
			return observation, nil, err
		case StationDiscoveryFailureObservationRejected,
			StationDiscoveryFailureProviderRejected:
			if deriveErr != nil {
				return nil, nil, fmt.Errorf("station discovery observation has no derivable expectation: %w", deriveErr)
			}
			validationErr := record.Observed.ValidateFor(llm.ProviderIdentityObservationRequest{
				Expectation: expected, ChallengeSHA256: opening.Challenge,
			})
			if record.FailureReason == StationDiscoveryFailureObservationRejected && validationErr == nil {
				return nil, nil, fmt.Errorf("station discovery observation rejection is not proven")
			}
			if record.FailureReason == StationDiscoveryFailureProviderRejected && validationErr != nil {
				return nil, nil, fmt.Errorf("provider contract rejection lacks a valid exact observation: %w", validationErr)
			}
			observation, err := exactjson.Canonical(struct {
				FailureReason StationDiscoveryFailureReason   `json:"failure_reason"`
				Attestation   llm.ProviderIdentityAttestation `json:"attestation"`
				Observation   llm.ProviderIdentityObservation `json:"observation"`
				Evidence      llm.ProviderIdentityEvidence    `json:"evidence"`
			}{record.FailureReason, record.Observed.Attestation,
				record.Observed.Observation, record.Observed.Evidence})
			return observation, nil, err
		}
	}
	if record.FailureReason != "" {
		return nil, nil, fmt.Errorf("successful station discovery cannot claim a failure reason")
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(record.Observed.Evidence, selection)
	if err != nil {
		return nil, nil, err
	}
	if err := record.Observed.ValidateFor(llm.ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: opening.Challenge,
	}); err != nil {
		return nil, nil, err
	}
	observation, err := exactjson.Canonical(struct {
		llm.ProviderIdentityObservation
		Evidence llm.ProviderIdentityEvidence `json:"evidence"`
	}{record.Observed.Observation, record.Observed.Evidence})
	if err != nil {
		return nil, nil, err
	}
	expectation, err := exactjson.Canonical(expected)
	return observation, expectation, err
}
