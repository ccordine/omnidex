package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

type identityRoundTripFunc func(*http.Request) (*http.Response, error)

func (function identityRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestOllamaAttestsExactInstalledAndRunningIdentity(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	seen := make(map[string]int)
	client := ollamaIdentityClient(t, expected, func(request *http.Request) {
		seen[request.URL.Path]++
		if request.URL.Path != "/api/generate" {
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		options, _ := payload["options"].(map[string]any)
		if payload["model"] != expected.Model || payload["prompt"] != nil ||
			int(options["num_ctx"].(float64)) != expected.NativeContextLimit {
			t.Errorf("preload payload=%+v", payload)
		}
	})
	observed, err := client.ObserveProviderIdentity(
		context.Background(), ollamaObservationRequest(t, expected, "attestation"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := observed.Attestation.ValidateFor(expected); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{
		"/api/version", "/api/tags", "/api/show", "/api/generate", "/api/ps",
	} {
		if seen[endpoint] != 1 {
			t.Fatalf("endpoint %s calls=%d want 1", endpoint, seen[endpoint])
		}
	}
}

func TestOllamaObservationBindsEveryLiveResponseBodyAndFreshTime(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	client := ollamaIdentityClient(t, expected, nil)
	first, err := client.ObserveProviderIdentity(
		context.Background(), ollamaObservationRequest(t, expected, "first"),
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.ObserveProviderIdentity(
		context.Background(), ollamaObservationRequest(t, expected, "second"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.ValidateFor(ollamaObservationRequest(t, expected, "first")); err != nil {
		t.Fatal(err)
	}
	if first.Observation.ObservationSHA256 == second.Observation.ObservationSHA256 ||
		!second.Observation.ObservedAt.After(first.Observation.ObservedAt) {
		t.Fatalf("fresh provider observations were indistinguishable: first=%+v second=%+v",
			first.Observation, second.Observation)
	}
	if first.Observation.VersionBodySHA256 == first.Observation.InstalledBodySHA256 ||
		first.Observation.InstalledBodySHA256 == first.Observation.TokenizerBodySHA256 ||
		first.Observation.TokenizerBodySHA256 == first.Observation.PreloadBodySHA256 ||
		first.Observation.PreloadBodySHA256 == first.Observation.RunnerBodySHA256 {
		t.Fatal("provider observation did not bind four distinct raw response bodies")
	}
}

func TestOllamaIdentityObservationNeverFollowsRedirectsOrAcceptsEncodedBodies(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	for _, status := range []int{301, 302, 303, 307, 308} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			client := ollamaIdentityClient(t, expected, nil)
			base := client.httpClient.Transport
			redirected := 0
			client.httpClient.Transport = identityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Header.Get("Accept-Encoding") != "identity" {
					t.Error("identity request omitted exact entity encoding")
				}
				if request.URL.Path == "/redirected" {
					redirected++
				}
				if request.URL.Path == "/api/version" {
					return &http.Response{
						StatusCode: status, Header: http.Header{"Location": []string{"/redirected"}},
						Body: io.NopCloser(strings.NewReader(`{"redirect":true}`)), Request: request,
					}, nil
				}
				return base.RoundTrip(request)
			})
			observed, err := client.ObserveProviderIdentity(
				context.Background(), ollamaObservationRequest(t, expected, "redirect"),
			)
			if err == nil || redirected != 0 || observed.Evidence.Operations[0].HTTPStatus != status ||
				observed.Evidence.Operations[0].Disposition != llm.ProviderIdentityHTTPError {
				t.Fatalf("redirect evidence=%+v redirected=%d error=%v", observed.Evidence, redirected, err)
			}
		})
	}

	client := ollamaIdentityClient(t, expected, nil)
	base := client.httpClient.Transport
	client.httpClient.Transport = identityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		response, err := base.RoundTrip(request)
		if request.URL.Path == "/api/version" {
			response.Header.Set("Content-Encoding", "gzip")
		}
		return response, err
	})
	observed, err := client.ObserveProviderIdentity(
		context.Background(), ollamaObservationRequest(t, expected, "encoded"),
	)
	if err == nil || observed.Evidence.Operations[0].Disposition != llm.ProviderIdentityInvalidJSON {
		t.Fatalf("encoded identity response=%+v error=%v", observed.Evidence, err)
	}
}

func ollamaObservationRequest(
	t *testing.T,
	expected llm.ProviderIdentityExpectation,
	scope string,
) llm.ProviderIdentityObservationRequest {
	t.Helper()
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(scope, expected)
	if err != nil {
		t.Fatal(err)
	}
	return llm.ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: challenge,
	}
}

func TestOllamaDiscoversProviderMaintainedIdentityFromLiveEndpoints(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	client := ollamaIdentityClient(t, expected, nil)
	observed, err := client.DiscoverProviderIdentityEvidence(
		context.Background(),
		llm.ProviderIdentitySelection{
			Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
		},
		strings.Repeat("e", 64),
	)
	if err != nil {
		t.Fatal(err)
	}
	attestation := observed.Attestation
	if observed.Evidence.Validate() != nil || attestation.Backend != expected.Backend ||
		attestation.BackendVersion != expected.BackendVersion ||
		attestation.Digest != expected.Digest ||
		attestation.Quantization != expected.Quantization {
		t.Fatalf("discovered identity=%+v", attestation)
	}
}

func TestOllamaDiscoveryRejectsInstalledRunnerIdentityDrift(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	client := ollamaIdentityClient(t, expected, nil)
	client.httpClient.Transport = identityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		identity := expected
		if request.URL.Path == "/api/ps" {
			identity.Digest = strings.Repeat("b", 64)
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
			raw = ollamaIdentityModelsJSON(t, identity, true)
		case "/api/generate":
		default:
			status = http.StatusNotFound
		}
		return &http.Response{
			StatusCode: status, Header: make(http.Header), Request: request,
			Body: io.NopCloser(strings.NewReader(string(raw))),
		}, nil
	})
	if _, err := client.DiscoverProviderIdentityEvidence(
		context.Background(),
		llm.ProviderIdentitySelection{
			Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
		},
		strings.Repeat("e", 64),
	); err == nil {
		t.Fatal("installed and running model identity drift was accepted")
	}
}

func TestOllamaAttestationRejectsEveryFrozenIdentityMismatch(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*llm.ProviderIdentityExpectation){
		"backend version": func(value *llm.ProviderIdentityExpectation) { value.BackendVersion = "0.25.0" },
		"digest":          func(value *llm.ProviderIdentityExpectation) { value.Digest = strings.Repeat("b", 64) },
		"quantization":    func(value *llm.ProviderIdentityExpectation) { value.Quantization = "Q8_0" },
		"native context":  func(value *llm.ProviderIdentityExpectation) { value.NativeContextLimit = 16384 },
	} {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			actual := ollamaIdentityExpectation()
			client := ollamaIdentityClient(t, actual, nil)
			expected := actual
			mutate(&expected)
			if _, err := client.ObserveProviderIdentity(
				context.Background(), ollamaObservationRequest(t, expected, "mismatch"),
			); err == nil {
				t.Fatal("mismatched live identity was accepted")
			}
		})
	}
}

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
	return []byte(`{"model_info":{"general.architecture":"qwen35",` +
		`"tokenizer.ggml.model":"gpt2","tokenizer.ggml.pre":"qwen35",` +
		`"tokenizer.ggml.add_eos_token":false,"tokenizer.ggml.add_padding_token":false,` +
		`"tokenizer.ggml.tokens":null,"tokenizer.ggml.token_type":null,` +
		`"tokenizer.ggml.merges":null}}`)
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
