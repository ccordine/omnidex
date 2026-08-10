package ollama

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestGeneratePreparedExactReturnsNativeUsageAndFreshIdentityObservation(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	seen := make(map[string]int)
	captured := make(map[string][]byte)
	client := exactPreparedIdentityClient(t, expected, http.StatusOK, exactRawBody(), seen, captured)
	prepared := exactPreparedRequest(expected)
	result, err := client.GeneratePreparedExact(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if result.Content != `{}` || result.Usage.PromptEvalCount != 41 ||
		result.Usage.EvalCount != 7 || result.ProviderObservation.AttestationSHA256 == "" ||
		result.ProviderIdentityEvidence.Ref != result.ProviderObservation.Evidence {
		t.Fatalf("exact prepared result=%+v", result)
	}
	wantRequest := exactChatRequestBytes(t, prepared)
	if string(captured["/api/generate"]) != string(wantRequest) {
		t.Fatalf("exact provider request=%s want=%s", captured["/api/generate"], wantRequest)
	}
	if !strings.Contains(string(wantRequest), `"raw":true`) ||
		!strings.Contains(string(wantRequest), `"shift":false`) ||
		!strings.Contains(string(wantRequest), `"truncate":false`) {
		t.Fatalf("raw request lacks exact no-truncation authority: %s", wantRequest)
	}
	digest := sha256.Sum256(captured["/api/generate"])
	if result.ProviderRequestSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("prepared generation did not bind the exact raw provider request")
	}
	for endpoint, want := range map[string]int{
		"/api/version": 1, "/api/tags": 1, "/api/show": 1,
		"/api/generate": 2, "/api/ps": 1,
	} {
		if seen[endpoint] != want {
			t.Fatalf("endpoint %s calls=%d want %d", endpoint, seen[endpoint], want)
		}
	}
}

func TestGeneratePreparedExactRejectsMissingOrNegativeNativeUsage(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	for name, body := range map[string]string{
		"missing count":    strings.Replace(exactRawBody(), `,"prompt_eval_count":41`, "", 1),
		"negative count":   strings.Replace(exactRawBody(), `"eval_count":7`, `"eval_count":-1`, 1),
		"missing duration": strings.Replace(exactRawBody(), `,"eval_duration":31`, "", 1),
		"not done":         strings.Replace(exactRawBody(), `"done":true`, `"done":false`, 1),
	} {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := exactPreparedIdentityClient(
				t, expected, http.StatusOK, body, make(map[string]int), make(map[string][]byte),
			)
			partial, err := client.GeneratePreparedExact(
				context.Background(), exactPreparedRequest(expected),
			)
			if err == nil {
				t.Fatal("inexact provider usage was accepted")
			}
			if !partial.ProviderRequestDispatched || partial.ProviderObservation.ObservationSHA256 == "" ||
				partial.ProviderResponseSHA256 == "" || partial.Content != `{}` {
				t.Fatalf("usage failure lost executed provider evidence: %+v", partial)
			}
		})
	}
}

func TestGeneratePreparedExactPreservesCompleteProviderErrorResponse(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	body := `{"error":"busy","prompt_eval_count":4}`
	client := exactPreparedIdentityClient(
		t, expected, http.StatusServiceUnavailable, body,
		make(map[string]int), make(map[string][]byte),
	)
	partial, err := client.GeneratePreparedExact(
		context.Background(), exactPreparedRequest(expected),
	)
	if err == nil {
		t.Fatal("provider HTTP failure was accepted")
	}
	digest := sha256.Sum256([]byte(body))
	wantSHA := hex.EncodeToString(digest[:])
	if !partial.ProviderRequestDispatched || partial.ProviderHTTPStatus != http.StatusServiceUnavailable ||
		partial.ProviderResponseDisposition != llm.ProviderResponseHTTPError ||
		!partial.ProviderResponseComplete || !partial.ProviderResponseBytesKnown ||
		partial.ProviderResponseSHA256 != wantSHA ||
		partial.ProviderResponseCaptureSHA256 != wantSHA ||
		partial.ProviderResponseCapturedBytes != len(body) ||
		partial.Usage != (llm.ProviderGenerationUsage{}) || partial.UsagePresent || partial.Content != "" {
		t.Fatalf("partial provider response evidence=%+v", partial)
	}
}

func TestGeneratePreparedExactKeepsLargeHTTPErrorBodyOutOfReturnedError(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	canary := strings.Repeat("SECRET_PROVIDER_CANARY", 16*1024)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusServiceUnavailable, canary,
		make(map[string]int), make(map[string][]byte),
	)
	partial, err := client.GeneratePreparedExact(
		context.Background(), exactPreparedRequest(expected),
	)
	if err == nil || strings.Contains(err.Error(), "SECRET_PROVIDER_CANARY") || len(err.Error()) > 512 {
		t.Fatalf("provider error leaked raw response body: %q", err)
	}
	if len(partial.ProviderResponseCapture) != len(canary) ||
		partial.ProviderResponseCaptureSHA256 == "" {
		t.Fatal("large HTTP error body was not retained as out-of-line evidence")
	}
}

func TestGeneratePreparedExactRejectsUnknownSuccessfulResponseField(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	body := strings.Replace(exactRawBody(), `"created_at":`, `"unknown":true,"created_at":`, 1)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, body, make(map[string]int), make(map[string][]byte),
	)
	partial, err := client.GeneratePreparedExact(
		context.Background(), exactPreparedRequest(expected),
	)
	if err == nil || partial.ProviderResponseDisposition != llm.ProviderResponseInvalidJSON ||
		partial.Content != "" || partial.ProviderResponseModel != "" ||
		len(partial.ProviderResponseCapture) != len(body) {
		t.Fatalf("unknown raw response field was accepted: result=%+v error=%v", partial, err)
	}
}

func TestGeneratePreparedExactReportsOnlyActualProviderRequestDispatch(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	captured := make(map[string][]byte)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, exactRawBody(), make(map[string]int), captured,
	)
	invalid := exactPreparedRequest(expected)
	invalid.PromptHint = "not the registered exact hint"
	if partial, err := client.GeneratePreparedExact(context.Background(), invalid); err == nil ||
		partial.ProviderRequestDispatched {
		t.Fatalf("pre-dispatch result=%+v error=%v", partial, err)
	}

	client = exactPreparedIdentityClient(
		t, expected, http.StatusOK, exactRawBody(), make(map[string]int), make(map[string][]byte),
	)
	base := client.httpClient.Transport
	client.httpClient.Transport = identityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/api/generate" && request.Body != nil {
			raw, readErr := io.ReadAll(request.Body)
			if readErr != nil {
				return nil, readErr
			}
			request.Body = io.NopCloser(strings.NewReader(string(raw)))
			if strings.Contains(string(raw), `"raw":true`) {
				return nil, errors.New("transport failed after dispatch")
			}
		}
		return base.RoundTrip(request)
	})
	partial, err := client.GeneratePreparedExact(context.Background(), exactPreparedRequest(expected))
	if err == nil || !partial.ProviderRequestDispatched ||
		partial.ProviderResponseDisposition != llm.ProviderResponseTransportError ||
		partial.ProviderRequestSHA256 == "" {
		t.Fatalf("post-dispatch result=%+v error=%v", partial, err)
	}
}

func TestGeneratePreparedExactRejectsRedirectsAndEncodedBodiesWithoutFollowing(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	for _, status := range []int{301, 302, 303, 307, 308} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			client := exactPreparedIdentityClient(
				t, expected, status, `{"redirect":true}`,
				make(map[string]int), make(map[string][]byte),
			)
			base := client.httpClient.Transport
			redirected := 0
			client.httpClient.Transport = identityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Header.Get("Accept-Encoding") != "identity" {
					t.Error("exact generation omitted identity content encoding")
				}
				if request.URL.Path == "/redirected" {
					redirected++
				}
				response, err := base.RoundTrip(request)
				if request.URL.Path == "/api/generate" && response.StatusCode == status {
					response.Header.Set("Location", "/redirected")
				}
				return response, err
			})
			result, err := client.GeneratePreparedExact(
				context.Background(), exactPreparedRequest(expected),
			)
			if err == nil || redirected != 0 || result.ProviderHTTPStatus != status ||
				result.ProviderResponseDisposition != llm.ProviderResponseHTTPError {
				t.Fatalf("redirect result=%+v redirected=%d error=%v", result, redirected, err)
			}
		})
	}

	captured := make(map[string][]byte)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, exactRawBody(), make(map[string]int), captured,
	)
	base := client.httpClient.Transport
	client.httpClient.Transport = identityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		response, err := base.RoundTrip(request)
		if request.URL.Path == "/api/generate" && response.StatusCode == http.StatusOK {
			if strings.Contains(string(captured["/api/generate"]), `"raw":true`) {
				response.Header.Set("Content-Encoding", "gzip")
			}
		}
		return response, err
	})
	result, err := client.GeneratePreparedExact(context.Background(), exactPreparedRequest(expected))
	if err == nil || result.ProviderResponseDisposition != llm.ProviderResponseInvalidJSON ||
		len(result.ProviderResponseCapture) == 0 {
		t.Fatalf("encoded exact result=%+v error=%v", result, err)
	}
}

func TestGeneratePreparedExactRejectsTransformedJSONStringsAndPartialDecode(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	malformed := map[string]string{
		"invalid UTF-8": string(append(
			[]byte(`{"response":"`), append([]byte{0xff}, []byte(`","done":true}`)...)...,
		)),
		"unpaired surrogate":       strings.Replace(exactRawBody(), `"response":"{}"`, `"response":"\uD800"`, 1),
		"trailing malformed usage": `{"response":"{}","done":true,"prompt_eval_count":`,
	}
	for name, body := range malformed {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := exactPreparedIdentityClient(
				t, expected, http.StatusOK, body, make(map[string]int), make(map[string][]byte),
			)
			result, err := client.GeneratePreparedExact(context.Background(), exactPreparedRequest(expected))
			if err == nil || result.ProviderResponseDisposition != llm.ProviderResponseInvalidJSON ||
				result.Content != "" || result.UsagePresent ||
				result.Usage != (llm.ProviderGenerationUsage{}) ||
				!result.ProviderResponseComplete || !result.ProviderResponseBytesKnown {
				t.Fatalf("invalid wrapper result=%+v error=%v", result, err)
			}
		})
	}

	validReplacement := strings.Replace(exactRawBody(), `"response":"{}"`, `"response":"�"`, 1)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, validReplacement, make(map[string]int), make(map[string][]byte),
	)
	result, err := client.GeneratePreparedExact(context.Background(), exactPreparedRequest(expected))
	if err != nil || result.Content != "�" {
		t.Fatalf("literal replacement rune result=%+v error=%v", result, err)
	}
}

func exactPreparedRequest(expected llm.ProviderIdentityExpectation) llm.PreparedModel {
	zero := 0.0
	challenge, err := llm.DeriveProviderIdentityObservationChallenge("test-policy-call", expected)
	if err != nil {
		panic(err)
	}
	return llm.PreparedModel{
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
