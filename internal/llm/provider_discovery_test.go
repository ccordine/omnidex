package llm

import (
	"context"
	"strings"
	"testing"
)

type providerDiscoveryTestClient struct {
	Client
	attestation ProviderIdentityAttestation
}

func (client providerDiscoveryTestClient) DiscoverProviderIdentity(
	_ context.Context,
	_ ProviderIdentitySelection,
) (ProviderIdentityAttestation, error) {
	return client.attestation, nil
}

func TestRequireDiscoveredProviderIdentityBindsOnlySelectedFields(t *testing.T) {
	selection := ProviderIdentitySelection{Model: "model:v1", NativeContextLimit: 32768}
	expected := ProviderIdentityExpectation{
		Backend: "provider", BackendVersion: "1.2.3", Model: selection.Model,
		Digest: strings.Repeat("a", 64), Quantization: "Q4",
		NativeContextLimit: selection.NativeContextLimit,
	}
	attestation, err := NewProviderIdentityAttestation(expected, "backend", "installed", "runner")
	if err != nil {
		t.Fatal(err)
	}
	got, err := RequireDiscoveredProviderIdentity(
		context.Background(), providerDiscoveryTestClient{attestation: attestation}, selection,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != attestation {
		t.Fatalf("attestation=%+v want %+v", got, attestation)
	}
}

func TestRequireDiscoveredProviderIdentityRejectsUnsupportedOrChangedSelection(t *testing.T) {
	selection := ProviderIdentitySelection{Model: "model:v1", NativeContextLimit: 32768}
	if _, err := RequireDiscoveredProviderIdentity(
		context.Background(), providerIdentityTestClient{}, selection,
	); err == nil {
		t.Fatal("provider without discovery authority was accepted")
	}
	expected := ProviderIdentityExpectation{
		Backend: "provider", BackendVersion: "1.2.3", Model: "model:v2",
		Digest: strings.Repeat("a", 64), Quantization: "Q4",
		NativeContextLimit: selection.NativeContextLimit,
	}
	attestation, err := NewProviderIdentityAttestation(expected, "backend", "installed", "runner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RequireDiscoveredProviderIdentity(
		context.Background(), providerDiscoveryTestClient{attestation: attestation}, selection,
	); err == nil {
		t.Fatal("provider discovery changed the selected model")
	}
}
