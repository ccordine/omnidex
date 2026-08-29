package api

import "testing"

func TestNewServerPreservesBlankProviderAuthority(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{})
	if server.defaultProvider != "" {
		t.Fatalf("blank provider was rewritten as %q", server.defaultProvider)
	}
	if server.providerConfiguration().LLMProvider != "" {
		t.Fatalf("blank provider configuration was rewritten as %q", server.providerConfiguration().LLMProvider)
	}
	payload := server.providerCatalog()
	if payload.ExactStationProvider != "" {
		t.Fatalf("provider catalog invented exact-station provider %q", payload.ExactStationProvider)
	}
	for _, provider := range payload.Providers {
		if provider.SelectedForStations {
			t.Fatalf("provider catalog selected %q for blank authority", provider.ID)
		}
	}
}
