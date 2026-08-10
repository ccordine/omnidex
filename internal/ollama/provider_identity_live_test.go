package ollama

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestLiveOllamaProviderIdentity(t *testing.T) {
	baseURL := os.Getenv("OMNIDEX_TEST_OLLAMA_URL")
	if baseURL == "" {
		t.Skip("OMNIDEX_TEST_OLLAMA_URL is not set")
	}
	contextLimit, err := strconv.Atoi(os.Getenv("OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if err != nil || contextLimit <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	expected := llm.ProviderIdentityExpectation{
		Backend:            ollamaProviderBackend,
		BackendVersion:     os.Getenv("OMNIDEX_TEST_OLLAMA_VERSION"),
		Model:              os.Getenv("OMNIDEX_TEST_OLLAMA_MODEL"),
		Digest:             os.Getenv("OMNIDEX_TEST_OLLAMA_DIGEST"),
		Quantization:       os.Getenv("OMNIDEX_TEST_OLLAMA_QUANTIZATION"),
		NativeContextLimit: contextLimit,
	}
	if err := expected.Validate(); err != nil {
		t.Fatal(err)
	}
	client := New(baseURL, expected.Model, "", 5*time.Minute, contextLimit)
	attestation, err := client.AttestProviderIdentity(context.Background(), expected)
	if err != nil {
		t.Fatal(err)
	}
	if err := attestation.ValidateFor(expected); err != nil {
		t.Fatal(err)
	}
}
