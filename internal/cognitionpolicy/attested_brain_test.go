package cognitionpolicy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

type brainAttestorClient struct {
	*policyTestClient
	observed llm.ObservedProviderIdentity
}

func (client brainAttestorClient) ObserveProviderIdentity(
	_ context.Context,
	_ llm.ProviderIdentityObservationRequest,
) (llm.ObservedProviderIdentity, error) {
	return client.observed, nil
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
	got, err := AttestBrain(
		context.Background(), brainAttestorClient{policyTestClient: &policyTestClient{}, observed: llm.ObservedProviderIdentity{
			Attestation: attestation, Observation: observed.Observation, Evidence: observed.Evidence,
		}}, brain,
	)
	if err != nil || got.Ref != brain || got.Attestation != attestation ||
		got.BootstrapObservation != observed.Observation {
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
