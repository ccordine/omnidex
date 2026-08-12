package llm

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

type staticProviderIdentityObserver struct {
	Client
	observed ObservedProviderIdentity
	err      error
}

func (observer staticProviderIdentityObserver) ObserveProviderIdentity(
	context.Context,
	ProviderIdentityObservationRequest,
) (ObservedProviderIdentity, error) {
	return observer.observed, observer.err
}

func TestRequireProviderIdentityObservationTakesBoundedOwnership(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	evidence := providerIdentityTestEvidence(t, expected)
	attestation, err := NewProviderIdentityAttestation(
		expected, "test:version", "test:installed", "test:runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := DeriveProviderIdentityObservationChallenge("ownership", expected)
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence, challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: challenge,
	}
	owned, err := RequireProviderIdentityObservation(
		context.Background(), staticProviderIdentityObserver{observed: source}, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	source.Evidence.Operations[0].ResponseCapture[0] ^= 0xff
	if err := owned.ValidateFor(request); err != nil {
		t.Fatalf("observer mutation changed owned identity: %v", err)
	}
}

func TestRequireProviderIdentityObservationRejectsUnownedOversize(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	request := ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: providerBodySHA256([]byte("ownership-limit")),
	}
	valid := providerIdentityTestEvidence(t, expected)
	for _, testCase := range []struct {
		name   string
		mutate func(*ProviderIdentityEvidence)
	}{
		{name: "operations", mutate: func(value *ProviderIdentityEvidence) {
			value.Operations = make([]ProviderIdentityOperationEvidence, 10_000)
		}},
		{name: "request", mutate: func(value *ProviderIdentityEvidence) {
			value.Operations[0].Request = bytes.Repeat([]byte{'r'}, MaxProviderIdentityComponentBytes+1)
		}},
		{name: "response", mutate: func(value *ProviderIdentityEvidence) {
			value.Operations[0].ResponseCapture = bytes.Repeat([]byte{'s'}, MaxProviderIdentityComponentBytes+2)
		}},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			changed := valid
			changed.Operations = append([]ProviderIdentityOperationEvidence(nil), valid.Operations...)
			testCase.mutate(&changed)
			_, err := RequireProviderIdentityObservation(
				context.Background(), staticProviderIdentityObserver{
					observed: ObservedProviderIdentity{Evidence: changed},
					err:      errors.New("provider identity failed"),
				}, request,
			)
			if err == nil {
				t.Fatal("oversized provider identity observation was accepted")
			}
		})
	}
}
