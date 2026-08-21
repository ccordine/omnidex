package ollama

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestResolveRoleplayRawContextUsesExactShowRequest(t *testing.T) {
	client := New("http://ollama.test", "", "", 0, 8192)
	seen := 0
	client.httpClient.Transport = identityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		seen++
		if request.Method != http.MethodPost || request.URL.Path != "/api/show" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		got, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		selection := llm.ProviderIdentitySelection{
			Model: "tinydolphin:latest", NativeContextLimit: 8192,
			ProfilePolicy: llm.ProviderIdentityProfileRoleplayRawCompletion,
		}
		want, err := llm.ExactProviderTokenizerRequestBytes(selection)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("show request=%s want %s", got, want)
		}
		show := `{"capabilities":["completion"],"parameters":"temperature 0.7","template":"{{ .Prompt }}","model_info":{"general.architecture":"llama","llama.context_length":4096,"tokenizer.ggml.model":"llama"}}`
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header), Request: request,
			Body: io.NopCloser(strings.NewReader(show)),
		}, nil
	})

	got, err := client.ResolveRoleplayRawContext(context.Background(), "tinydolphin:latest", 8192)
	if err != nil {
		t.Fatal(err)
	}
	if got != 4096 || seen != 1 {
		t.Fatalf("resolved context=%d requests=%d", got, seen)
	}
}
