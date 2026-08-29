package llm

import (
	"context"
	"testing"
	"time"
)

type providerDiscoveryTestClient struct {
	observed ObservedProviderIdentity
}

func (client providerDiscoveryTestClient) DiscoverProviderIdentityEvidence(
	_ context.Context,
	_ ProviderIdentitySelection,
	_ string,
) (ObservedProviderIdentity, error) {
	return client.observed, nil
}

func TestRequireDiscoveredProviderIdentityEvidencePreservesRawAuthority(t *testing.T) {
	expected := providerIdentityTestExpectation()
	selection := ProviderIdentitySelection{
		Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
	}
	attestation, err := NewProviderIdentityAttestation(expected, "backend", "installed", "runner")
	if err != nil {
		t.Fatal(err)
	}
	scope := "provider-discovery-test"
	challenge, err := DeriveProviderIdentityDiscoveryChallenge(scope, selection)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := NewObservedProviderIdentity(
		time.Date(2026, 8, 10, 18, 0, 0, 0, time.UTC), attestation,
		providerIdentityTestEvidence(t, expected), challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := RequireDiscoveredProviderIdentityEvidence(
		context.Background(), providerDiscoveryTestClient{observed: observed}, selection, scope,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attestation != observed.Attestation || got.Observation != observed.Observation ||
		got.Evidence.Ref != observed.Evidence.Ref {
		t.Fatalf("observed identity changed: %#v", got)
	}
}

func TestRequireDiscoveredProviderIdentityEvidenceRejectsUnsupportedOrChangedSelection(t *testing.T) {
	expected := providerIdentityTestExpectation()
	selection := ProviderIdentitySelection{
		Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
	}
	if _, err := RequireDiscoveredProviderIdentityEvidence(
		context.Background(), nil, selection, "provider-discovery-test",
	); err == nil {
		t.Fatal("provider without raw discovery authority was accepted")
	}
	changed := selection
	changed.Model = "other-model"
	if _, err := RequireDiscoveredProviderIdentityEvidence(
		context.Background(), providerDiscoveryTestClient{}, changed, "provider-discovery-test",
	); err == nil {
		t.Fatal("provider discovery without raw evidence was accepted")
	}
}
