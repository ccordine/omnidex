package openai

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestEmbeddingConstructorsRejectTimeoutOutsideHardMaximum(t *testing.T) {
	constructors := map[string]func(time.Duration) (*Client, error){
		"openai": func(timeout time.Duration) (*Client, error) {
			return NewEmbedding(
				"https://api.example.test/v1", "key", "embedding-model", "", "", timeout,
			)
		},
		"compatible": func(timeout time.Duration) (*Client, error) {
			return NewCompatibleEmbedding(
				"compatible", "COMPATIBLE_API_KEY", "https://api.example.test/v1",
				"key", "embedding-model", "", "", timeout,
			)
		},
		"azure": func(timeout time.Duration) (*Client, error) {
			return NewAzureAIEmbedding(
				"https://resource.openai.azure.com", "key", "embedding-model",
				"2024-10-21", "azure_openai", timeout,
			)
		},
	}
	for name, construct := range constructors {
		t.Run(name, func(t *testing.T) {
			for _, invalid := range []time.Duration{
				0, -time.Second, llm.MaximumModelRequestDuration + time.Nanosecond,
			} {
				if _, err := construct(invalid); err == nil ||
					!strings.Contains(err.Error(), "no greater than 30m0s") {
					t.Fatalf("constructor timeout=%s error=%v", invalid, err)
				}
			}
			client, err := construct(llm.MaximumModelRequestDuration)
			if err != nil {
				t.Fatal(err)
			}
			if got := client.httpClient.Timeout; got != llm.MaximumModelRequestDuration {
				t.Fatalf("HTTP timeout=%s want %s", got, llm.MaximumModelRequestDuration)
			}
		})
	}
}
