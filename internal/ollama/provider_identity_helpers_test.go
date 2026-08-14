package ollama

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func ollamaIdentityClient(
	t *testing.T,
	identity llm.ProviderIdentityExpectation,
	observe func(*http.Request),
) *Client {
	t.Helper()
	return &Client{
		baseURL: "http://ollama.test", defaultModel: identity.Model,
		contextTokens: identity.NativeContextLimit,
		httpClient: &http.Client{Transport: identityRoundTripFunc(func(
			request *http.Request,
		) (*http.Response, error) {
			if observe != nil {
				observe(request)
			}
			status, raw := http.StatusOK, []byte{}
			switch request.URL.Path {
			case "/api/version":
				raw = []byte(`{"version":"0.24.0"}`)
			case "/api/tags":
				raw = ollamaIdentityModelsJSON(t, identity, false)
			case "/api/show":
				raw = ollamaTokenizerProfileJSON()
			case "/api/generate":
				raw = []byte(`{"done":true}`)
			case "/api/ps":
				raw = ollamaIdentityModelsJSON(t, identity, true)
			default:
				status, raw = http.StatusNotFound, []byte(`{"error":"not found"}`)
			}
			return &http.Response{
				StatusCode: status, Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader(string(raw))), Request: request,
			}, nil
		})},
	}
}

func ollamaTokenizerProfileJSON() []byte {
	raw, err := json.Marshal(map[string]any{
		"capabilities": []string{"completion", "vision", "tools", "thinking"},
		"model_info": map[string]any{
			"general.architecture":             "qwen35",
			"tokenizer.ggml.model":             "gpt2",
			"tokenizer.ggml.pre":               "qwen35",
			"tokenizer.ggml.add_eos_token":     false,
			"tokenizer.ggml.add_padding_token": false,
			"tokenizer.ggml.tokens":            nil,
			"tokenizer.ggml.token_type":        nil,
			"tokenizer.ggml.merges":            nil,
		},
		"template": "{{ .Prompt }}",
		"parameters": "temperature                    1\n" +
			"top_k                          20\n" +
			"top_p                          0.95\n" +
			"presence_penalty               1.5",
	})
	if err != nil {
		panic(err)
	}
	return raw
}

func ollamaIdentityModelsJSON(
	t *testing.T,
	identity llm.ProviderIdentityExpectation,
	running bool,
) []byte {
	t.Helper()
	model := map[string]any{
		"name": identity.Model, "model": identity.Model, "digest": identity.Digest,
		"size":        int64(6_594_474_711),
		"modified_at": time.Date(2026, 8, 8, 0, 35, 46, 0, time.FixedZone("EDT", -4*60*60)),
		"details": map[string]any{
			"parent_model": "", "format": "gguf", "family": "qwen35",
			"families": []string{"qwen35"}, "parameter_size": "9.7B",
			"quantization_level": identity.Quantization,
		},
	}
	if running {
		model["context_length"] = identity.NativeContextLimit
		model["size"] = int64(14_524_483_488)
		model["size_vram"] = int64(7_165_128_704)
		model["expires_at"] = time.Date(2026, 8, 9, 17, 22, 58, 0, time.FixedZone("EDT", -4*60*60))
		delete(model, "modified_at")
	}
	raw, err := json.Marshal(map[string]any{"models": []any{model}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func ollamaIdentityExpectation() llm.ProviderIdentityExpectation {
	return llm.ProviderIdentityExpectation{
		Backend: "ollama", BackendVersion: "0.24.0", Model: "qwen3.5:9b-q4_K_M",
		Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M", NativeContextLimit: 32768,
		TokenizerProfile: llm.ExactPreparedTokenizerProfile,
	}
}
