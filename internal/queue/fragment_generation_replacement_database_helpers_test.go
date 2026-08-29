package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func persistOpenedStationDiscoverySuccess(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
	gap StationGapOpening,
	opening StationDiscoveryOpening,
	prepared llm.PreparedModel,
) StationDiscoveryReceipt {
	t.Helper()
	if prepared.ProviderIdentityExpectation == nil {
		t.Fatal("opened station discovery fixture requires one provider expectation")
	}
	observed := stationCallObservedIdentity(
		t, *prepared.ProviderIdentityExpectation, opening.Challenge,
	)
	receipt, err := repository.RecordStationDiscoveryReceipt(
		t.Context(),
		StationDiscoveryReceiptRecord{
			Authority: authority,
			OpeningID: opening.ID,
			GapID:     gap.GapID,
			Observed:  observed,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}
