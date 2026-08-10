package cognitionpolicy

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPolicyTerminalizesEveryContradictoryPredispatchProviderOutcome(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name       string
		generation func(CallAttempt) llm.PreparedGeneration
		failure    CallFailureCode
		want       error
	}{
		{
			name: "missing identity evidence",
			generation: func(CallAttempt) llm.PreparedGeneration {
				return llm.PreparedGeneration{Schema: llm.PreparedGenerationSchemaV1}
			},
			failure: CallFailurePolicyAuthority, want: ErrInvalidEvidence,
		},
		{
			name: "successful identity plus error",
			generation: func(attempt CallAttempt) llm.PreparedGeneration {
				value := policyTestPreparedGeneration(attempt, `{}`)
				value.ProviderRequestDispatched = false
				value.ProviderRequestSHA256 = ""
				value.ProviderHTTPStatus = 0
				value.ProviderResponseDisposition = ""
				value.ProviderResponseComplete = false
				value.ProviderResponseBytesKnown = false
				value.ProviderResponseSHA256 = ""
				value.ProviderResponseBytes = 0
				value.ProviderResponseCaptureSHA256 = ""
				value.ProviderResponseCapturedBytes = 0
				value.ProviderResponseCapture = nil
				value.ProviderResponseModel = ""
				value.ProviderDonePresent = false
				value.ProviderDone = false
				value.ProviderDoneReason = ""
				value.UsagePresent = false
				value.Usage = llm.ProviderGenerationUsage{}
				value.Content = ""
				return value
			},
			failure: CallFailurePolicyAuthority, want: ErrInvalidEvidence,
		},
		{
			name: "failed identity probe",
			generation: func(attempt CallAttempt) llm.PreparedGeneration {
				return policyTestFailedProviderIdentityGeneration(attempt)
			},
			failure: CallFailureProviderIdentity, want: ErrProviderIdentity,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			projection := policyTestProjection(t, "predispatch exact provider outcome")
			snapshot, _ := policyTestSnapshot(t, projection)
			brain := policyTestAttestedBrain()
			envelope, err := Render(snapshot, projection, brain.Ref)
			if err != nil {
				t.Fatal(err)
			}
			attempt, err := newCallAttempt(snapshot, brain, envelope)
			if err != nil {
				t.Fatal(err)
			}
			client := &policyTestClient{exactOverride: func(llm.PreparedModel) (llm.PreparedGeneration, error) {
				return testCase.generation(attempt), errors.New("contradictory predispatch error")
			}}
			journal := &policyTestCallJournal{}
			policy, err := New(client, brain, newPolicyTestProjectionLoader(projection), journal)
			if err != nil {
				t.Fatal(err)
			}
			_, decideErr := policy.Decide(context.Background(), snapshot)
			if !errors.Is(decideErr, testCase.want) {
				t.Fatalf("fresh error=%v want %v", decideErr, testCase.want)
			}
			if len(journal.results) != 1 || journal.results[0].FailureCode != testCase.failure {
				t.Fatalf("terminal result=%+v error=%v", journal.results, decideErr)
			}

			result := journal.results[0]
			replayClient := &policyTestClient{exactOverride: func(llm.PreparedModel) (llm.PreparedGeneration, error) {
				t.Fatal("terminal replay called exact provider")
				return llm.PreparedGeneration{}, nil
			}}
			replay, err := New(
				replayClient, brain, newPolicyTestProjectionLoader(projection),
				&policyTestCallJournal{reservation: &CallReservation{
					Attempt: journal.attempts[0], ExistingResult: &result,
				}},
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := replay.Decide(context.Background(), snapshot); !errors.Is(err, testCase.want) {
				t.Fatalf("replay error=%v want %v", err, testCase.want)
			}
			if replayClient.observationCalls != 0 {
				t.Fatal("terminal replay observed provider")
			}
		})
	}
}
