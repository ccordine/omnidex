package cognitionpolicy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

type providerProcessObserver struct {
	llm.Client
	brain AttestedBrain
	time  time.Time
}

func (observer providerProcessObserver) ObserveProviderIdentity(
	_ context.Context,
	request llm.ProviderIdentityObservationRequest,
) (llm.ObservedProviderIdentity, error) {
	observed, err := policyTestObservedProviderIdentity(
		observer.time, observer.brain.Attestation, request.ChallengeSHA256,
	)
	return observed, err
}

func TestProviderProcessObservationKeepsEpisodeBrainStableAcrossFreshRuns(t *testing.T) {
	t.Parallel()
	brain := policyTestAttestedBrain()
	episode, err := cognition.NewEpisodeRef("episode-process")
	if err != nil {
		t.Fatal(err)
	}
	actor := cognition.AttemptRef{
		JobID: 1, Generation: 2, StepID: 3, Attempt: 4, WorkerID: "worker-process",
	}
	first, err := ObserveProviderProcess(
		context.Background(), providerProcessObserver{
			brain: brain, time: time.Date(2026, 8, 9, 22, 0, 0, 0, time.UTC),
		}, brain, episode, actor, ProviderProcessEpisodeInvocation,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ObserveProviderProcess(
		context.Background(), providerProcessObserver{
			brain: brain, time: time.Date(2026, 8, 9, 22, 1, 0, 0, time.UTC),
		}, brain, episode, actor, ProviderProcessEpisodeInvocation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.Observation.ObservationSHA256 == second.Observation.ObservationSHA256 ||
		first.StableBrain != second.StableBrain {
		t.Fatalf("fresh process observations did not preserve one stable brain: %+v / %+v", first, second)
	}
	changed := first
	changed.Observation.RunnerBodySHA256 = strings.Repeat("f", 64)
	changed.ID = providerProcessObservationID(changed)
	if err := changed.ValidateFor(brain); err == nil {
		t.Fatal("forged process observation body was accepted")
	}
}

func TestStableBrainAuthorityExcludesOnlyBootstrapObservation(t *testing.T) {
	t.Parallel()
	first := policyTestAttestedBrain()
	second := first
	request, err := BootstrapProviderIdentityRequest(second.Ref)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := policyTestObservedProviderIdentity(
		first.BootstrapObservation.ObservedAt.Add(time.Minute), second.Attestation,
		request.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	second.BootstrapObservation = observed.Observation
	if err := second.Validate(); err != nil {
		t.Fatal(err)
	}
	firstStable, err := first.StableAuthority()
	if err != nil {
		t.Fatal(err)
	}
	secondStable, err := second.StableAuthority()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || firstStable != secondStable {
		t.Fatal("bootstrap freshness changed the stable Brain authority")
	}
}
