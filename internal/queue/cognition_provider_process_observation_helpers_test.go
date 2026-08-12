package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func providerProcessReceiptForTest(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
) cognitionpolicy.ProviderProcessActivation {
	t.Helper()
	outcome, err := cognitionpolicy.ObserveProviderProcess(
		fixture.Context, cognitionGuardPolicyClient{}, fixture.Start.BrainBootstrap.AttestedBrain,
		cognition.EpisodeRef{ID: fixture.EpisodeID}, cognition.AttemptRef{
			JobID: fixture.Authority.JobID, Generation: fixture.Authority.Generation,
			StepID: fixture.Authority.StepID, Attempt: uint64(fixture.Authority.Attempt),
			WorkerID: fixture.Authority.WorkerID,
		}, cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := outcome.RequireSuccess(fixture.Start.BrainBootstrap.AttestedBrain)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func assertSealedProviderProcessRecord(
	t *testing.T,
	repository *Repository,
	fixture taskGenerationRetirementFixture,
	activation cognitionpolicy.ProviderProcessActivation,
) {
	t.Helper()
	page, err := repository.ReadCognitionSealedTrace(
		fixture.Context, fixture.EpisodeID,
		CognitionTracePageRequest{Limit: MaxCognitionTracePageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range page.Records {
		if record.Kind == "provider_process_observation" && record.ID == activation.Receipt.ID {
			return
		}
	}
	t.Fatalf("sealed trace omitted provider process observation %q", activation.Receipt.ID)
}
