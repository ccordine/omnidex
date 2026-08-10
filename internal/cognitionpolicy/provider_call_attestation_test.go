package cognitionpolicy

import (
	"context"
	"strings"
	"testing"

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
	policy, err := New(client, brain, loader, journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), firstSnapshot); err != nil {
		t.Fatalf("first decision: %v", err)
	}
	if _, err := policy.Decide(context.Background(), secondSnapshot); err == nil {
		t.Fatal("changed live provider identity reached a second generation")
	}
	if client.attestationCalls != 2 || client.generateCalls != 1 || client.prepareCalls != 1 {
		t.Fatalf(
			"attest=%d generate=%d prepare=%d, want 2/1/1",
			client.attestationCalls, client.generateCalls, client.prepareCalls,
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
		firstClient, policyTestAttestedBrain(),
		newPolicyTestProjectionLoader(projection), firstJournal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Decide(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	attempt, result := firstJournal.attempts[0], firstJournal.results[0]
	replayClient := &policyTestClient{}
	replayJournal := &policyTestCallJournal{reservation: &CallReservation{
		Attempt: attempt, ExistingResult: &result, Created: false,
	}}
	replay, err := New(
		replayClient, policyTestAttestedBrain(),
		newPolicyTestProjectionLoader(projection), replayJournal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := replay.Decide(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if replayClient.attestationCalls != 0 || replayClient.generateCalls != 0 ||
		replayClient.prepareCalls != 0 {
		t.Fatalf(
			"replay touched provider: attest=%d prepare=%d generate=%d",
			replayClient.attestationCalls, replayClient.prepareCalls, replayClient.generateCalls,
		)
	}
}
