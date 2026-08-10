package cognitionpolicy

import (
	"context"
	"errors"
	"testing"
)

func TestPolicyRejectsPreparedIdentityDriftWithoutPlainGenerateFallback(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	client := &policyTestClient{changePreparedIdentity: true}
	journal := &policyTestCallJournal{}
	policy, err := New(client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), snapshot); !errors.Is(err, ErrGeneration) {
		t.Fatalf("prepared identity error=%v, want ErrGeneration", err)
	}
	if client.prepareCalls != 1 || client.generateCalls != 0 ||
		client.plainGenerateCalls != 0 || client.cleanupCalls != 1 {
		t.Fatalf("calls prepare=%d prepared=%d plain=%d cleanup=%d",
			client.prepareCalls, client.generateCalls, client.plainGenerateCalls, client.cleanupCalls)
	}
}

func TestPolicyRejectsClientMutationOfExactPreparedContract(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, evidence := policyTestSnapshot(t, projection)
	client := &policyTestClient{
		response: policyTestResponse(t, snapshot, evidence), mutateContract: true,
	}
	journal := &policyTestCallJournal{}
	policy, err := New(client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), snapshot); !errors.Is(err, ErrGeneration) {
		t.Fatalf("mutated contract error=%v, want ErrGeneration", err)
	}
	if client.generateCalls != 1 || client.plainGenerateCalls != 0 || client.cleanupCalls != 1 ||
		len(journal.results) != 1 || journal.results[0].Status != CallResultFailed {
		t.Fatalf("calls/results prepared=%d plain=%d cleanup=%d results=%+v",
			client.generateCalls, client.plainGenerateCalls, client.cleanupCalls, journal.results)
	}
}
