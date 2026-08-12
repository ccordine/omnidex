package cognitionpolicy

import (
	"context"
	"errors"
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

type providerProcessFailureObserver struct {
	llm.Client
	evidence llm.ProviderIdentityEvidence
}

func (observer providerProcessFailureObserver) ObserveProviderIdentity(
	_ context.Context,
	_ llm.ProviderIdentityObservationRequest,
) (llm.ObservedProviderIdentity, error) {
	return llm.ObservedProviderIdentity{Evidence: observer.evidence},
		errors.New("preload identity probe failed")
}

func TestProviderProcessObservationReturnsRawFailureOutcome(t *testing.T) {
	t.Parallel()
	brain := policyTestAttestedBrain()
	evidence := policyTestProviderIdentityFailureEvidence(t, brain.Ref, llm.ProviderIdentityPreload)
	outcome, err := ObserveProviderProcess(
		context.Background(), providerProcessFailureObserver{evidence: evidence}, brain,
		cognition.EpisodeRef{ID: "episode-process-failure"}, cognition.AttemptRef{
			JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker-process-failure",
		}, ProviderProcessEpisodeInvocation,
	)
	if err == nil || outcome.Failure == nil || outcome.Success != nil ||
		outcome.Failure.Receipt.Code != ProviderIdentityObservationFailed ||
		outcome.Failure.ValidateFor(brain) != nil ||
		len(outcome.Failure.IdentityEvidence.Operations) != 5 {
		t.Fatalf("provider process failure outcome=%+v error=%v", outcome, err)
	}
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
	firstOutcome, err := ObserveProviderProcess(
		context.Background(), providerProcessObserver{
			brain: brain, time: time.Date(2026, 8, 9, 22, 0, 0, 0, time.UTC),
		}, brain, episode, actor, ProviderProcessEpisodeInvocation,
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstOutcome.RequireSuccess(brain)
	if err != nil {
		t.Fatal(err)
	}
	secondOutcome, err := ObserveProviderProcess(
		context.Background(), providerProcessObserver{
			brain: brain, time: time.Date(2026, 8, 9, 22, 1, 0, 0, time.UTC),
		}, brain, episode, actor, ProviderProcessEpisodeInvocation,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondOutcome.RequireSuccess(brain)
	if err != nil {
		t.Fatal(err)
	}
	if first.Receipt.ID == second.Receipt.ID ||
		first.Receipt.Observation.ObservationSHA256 == second.Receipt.Observation.ObservationSHA256 ||
		first.Receipt.StableBrain != second.Receipt.StableBrain {
		t.Fatalf("fresh process observations did not preserve one stable brain: %+v / %+v", first, second)
	}
	changed := first.Receipt
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

func TestProviderProcessActivationRejectsFreshHostDriftBeforeProviderCall(t *testing.T) {
	t.Parallel()
	brain := policyTestAttestedBrain()
	brain.Host.CPUIdentitySHA256 = strings.Repeat("f", 64)
	brain.Host.AttestationSHA256 = hostAttestationSHA256(brain.Host)
	outcome, err := ObserveProviderProcess(
		context.Background(), providerProcessObserver{
			brain: brain, time: time.Date(2026, 8, 11, 2, 0, 0, 0, time.UTC),
		}, brain,
		cognition.EpisodeRef{ID: "episode-host-drift"}, cognition.AttemptRef{
			JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker-host-drift",
		}, ProviderProcessEpisodeInvocation,
	)
	if err == nil || !strings.Contains(err.Error(), "process host differs") {
		t.Fatalf("host drift error=%v", err)
	}
	if outcome.Failure == nil ||
		outcome.Failure.Receipt.Code != ProviderHostIdentityMismatch ||
		outcome.Failure.ValidateFor(brain) != nil ||
		len(outcome.Failure.IdentityEvidence.Operations) != 5 {
		t.Fatalf("host drift failure outcome=%+v", outcome)
	}
}

func TestProviderProcessReturnsRawEvidenceWhenHostAttestationFails(t *testing.T) {
	t.Parallel()
	brain := policyTestAttestedBrain()
	episode := cognition.EpisodeRef{ID: "episode-host-probe-failure"}
	actor := cognition.AttemptRef{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker-host-probe-failure",
	}
	outcome, err := observeProviderProcessWithHostAttestor(
		context.Background(), providerProcessObserver{
			brain: brain, time: time.Date(2026, 8, 11, 2, 1, 0, 0, time.UTC),
		}, brain, episode, actor, ProviderProcessEpisodeInvocation,
		func() (HostHardwareAttestation, error) {
			return HostHardwareAttestation{}, errors.New("forced host probe failure")
		},
	)
	if err == nil || outcome.Failure == nil || outcome.Success != nil ||
		outcome.Failure.Receipt.Code != ProviderHostAttestationFailed ||
		outcome.Failure.ValidateFor(brain) != nil ||
		len(outcome.Failure.IdentityEvidence.Operations) != 5 {
		t.Fatalf("process host failure outcome=%+v error=%v", outcome, err)
	}
}

func TestProviderProcessReturnsRawEvidenceWhenStableAttestationChanges(t *testing.T) {
	t.Parallel()
	brain := policyTestAttestedBrain()
	expected, err := brain.Ref.ProviderExpectation()
	if err != nil {
		t.Fatal(err)
	}
	changed, err := llm.NewProviderIdentityAttestation(
		expected, "changed-backend", "changed-installed", "changed-runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	observedBrain := brain
	observedBrain.Attestation = changed
	outcome, observeErr := ObserveProviderProcess(
		context.Background(), providerProcessObserver{
			brain: observedBrain, time: time.Date(2026, 8, 11, 2, 2, 0, 0, time.UTC),
		}, brain, cognition.EpisodeRef{ID: "episode-provider-attestation-drift"},
		cognition.AttemptRef{
			JobID: 1, Generation: 1, StepID: 1, Attempt: 1,
			WorkerID: "worker-provider-attestation-drift",
		}, ProviderProcessEpisodeInvocation,
	)
	if observeErr == nil || outcome.Failure == nil || outcome.Success != nil ||
		outcome.Failure.Receipt.Code != ProviderAttestationIdentityMismatch ||
		outcome.Failure.ValidateFor(brain) != nil ||
		len(outcome.Failure.IdentityEvidence.Operations) != 5 {
		t.Fatalf("provider attestation drift outcome=%+v error=%v", outcome, observeErr)
	}
}
