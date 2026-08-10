package config

import (
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	fixtures := map[string]string{
		"COGNITION_MODEL_SHA256":          strings.Repeat("a", 64),
		"COGNITION_MODEL_QUANTIZATION":    "Q4_K_M",
		"COGNITION_BACKEND_VERSION":       "test-backend-1.0.0",
		"COGNITION_HARDWARE":              "test-hardware",
		"COGNITION_CONTEXT_CEILING_BYTES": "24576",
		"COGNITION_MAX_OUTPUT_TOKENS":     "4096",
	}
	for key, value := range fixtures {
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	os.Exit(m.Run())
}

func TestLoadRequiresEveryCognitionBrainIdentityFieldWithoutDefaults(t *testing.T) {
	for _, key := range []string{
		"COGNITION_MODEL_SHA256", "COGNITION_MODEL_QUANTIZATION",
		"COGNITION_BACKEND_VERSION", "COGNITION_HARDWARE",
		"COGNITION_CONTEXT_CEILING_BYTES", "COGNITION_MAX_OUTPUT_TOKENS",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("WRAPPER_ONLY", "true")
			t.Setenv("LLM_PROVIDER", "ollama")
			t.Setenv("OLLAMA_MODEL", "test-model")
			t.Setenv(key, "")
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("Load() error=%v, want required %s failure", err, key)
			}
		})
	}
}

func TestLoadRejectsDivergentCognitionSamplingAndIdentity(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_MODEL", "test-model")
	for _, test := range []struct{ key, value string }{
		{"COGNITION_MODEL_SHA256", "not-a-digest"},
		{"COGNITION_MODEL_QUANTIZATION", " Q4_K_M "},
		{"COGNITION_MAX_OUTPUT_TOKENS", "999999"},
		{"COGNITION_CONTEXT_CEILING_BYTES", "invalid"},
	} {
		t.Run(test.key, func(t *testing.T) {
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil {
				t.Fatal("Load() accepted invalid cognition identity")
			}
		})
	}
}
