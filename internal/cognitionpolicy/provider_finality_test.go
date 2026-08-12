package cognitionpolicy

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPolicyDurablyRejectsNonfinalProviderResponseAndReplaysWithoutProvider(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name   string
		mutate func(*llm.PreparedGeneration)
	}{
		{
			name: "done false",
			mutate: func(generation *llm.PreparedGeneration) {
				generation.ProviderDone = false
			},
		},
		{
			name: "done missing",
			mutate: func(generation *llm.PreparedGeneration) {
				generation.ProviderDonePresent = false
				generation.ProviderDone = false
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			projection := policyTestProjection(t, "provider finality authority")
			snapshot, evidence := policyTestSnapshot(t, projection)
			client := &policyTestClient{
				response: policyTestResponse(t, snapshot, evidence),
				generationOverride: func(
					_ llm.PreparedModel,
					generation llm.PreparedGeneration,
				) (llm.PreparedGeneration, error) {
					testCase.mutate(&generation)
					policyTestRefreshRawProviderResponse(&generation, true)
					return generation, errors.New("provider response was not final")
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
			if !outcome.PolicyCallConsumed || !errors.Is(decideErr, ErrProviderUsage) ||
				len(journal.results) != 1 || journal.results[0].FailureCode != CallFailureProviderUsage {
				t.Fatalf("result=%+v error=%v", journal.results, decideErr)
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
			if _, err := replay.Decide(context.Background(), snapshot); !errors.Is(err, ErrProviderUsage) {
				t.Fatalf("replay error=%v", err)
			}
			if replayClient.generateCalls != 0 || replayClient.observationCalls != 0 {
				t.Fatal("durable provider finality failure called provider during replay")
			}
		})
	}
}
