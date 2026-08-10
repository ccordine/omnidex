package cognitionpolicy

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

type brainAttestorClient struct {
	llm.Client
	attestation llm.ProviderIdentityAttestation
}

func (client brainAttestorClient) AttestProviderIdentity(
	_ context.Context,
	_ llm.ProviderIdentityExpectation,
) (llm.ProviderIdentityAttestation, error) {
	return client.attestation, nil
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
	host, err := AttestLocalHostHardware()
	if err != nil {
		t.Fatal(err)
	}
	got, err := AttestBrain(
		context.Background(), brainAttestorClient{attestation: attestation}, brain,
	)
	if err != nil || got.Ref != brain || got.Attestation != attestation {
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
			if _, err := NewAttestedBrain(brain, changed, host); err == nil {
				t.Fatal("mismatched provider attestation was accepted")
			}
		})
	}
}
