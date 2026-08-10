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
	attestation, err := client.AttestProviderIdentity(context.Background(), expected)
	if err != nil {
		t.Fatal(err)
	}
	if err := attestation.ValidateFor(expected); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"/api/version", "/api/tags", "/api/generate", "/api/ps"} {
		if seen[endpoint] != 1 {
			t.Fatalf("endpoint %s calls=%d want 1", endpoint, seen[endpoint])
		}
	}
}

func TestOllamaDiscoversProviderMaintainedIdentityFromLiveEndpoints(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	client := ollamaIdentityClient(t, expected, nil)
	attestation, err := client.DiscoverProviderIdentity(
		context.Background(),
		llm.ProviderIdentitySelection{
			Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if attestation.Backend != expected.Backend ||
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
	if _, err := client.DiscoverProviderIdentity(
		context.Background(),
		llm.ProviderIdentitySelection{
			Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
		},
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
			if _, err := client.AttestProviderIdentity(context.Background(), expected); err == nil {
				t.Fatal("mismatched live identity was accepted")
			}
		})
	}
}

func TestOllamaAttestationRejectsAmbiguousProviderJSON(t *testing.T) {
	t.Parallel()
	for name, testCase := range map[string]struct {
		raw    string
		target any
	}{
		"duplicate version": {`{"version":"0.24.0","version":"0.25.0"}`, &versionResponse{}},
		"version alias":     {`{"Version":"0.24.0"}`, &versionResponse{}},
		"duplicate digest": {
			`{"models":[{"name":"m","model":"m","digest":"a","digest":"b","details":{"quantization_level":"Q4"}}]}`,
			&tagsResponse{},
		},
		"quantization alias": {
			`{"models":[{"name":"m","model":"m","digest":"a","details":{"Quantization_Level":"Q4"}}]}`,
			&tagsResponse{},
		},
		"unregistered runner field": {
			`{"models":[{"name":"m","model":"m","size":1,"digest":"a","details":{},"expires_at":"2026-08-09T17:22:58-04:00","size_vram":1,"context_length":32768,"hidden_runner_label":"x"}]}`,
			&runningModelsResponse{},
		},
	} {
		testCase := testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := &Client{
				baseURL: "http://ollama.test",
				httpClient: &http.Client{Transport: identityRoundTripFunc(func(
					request *http.Request,
				) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK, Header: make(http.Header), Request: request,
						Body: io.NopCloser(strings.NewReader(testCase.raw)),
					}, nil
				})},
			}
			if err := client.attestationJSON(
				context.Background(), http.MethodGet, "/identity", nil, testCase.target,
			); err == nil {
				t.Fatal("ambiguous provider identity JSON was accepted")
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
	}
}
