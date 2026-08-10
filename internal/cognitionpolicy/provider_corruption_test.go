package cognitionpolicy

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/exactjson"
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
			name: "oversized top-level content encoding receipt",
			mutate: func(value *llm.PreparedGeneration) {
				value.ProviderContentEncoding.CapturedBase64 = strings.Repeat(
					"x", maxProviderContentEncodingBase64Bytes+2,
				)
			},
			failure: CallFailureProviderEvidence, want: []error{ErrInvalidEvidence},
		},
		{
			name: "oversized nested identity content encoding receipts",
			mutate: func(value *llm.PreparedGeneration) {
				for index := range value.ProviderIdentityEvidence.Operations {
					value.ProviderIdentityEvidence.Operations[index].ContentEncoding.CapturedBase64 =
						strings.Repeat("y", maxProviderContentEncodingBase64Bytes+2)
				}
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
			name: "changed raw identity takes precedence over request mismatch",
			mutate: func(value *llm.PreparedGeneration) {
				operation := value.ProviderIdentityEvidence.Operations[1]
				changedBody := bytes.ReplaceAll(
					operation.ResponseCapture,
					[]byte(policyTestBrain().Digest), []byte(strings.Repeat("c", 64)),
				)
				changed, err := llm.NewProviderIdentityOperationEvidence(
					operation.Operation, operation.Method, operation.Endpoint,
					operation.RequestDispatched, operation.Request, operation.HTTPStatus,
					operation.Disposition, operation.ResponseComplete,
					operation.ContentEncoding, changedBody,
				)
				if err != nil {
					panic(err)
				}
				operations := value.ProviderIdentityEvidence.Clone().Operations
				operations[1] = changed
				evidence, err := llm.NewProviderIdentityEvidence(operations)
				if err != nil {
					panic(err)
				}
				value.ProviderIdentityEvidence = evidence
				value.ProviderObservation.InstalledBodySHA256 = policySHA256(string(changedBody))
				value.ProviderObservation.Evidence = evidence.Ref
				value.ProviderObservation.ObservationSHA256 = ""
				raw, err := exactjson.Canonical(value.ProviderObservation)
				if err != nil {
					panic(err)
				}
				value.ProviderObservation.ObservationSHA256 = policySHA256(string(raw))
				value.ProviderRequestSHA256 = strings.Repeat("d", 64)
			},
			failure: CallFailureProviderEvidence, want: []error{ErrInvalidEvidence},
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
			failure:     CallFailureProviderEvidence, want: []error{ErrInvalidEvidence},
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
			if journal.providerEvidence[0].Validate() != nil ||
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
