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
	if result.Content != `semantic leaf` || result.Usage.PromptEvalCount != 41 ||
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
			if partial.ProviderRequestDisposition != llm.ProviderRequestDispatched ||
				partial.ProviderObservation.ObservationSHA256 == "" ||
				partial.ProviderResponseSHA256 == "" || partial.Content != `semantic leaf` {
				t.Fatalf("usage failure lost executed provider evidence: %+v", partial)
			}
		})
	}
}

func TestGeneratePreparedExactPreservesCompleteProviderErrorResponse(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	body := `{"error":"busy","prompt_eval_count":4}`
	seen := make(map[string]int)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusServiceUnavailable, body,
		seen, make(map[string][]byte),
	)
	partial, err := client.GeneratePreparedExact(
		context.Background(), exactPreparedRequest(expected),
	)
	if err == nil {
		t.Fatal("provider HTTP failure was accepted")
	}
	digest := sha256.Sum256([]byte(body))
	wantSHA := hex.EncodeToString(digest[:])
	if partial.ProviderRequestDisposition != llm.ProviderRequestDispatched ||
		partial.ProviderHTTPStatus != http.StatusServiceUnavailable ||
		partial.ProviderResponseDisposition != llm.ProviderResponseHTTPError ||
		!partial.ProviderResponseComplete || !partial.ProviderResponseBytesKnown ||
		partial.ProviderResponseSHA256 != wantSHA ||
		partial.ProviderResponseCaptureSHA256 != wantSHA ||
		partial.ProviderResponseCapturedBytes != len(body) ||
		partial.Usage != (llm.ProviderGenerationUsage{}) || partial.UsagePresent || partial.Content != "" {
		t.Fatalf("partial provider response evidence=%+v", partial)
	}
	if seen["/api/generate"] != 2 {
		t.Fatalf(
			"HTTP failure dispatched %d generate requests; want one identity probe and one exact request",
			seen["/api/generate"],
		)
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

func TestGeneratePreparedExactPreservesButDoesNotProjectProviderMetadata(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	body := strings.Replace(
		exactRawBody(), `"created_at":`,
		`"context":[1,"opaque",{"token":2}],"unknown":{"provider":true},"created_at":`, 1,
	)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, body, make(map[string]int), make(map[string][]byte),
	)
	result, err := client.GeneratePreparedExact(
		context.Background(), exactPreparedRequest(expected),
	)
	if err != nil {
		t.Fatalf("provider metadata blocked exact generation: %v", err)
	}
	if result.Content != `semantic leaf` || result.ProviderResponseModel != expected.Model ||
		string(result.ProviderResponseCapture) != body ||
		result.ProviderResponseCapturedBytes != len(body) {
		t.Fatalf("provider metadata changed normalized generation or raw capture: %+v", result)
	}
}

func TestGeneratePreparedExactRejectsNonEmptyProviderThinking(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	body := strings.Replace(
		exactRawBody(), "{", `{"thinking":"private trace",`, 1,
	)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, body, make(map[string]int), make(map[string][]byte),
	)
	result, err := client.GeneratePreparedExact(
		context.Background(), exactPreparedRequest(expected),
	)
	if err == nil || !strings.Contains(err.Error(), "forbidden separate thinking content") ||
		result.ProviderResponseDisposition != llm.ProviderResponseInvalidJSON ||
		result.Content != "" || string(result.ProviderResponseCapture) != body {
		t.Fatalf("non-empty provider thinking result=%+v error=%v", result, err)
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
		partial.ProviderRequestDisposition != "" {
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
	if err == nil || partial.ProviderRequestDisposition != llm.ProviderRequestNotDispatched ||
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
		"unpaired surrogate": strings.Replace(
			exactRawBody(), `"response":"semantic leaf"`, `"response":"\uD800"`, 1,
		),
		"trailing malformed usage": `{"response":"semantic leaf","done":true,"prompt_eval_count":`,
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

	validReplacement := strings.Replace(
		exactRawBody(), `"response":"semantic leaf"`, `"response":"�"`, 1,
	)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, validReplacement, make(map[string]int), make(map[string][]byte),
	)
	result, err := client.GeneratePreparedExact(context.Background(), exactPreparedRequest(expected))
	if err != nil || result.Content != "�" {
		t.Fatalf("literal replacement rune result=%+v error=%v", result, err)
	}
}
