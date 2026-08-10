package cognitionpolicy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPolicyTerminallyRecordsEveryDispatchedMalformedProviderReturn(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name        string
		mutate      func(*llm.PreparedGeneration)
		providerErr error
		failure     CallFailureCode
		want        []error
	}{
		{
			name: "valid observation malformed receipt",
			mutate: func(value *llm.PreparedGeneration) {
				value.ProviderResponseSHA256 = ""
			},
			failure: CallFailureProviderEvidence, want: []error{ErrInvalidEvidence},
		},
		{
			name: "normalized content differs from raw capture",
			mutate: func(value *llm.PreparedGeneration) {
				value.Content = `{"substituted":true}`
			},
			failure: CallFailureProviderEvidence, want: []error{ErrInvalidEvidence},
		},
		{
			name: "raw response names another model",
			mutate: func(value *llm.PreparedGeneration) {
				value.ProviderResponseModel = "another-model"
				policyTestRefreshRawProviderResponse(value, true)
			},
			failure: CallFailureProviderEvidence, want: []error{ErrInvalidEvidence},
		},
		{
			name: "malformed receipt and provider error",
			mutate: func(value *llm.PreparedGeneration) {
				value.ProviderResponseSHA256 = ""
			},
			providerErr: errors.New("provider also returned an error"),
			failure:     CallFailureProviderEvidence, want: []error{ErrInvalidEvidence},
		},
		{
			name: "request mismatch and malformed receipt",
			mutate: func(value *llm.PreparedGeneration) {
				value.ProviderRequestSHA256 = strings.Repeat("d", 64)
				value.ProviderResponseSHA256 = ""
			},
			failure: CallFailureProviderRequest,
			want:    []error{ErrGeneration, ErrInvalidEvidence},
		},
		{
			name: "invalid observation and malformed receipt",
			mutate: func(value *llm.PreparedGeneration) {
				value.ProviderObservation.ChallengeSHA256 = strings.Repeat("e", 64)
				value.ProviderResponseSHA256 = ""
			},
			failure: CallFailureProviderEvidence, want: []error{ErrInvalidEvidence},
		},
		{
			name:        "successful receipt and contradictory provider error",
			mutate:      func(*llm.PreparedGeneration) {},
			providerErr: errors.New("provider contradicted its successful receipt"),
			failure:     CallFailurePolicyAuthority, want: []error{ErrInvalidEvidence},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			projection := policyTestProjection(t, "malformed provider return")
			snapshot, evidence := policyTestSnapshot(t, projection)
			client := &policyTestClient{
				response: policyTestResponse(t, snapshot, evidence),
				generationOverride: func(
					_ llm.PreparedModel,
					generation llm.PreparedGeneration,
				) (llm.PreparedGeneration, error) {
					testCase.mutate(&generation)
					return generation, testCase.providerErr
				},
			}
			journal := &policyTestCallJournal{}
			policy, err := New(
				client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), journal,
			)
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := policy.Decide(context.Background(), snapshot)
			for _, want := range testCase.want {
				if !errors.Is(err, want) {
					t.Fatalf("error=%v want %v", err, want)
				}
			}
			if !outcome.ProviderRequestDispatched || len(journal.results) != 1 ||
				journal.results[0].FailureCode != testCase.failure ||
				len(journal.providerEvidence) != 1 {
				t.Fatalf("terminal result/evidence=%+v / %+v", journal.results, journal.providerEvidence)
			}
			if testCase.failure == CallFailurePolicyAuthority {
				if journal.providerEvidence[0].Ref != (ProviderGenerationEvidenceRef{}) ||
					len(journal.providerEvidence[0].Generation) != 0 {
					t.Fatal("trusted authority failure unexpectedly used opaque provider evidence")
				}
			} else if journal.providerEvidence[0].Validate() != nil ||
				journal.results[0].ProviderGenerationEvidence != journal.providerEvidence[0].Ref {
				t.Fatal("malformed provider result lacks exact opaque evidence")
			}

			attempt, result := journal.attempts[0], journal.results[0]
			replayClient := &policyTestClient{err: errors.New("provider must not be called")}
			replayJournal := &policyTestCallJournal{reservation: &CallReservation{
				Attempt: attempt, ExistingResult: &result,
			}}
			replay, err := New(
				replayClient, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection),
				replayJournal,
			)
			if err != nil {
				t.Fatal(err)
			}
			_, replayErr := replay.Decide(context.Background(), snapshot)
			for _, want := range testCase.want {
				if !errors.Is(replayErr, want) {
					t.Fatalf("replay error=%v want %v", replayErr, want)
				}
			}
			if replayClient.generateCalls != 0 || replayClient.observationCalls != 0 {
				t.Fatal("durable malformed provider outcome called the provider during replay")
			}
		})
	}
}
