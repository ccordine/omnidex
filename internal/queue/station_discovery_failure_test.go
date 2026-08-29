package queue

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func TestStationDiscoveryReceiptRecordsBoundedObservationRejection(t *testing.T) {
	t.Parallel()
	authority := model.StepAttemptAuthority{
		JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a",
	}
	gap := stationCallTestGap(t, authority)
	prepared := stationCallTestPrepared(t, gap)
	opening, err := validateStationDiscoveryOpening(StationDiscoveryOpenRecord{
		Authority: authority, Gap: gap,
		Selection: llm.ProviderIdentitySelection{
			Model: prepared.ContextModel, NativeContextLimit: prepared.ContextTokens,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	opening.ID = 19
	evidence := stationCallIdentityEvidence(t, *prepared.ProviderIdentityExpectation)
	attestation, err := llm.NewProviderIdentityAttestation(
		*prepared.ProviderIdentityExpectation,
		"ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := llm.NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence,
		strings.Repeat("f", 64),
	)
	if err != nil {
		t.Fatal(err)
	}
	observation, expectation, err := validateStationDiscoveryReceipt(
		StationDiscoveryReceiptRecord{
			Authority: authority, OpeningID: opening.ID, GapID: opening.GapID,
			Observed: observed, FailureReason: StationDiscoveryFailureObservationRejected,
			Error: "provider observation challenge differs from its opening",
		},
		opening,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(expectation) != 0 || !strings.Contains(string(observation), `"failure_reason":"observation_rejected"`) ||
		!strings.Contains(string(observation), `"attestation"`) {
		t.Fatalf("failed discovery did not retain its typed rejection: %s", observation)
	}
}

func TestStationDiscoveryReceiptRejectsUnprovenFailureReason(t *testing.T) {
	t.Parallel()
	authority := model.StepAttemptAuthority{
		JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a",
	}
	gap := stationCallTestGap(t, authority)
	prepared := stationCallTestPrepared(t, gap)
	opening, err := validateStationDiscoveryOpening(StationDiscoveryOpenRecord{
		Authority: authority, Gap: gap,
		Selection: llm.ProviderIdentitySelection{
			Model: prepared.ContextModel, NativeContextLimit: prepared.ContextTokens,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	opening.ID = 19
	observed := stationCallObservedIdentity(t, *prepared.ProviderIdentityExpectation, opening.Challenge)
	for _, reason := range []StationDiscoveryFailureReason{
		"", StationDiscoveryFailureEvidenceRejected, StationDiscoveryFailureObservationRejected,
	} {
		_, _, err := validateStationDiscoveryReceipt(StationDiscoveryReceiptRecord{
			Authority: authority, OpeningID: opening.ID, GapID: opening.GapID,
			Observed: observed, FailureReason: reason, Error: "pretend discovery failure",
		}, opening)
		if err == nil {
			t.Fatalf("unproven discovery failure reason %q was accepted", reason)
		}
	}
	if _, _, err := validateStationDiscoveryReceipt(StationDiscoveryReceiptRecord{
		Authority: authority, OpeningID: opening.ID, GapID: opening.GapID,
		Observed: observed, FailureReason: StationDiscoveryFailureProviderRejected,
		Error: "provider returned an error with a valid observation",
	}, opening); err != nil {
		t.Fatalf("exact provider contract rejection was not representable: %v", err)
	}
}
