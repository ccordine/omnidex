package llm

import (
	"context"
	"strings"
	"testing"
)

type providerIdentityTestClient struct {
	Client
	attestation ProviderIdentityAttestation
}

func (client providerIdentityTestClient) AttestProviderIdentity(
	_ context.Context,
	_ ProviderIdentityExpectation,
) (ProviderIdentityAttestation, error) {
	return client.attestation, nil
}

func TestProviderIdentityAttestationBindsEveryLiveField(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	attestation, err := NewProviderIdentityAttestation(
		expected, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	client := providerIdentityTestClient{attestation: attestation}
	got, err := RequireProviderIdentityAttestation(context.Background(), client, expected)
	if err != nil || got != attestation {
		t.Fatalf("attestation=%+v error=%v", got, err)
	}
	changed := expected
	changed.Digest = strings.Repeat("b", 64)
	if _, err := RequireProviderIdentityAttestation(context.Background(), client, changed); err == nil {
		t.Fatal("attestation accepted another model digest")
	}
}

func TestProviderIdentityAttestationRejectsUnsupportedClient(t *testing.T) {
	t.Parallel()
	if _, err := RequireProviderIdentityAttestation(
		context.Background(), providerIdentityTestClient{}.Client,
		providerIdentityTestExpectation(),
	); err == nil {
		t.Fatal("client without live identity capability was accepted")
	}
}

func providerIdentityTestExpectation() ProviderIdentityExpectation {
	return ProviderIdentityExpectation{
		Backend: "ollama", BackendVersion: "0.24.0", Model: "qwen:9b",
		Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M", NativeContextLimit: 32768,
	}
}
