package cognitionpolicy

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPolicyPersistsWhitespaceOnlyProviderOutputAndReplaysWithoutProvider(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "empty provider output")
	snapshot, _ := policyTestSnapshot(t, projection)
	client := &policyTestClient{
		generationOverride: func(
			_ llm.PreparedModel,
			generation llm.PreparedGeneration,
		) (llm.PreparedGeneration, error) {
			generation.Content = " \t \n"
			generation.ProviderResponseDisposition = llm.ProviderResponseEmptyContent
			policyTestRefreshRawProviderResponse(&generation, true)
			return generation, errors.New("provider returned no model output")
		},
	}
	journal := &policyTestCallJournal{}
	policy, err := New(
		client, policyTestAttestedBrain(), policyTestActivation(), newPolicyTestProjectionLoader(projection), journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), snapshot); !errors.Is(err, ErrGeneration) {
		t.Fatalf("fresh error=%v want ErrGeneration", err)
	}
	if len(journal.results) != 1 || len(journal.evidence) != 1 ||
		journal.results[0].FailureCode != CallFailureGeneration ||
		journal.results[0].ProviderResponseDisposition != llm.ProviderResponseEmptyContent ||
		string(journal.evidence[0].Content) != " \t \n" {
		t.Fatalf("whitespace response was not persisted exactly: %+v / %+v", journal.results, journal.evidence)
	}

	result := journal.results[0]
	replayClient := &policyTestClient{err: errors.New("provider must not be called")}
	replay, err := New(
		replayClient, policyTestAttestedBrain(), policyTestActivation(), newPolicyTestProjectionLoader(projection),
		&policyTestCallJournal{reservation: &CallReservation{
			Attempt: journal.attempts[0], ExistingResult: &result,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := replay.Decide(context.Background(), snapshot); !errors.Is(err, ErrGeneration) {
		t.Fatalf("replay error=%v want ErrGeneration", err)
	}
	if replayClient.generateCalls != 0 || replayClient.observationCalls != 0 {
		t.Fatal("terminal empty-content replay called the provider")
	}
}
