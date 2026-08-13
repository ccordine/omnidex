package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestResearchStatusReportsOnlyDeterministicWebCapability(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/research", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"generation_provider", "embedding_provider", "research_runnable", "ollama", "warnings"} {
		if _, exists := raw[removed]; exists {
			t.Fatalf("broad research status field %q remains: %s", removed, rec.Body.String())
		}
	}
	if _, exists := raw["component"]; exists {
		t.Fatalf("duplicate legacy research component field remains: %s", rec.Body.String())
	}
	var payload researchStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.WebSearch.Enabled {
		t.Fatalf("unconfigured web specialist reported available: %+v", payload.WebSearch)
	}
	if !strings.Contains(payload.HTML.Bundle, `data-recyclr-target="research-status-output"`) ||
		!strings.Contains(payload.HTML.Bundle, "not configured") {
		t.Fatalf("status component=%s", payload.HTML.Bundle)
	}
}

func TestResearchStatusRejectsUnregisteredProviderWithoutNetworkFallback(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		WebSearchProviders: []string{"metadata.internal.example"},
	})
	if _, err := server.collectResearchStatus(); err == nil || !strings.Contains(err.Error(), "unregistered provider") {
		t.Fatalf("status error=%v", err)
	}
}

func TestResearchStatusOwnsNoOutboundHTTPCapability(t *testing.T) {
	raw, err := os.ReadFile("research_status.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"http.DefaultClient", "probeHTTP(", `"https://" +`, "CheckRedirect"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("research status retained outbound network authority %q", forbidden)
		}
	}
}

func TestConfiguredWebSearchProvidersHasNoImplicitFallback(t *testing.T) {
	got, err := configuredWebSearchProviders(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("implicit providers=%v", got)
	}
	got, err = configuredWebSearchProviders([]string{" reddit ", "reddit", "google"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "reddit" || got[1] != "google" {
		t.Fatalf("providers=%v", got)
	}
}

func TestNormalizeURLStripsTrailingDotHost(t *testing.T) {
	if got := normalizeURL("http://172.20.0.1.:11434"); got != "http://172.20.0.1:11434" {
		t.Fatalf("normalizeURL()=%q", got)
	}
}
