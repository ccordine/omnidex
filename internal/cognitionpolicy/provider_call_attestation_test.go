package cognitionpolicy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

func TestPolicyReattestsExactProviderBeforeEveryFreshGeneration(t *testing.T) {
	t.Parallel()
	firstProjection := policyTestProjection(t, "first provider-bound desk")
	secondProjection := policyTestProjection(t, "second provider-bound desk")
	firstSnapshot, evidence := policyTestSnapshot(t, firstProjection)
	secondSnapshot, _ := policyTestSnapshot(t, secondProjection)
	loader := newPolicyTestProjectionLoader(firstProjection)
	loader.projections[secondSnapshot.ContextProjection().ID] = cloneProjection(secondProjection)

	brain := policyTestAttestedBrain()
	changedExpectation, err := brain.Ref.ProviderExpectation()
	if err != nil {
		t.Fatal(err)
	}
	changedExpectation.Digest = strings.Repeat("c", 64)
	changed, err := llm.NewProviderIdentityAttestation(
		changedExpectation, "changed:/version", "changed:/installed", "changed:/runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	client := &policyTestClient{
		response:     policyTestResponse(t, firstSnapshot, evidence),
		attestations: []llm.ProviderIdentityAttestation{brain.Attestation, changed},
	}
	journal := &policyTestCallJournal{}
	policy, err := New(
		client, brain, policyTestDefaultProviderProcessActivation(brain), loader, journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), firstSnapshot); err != nil {
		t.Fatalf("first decision: %v", err)
	}
	if _, err := policy.Decide(context.Background(), secondSnapshot); err == nil {
		t.Fatal("changed live provider identity reached a second generation")
	}
	if client.observationCalls != 2 || client.generateCalls != 1 || client.prepareCalls != 2 {
		t.Fatalf(
			"observe=%d generate=%d prepare=%d, want 2/1/2",
			client.observationCalls, client.generateCalls, client.prepareCalls,
		)
	}
	if len(journal.results) != 2 || journal.results[1].FailureCode != CallFailureProviderIdentity {
		t.Fatalf("provider identity failure was not sealed exactly: %#v", journal.results)
	}
}

func TestPolicyReplayPerformsNoProviderIdentityProbe(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "replayed provider-bound desk")
	snapshot, evidence := policyTestSnapshot(t, projection)
	firstClient := &policyTestClient{response: policyTestResponse(t, snapshot, evidence)}
	firstJournal := &policyTestCallJournal{}
	first, err := New(
		firstClient, policyTestAttestedBrain(), policyTestActivation(),
		newPolicyTestProjectionLoader(projection), firstJournal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Decide(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	attempt, result, responseEvidence := firstJournal.attempts[0], firstJournal.results[0], firstJournal.evidence[0]
	replayClient := &policyTestClient{}
	replayJournal := &policyTestCallJournal{reservation: &CallReservation{
		Attempt: attempt, ExistingResult: &result, ExistingResponseEvidence: &responseEvidence,
		Created: false,
	}}
	replay, err := New(
		replayClient, policyTestAttestedBrain(), policyTestActivation(),
		newPolicyTestProjectionLoader(projection), replayJournal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := replay.Decide(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if replayClient.observationCalls != 0 || replayClient.generateCalls != 0 ||
		replayClient.prepareCalls != 0 {
		t.Fatalf(
			"replay touched provider: observe=%d prepare=%d generate=%d",
			replayClient.observationCalls, replayClient.prepareCalls, replayClient.generateCalls,
		)
	}
}

func TestProviderRawIdentityMustDeriveTheFrozenBrainExpectation(t *testing.T) {
	t.Parallel()
	attempt := policyTestCallAttempt(t)
	expected, err := attempt.Brain.ProviderExpectation()
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := callProviderObservationChallenge(attempt, expected)
	if err != nil {
		t.Fatal(err)
	}
	changed := expected
	changed.Digest = strings.Repeat("c", 64)
	changedAttestation, err := llm.NewProviderIdentityAttestation(
		changed, "changed:/version", "changed:/installed", "changed:/runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := policyTestObservedProviderIdentity(
		time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC), changedAttestation, challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	forged := observed.Observation
	forged.AttestationSHA256 = attempt.ProviderAttestation.AttestationSHA256
	forged.ObservationSHA256 = ""
	raw, err := exactjson.Canonical(forged)
	if err != nil {
		t.Fatal(err)
	}
	forged.ObservationSHA256 = policySHA256(string(raw))
	if err := forged.ValidateFor(attempt.ProviderAttestation, challenge); err != nil {
		t.Fatalf("forged normalized observation is self-consistent: %v", err)
	}
	if err := validateObservedProviderForAttempt(attempt, forged, observed.Evidence); err == nil {
		t.Fatal("raw provider identity for another model digest was accepted")
	}
}
