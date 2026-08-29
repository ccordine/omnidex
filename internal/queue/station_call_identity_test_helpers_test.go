package queue

import (
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func openStationCallFixture(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
) (StationGapOpening, llm.PreparedModel, StationCallOpening) {
	t.Helper()
	gapRecord := stationGapOpenFixture(t, authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	return gap, prepared, call
}

func stationCallObservedIdentity(
	t *testing.T,
	expected llm.ProviderIdentityExpectation,
	challenge string,
) llm.ObservedProviderIdentity {
	t.Helper()
	evidence := stationCallIdentityEvidence(t, expected)
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := llm.NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence, challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	return observed
}
