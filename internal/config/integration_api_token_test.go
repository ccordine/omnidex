package config

import (
	"strings"
	"testing"
)

func TestIntegrationAPITokenIsOptionalButExactWhenConfigured(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OMNIDEX_INTEGRATION_API_TOKEN", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IntegrationAPIToken != "" {
		t.Fatalf("unconfigured token=%q", cfg.IntegrationAPIToken)
	}

	for _, invalid := range []string{"short", " " + testConfigIntegrationToken, testConfigIntegrationToken + "\n"} {
		t.Setenv("OMNIDEX_INTEGRATION_API_TOKEN", invalid)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OMNIDEX_INTEGRATION_API_TOKEN") {
			t.Fatalf("invalid token %q error=%v", invalid, err)
		}
	}

	t.Setenv("OMNIDEX_INTEGRATION_API_TOKEN", testConfigIntegrationToken)
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IntegrationAPIToken != testConfigIntegrationToken {
		t.Fatal("configured integration token was not preserved exactly")
	}
}

const testConfigIntegrationToken = "integration-token-0123456789abcdef"
