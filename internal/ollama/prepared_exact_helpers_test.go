package ollama

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func exactPreparedRequest(expected llm.ProviderIdentityExpectation) llm.PreparedModel {
	zero := 0.0
	challenge, err := llm.DeriveProviderIdentityObservationChallenge("test-policy-call", expected)
	if err != nil {
		panic(err)
	}
	return llm.PreparedModel{
		Protocol:  llm.ExactPreparedProtocolStructuredV1,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: `{"instruction":"select"}`, PromptHint: llm.MinimalGeneratePrompt,
		MaxOutputTokens: 1024, ContextTokens: expected.NativeContextLimit,
		ResponseFormat:  llm.ResponseFormatJSON,
		ResponseSchema:  map[string]any{"type": "object"},
		ThinkingEnabled: false, Temperature: &zero,
		ProviderIdentityExpectation:  &expected,
		ProviderObservationChallenge: challenge,
	}
}

func exactPreparedIdentityClient(
	t *testing.T,
	expected llm.ProviderIdentityExpectation,
	chatStatus int,
	chatBody string,
	seen map[string]int,
	captured map[string][]byte,
) *Client {
	t.Helper()
	client := New("http://ollama.test", expected.Model, "", 5*time.Second, expected.NativeContextLimit)
	client.httpClient = &http.Client{Transport: identityRoundTripFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		seen[request.URL.Path]++
		if request.Body != nil {
			raw, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			captured[request.URL.Path] = raw
		}
		status, raw := http.StatusOK, []byte(`{"done":true}`)
		switch request.URL.Path {
		case "/api/version":
			raw = []byte(`{"version":"0.24.0"}`)
		case "/api/tags":
			raw = ollamaIdentityModelsJSON(t, expected, false)
		case "/api/show":
			raw = ollamaTokenizerProfileJSON()
		case "/api/ps":
			raw = ollamaIdentityModelsJSON(t, expected, true)
		case "/api/generate":
			if strings.Contains(string(captured[request.URL.Path]), `"raw":true`) {
				status = chatStatus
				raw = []byte(chatBody)
			}
		default:
			status = http.StatusNotFound
		}
		return &http.Response{
			StatusCode: status, Header: make(http.Header), Request: request,
			Body: io.NopCloser(strings.NewReader(string(raw))),
		}, nil
	})}
	return client
}

func exactChatRequestBytes(t *testing.T, prepared llm.PreparedModel) []byte {
	t.Helper()
	raw, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func exactRawBody() string {
	return `{"model":"qwen3.5:9b-q4_K_M","created_at":"2026-08-09T22:00:00Z",` +
		`"response":"{}","done":true,"done_reason":"stop",` +
		`"total_duration":101,"load_duration":11,"prompt_eval_count":41,` +
		`"prompt_eval_duration":21,"eval_count":7,"eval_duration":31}`
}
