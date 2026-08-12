package cognitionpolicy

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPolicyTerminalizesEveryDispatchedOversizedProviderReturn(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*llm.PreparedGeneration)
	}{
		{
			name: "model content",
			mutate: func(generation *llm.PreparedGeneration) {
				generation.Content = string(bytes.Repeat(
					[]byte{'x'}, MaxModelResponseEvidenceBytes+1,
				))
			},
		},
		{
			name: "raw response capture",
			mutate: func(generation *llm.PreparedGeneration) {
				generation.ProviderResponseCapture = bytes.Repeat(
					[]byte{'y'}, llm.MaxExactPreparedProviderResponseBytes+2,
				)
			},
		},
		{
			name: "identity operation count",
			mutate: func(generation *llm.PreparedGeneration) {
				generation.ProviderIdentityEvidence.Operations = make(
					[]llm.ProviderIdentityOperationEvidence, 10_000,
				)
			},
		},
		{
			name: "identity request component",
			mutate: func(generation *llm.PreparedGeneration) {
				generation.ProviderIdentityEvidence.Operations[0].Request = bytes.Repeat(
					[]byte{'r'}, llm.MaxProviderIdentityComponentBytes+1,
				)
			},
		},
		{
			name: "identity response component",
			mutate: func(generation *llm.PreparedGeneration) {
				generation.ProviderIdentityEvidence.Operations[0].ResponseCapture = bytes.Repeat(
					[]byte{'s'}, llm.MaxProviderIdentityComponentBytes+2,
				)
			},
		},
		{
			name: "identity aggregate",
			mutate: func(generation *llm.PreparedGeneration) {
				shared := bytes.Repeat([]byte{'a'}, 3*1024*1024)
				for index := range generation.ProviderIdentityEvidence.Operations {
					generation.ProviderIdentityEvidence.Operations[index].Request = shared
					generation.ProviderIdentityEvidence.Operations[index].ResponseCapture = shared
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			projection := policyTestProjection(t, "oversized dispatched provider return")
			snapshot, evidence := policyTestSnapshot(t, projection)
			client := &policyTestClient{
				response: policyTestResponse(t, snapshot, evidence),
				generationOverride: func(
					_ llm.PreparedModel,
					generation llm.PreparedGeneration,
				) (llm.PreparedGeneration, error) {
					testCase.mutate(&generation)
					return generation, nil
				},
			}
			journal := &policyTestCallJournal{}
			policy, err := New(
				client, policyTestAttestedBrain(), policyTestActivation(), newPolicyTestProjectionLoader(projection), journal,
			)
			if err != nil {
				t.Fatal(err)
			}
			outcome, decideErr := policy.Decide(context.Background(), snapshot)
			if !outcome.PolicyCallConsumed || !errors.Is(decideErr, ErrInvalidEvidence) ||
				len(journal.results) != 1 ||
				journal.results[0].FailureCode != CallFailureProviderEvidence ||
				len(journal.providerEvidence) != 1 || journal.providerEvidence[0].Validate() != nil {
				t.Fatalf("terminal result=%+v evidence=%+v error=%v", journal.results, journal.providerEvidence, decideErr)
			}

			attempt, result := journal.attempts[0], journal.results[0]
			replayClient := &policyTestClient{err: errors.New("provider replay is forbidden")}
			replay, err := New(
				replayClient, policyTestAttestedBrain(), policyTestActivation(), newPolicyTestProjectionLoader(projection),
				&policyTestCallJournal{reservation: &CallReservation{
					Attempt: attempt, ExistingResult: &result,
				}},
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := replay.Decide(context.Background(), snapshot); !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("replay error=%v", err)
			}
			if replayClient.generateCalls != 0 || replayClient.observationCalls != 0 {
				t.Fatal("terminal overflow replay called the provider")
			}
		})
	}
}
