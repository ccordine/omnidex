package cognitionpolicy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

type brainAttestorClient struct {
	*policyTestClient
	observed llm.ObservedProviderIdentity
	err      error
}

func (client brainAttestorClient) ObserveProviderIdentity(
	_ context.Context,
	_ llm.ProviderIdentityObservationRequest,
) (llm.ObservedProviderIdentity, error) {
	return client.observed, client.err
}

func TestAttestBrainReturnsRawProviderFailureOutcome(t *testing.T) {
	t.Parallel()
	brain := policyTestBrain()
	evidence := policyTestProviderIdentityFailureEvidence(t, brain, llm.ProviderIdentityTokenizer)
	outcome, err := AttestBrain(
		context.Background(), brainAttestorClient{
			policyTestClient: &policyTestClient{},
			observed:         llm.ObservedProviderIdentity{Evidence: evidence},
			err:              errors.New("tokenizer identity probe failed"),
		}, brain,
	)
	if err == nil || outcome.Failure == nil || outcome.Success != nil ||
		outcome.Failure.Receipt.Code != ProviderIdentityObservationFailed ||
		outcome.Failure.Validate() != nil ||
		len(outcome.Failure.IdentityEvidence.Operations) != 5 {
		t.Fatalf("bootstrap failure outcome=%+v error=%v", outcome, err)
	}
}

func TestBrainBootstrapFailureRejectsProcessOnlyHostCodes(t *testing.T) {
	t.Parallel()
	brain := policyTestBrain()
	request, err := BootstrapProviderIdentityRequest(brain)
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := llm.NewProviderIdentityAttestation(
		request.Expectation, "test:/version", "test:/installed", "test:/runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := policyTestObservedProviderIdentity(
		time.Date(2026, 8, 11, 1, 1, 0, 0, time.UTC),
		attestation, request.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newBrainBootstrapFailure(
		brain, request, observed, ProviderHostIdentityMismatch,
	); err == nil {
		t.Fatal("bootstrap failure accepted a process-only host mismatch code")
	}
}

func TestAttestBrainReturnsRawEvidenceWhenHostAttestationFails(t *testing.T) {
	t.Parallel()
	brain := policyTestBrain()
	request, err := BootstrapProviderIdentityRequest(brain)
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := llm.NewProviderIdentityAttestation(
		request.Expectation, "test:/version", "test:/installed", "test:/runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := policyTestObservedProviderIdentity(
		time.Date(2026, 8, 11, 1, 2, 0, 0, time.UTC),
		attestation, request.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := attestBrainWithHostAttestor(
		context.Background(), brainAttestorClient{
			policyTestClient: &policyTestClient{}, observed: observed,
		}, brain, func() (HostHardwareAttestation, error) {
			return HostHardwareAttestation{}, errors.New("forced host probe failure")
		},
	)
	if err == nil || outcome.Failure == nil || outcome.Success != nil ||
		outcome.Failure.Receipt.Code != ProviderHostAttestationFailed ||
		outcome.Failure.Validate() != nil ||
		outcome.Failure.IdentityEvidence.Ref != observed.Evidence.Ref {
		t.Fatalf("host failure outcome=%+v error=%v", outcome, err)
	}
}

func TestAttestedBrainRequiresLiveProviderEvidence(t *testing.T) {
	t.Parallel()
	brain := policyTestBrain()
	expected, err := brain.ProviderExpectation()
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err := BootstrapProviderIdentityRequest(brain)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := policyTestObservedProviderIdentity(
		time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC), attestation,
		request.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	host, err := AttestLocalHostHardware()
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := AttestBrain(
		context.Background(), brainAttestorClient{policyTestClient: &policyTestClient{}, observed: llm.ObservedProviderIdentity{
			Attestation: attestation, Observation: observed.Observation, Evidence: observed.Evidence,
		}}, brain,
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := outcome.RequireSuccess()
	if err != nil || got.AttestedBrain.Ref != brain ||
		got.AttestedBrain.Attestation != attestation ||
		got.AttestedBrain.BootstrapObservation != observed.Observation ||
		got.BootstrapEvidence.Ref != observed.Evidence.Ref {
		t.Fatalf("attested brain=%+v error=%v", got, err)
	}
	for name, mutate := range map[string]func(*llm.ProviderIdentityAttestation){
		"backend":      func(value *llm.ProviderIdentityAttestation) { value.Backend = "other-backend" },
		"version":      func(value *llm.ProviderIdentityAttestation) { value.BackendVersion = "2.0.0" },
		"model":        func(value *llm.ProviderIdentityAttestation) { value.Model = "other:model" },
		"digest":       func(value *llm.ProviderIdentityAttestation) { value.Digest = strings.Repeat("a", 64) },
		"quantization": func(value *llm.ProviderIdentityAttestation) { value.Quantization = "q8_0" },
	} {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			changed := attestation
			mutate(&changed)
			if _, err := NewAttestedBrain(brain, changed, observed.Observation, host); err == nil {
				t.Fatal("mismatched provider attestation was accepted")
			}
		})
	}
	changedObservation := observed.Observation
	changedObservation.PreloadBodySHA256 = strings.Repeat("f", 64)
	if _, err := NewAttestedBrain(brain, attestation, changedObservation, host); err == nil {
		t.Fatal("changed bootstrap provider operation was accepted")
	}
}

func policyTestProviderIdentityFailureEvidence(
	t *testing.T,
	brain BrainRef,
	failing llm.ProviderIdentityOperation,
) llm.ProviderIdentityEvidence {
	t.Helper()
	request, err := BootstrapProviderIdentityRequest(brain)
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := llm.NewProviderIdentityAttestation(
		request.Expectation, "test:/version", "test:/installed", "test:/runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := policyTestObservedProviderIdentity(
		time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
		attestation, request.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	operations := observed.Evidence.Clone().Operations
	failed := false
	for index := range operations {
		operation := operations[index]
		if operation.Operation == failing {
			operations[index], err = llm.NewProviderIdentityOperationEvidence(
				operation.Operation, operation.Method, operation.Endpoint,
				llm.ProviderRequestDispatched, operation.Request, 200,
				llm.ProviderIdentityInvalidJSON, true,
				llm.NewProviderContentEncodingEvidence(nil, false), []byte(`{`),
			)
			failed = true
			continue
		}
		if failed {
			operations[index], err = llm.NewProviderIdentityOperationEvidence(
				operation.Operation, operation.Method, operation.Endpoint,
				llm.ProviderRequestNotDispatched, operation.Request, 0,
				llm.ProviderIdentityNotDispatched, false,
				llm.ProviderContentEncodingEvidence{}, nil,
			)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	evidence, err := llm.NewProviderIdentityEvidence(operations)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}
