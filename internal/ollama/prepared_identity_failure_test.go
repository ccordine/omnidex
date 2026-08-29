package ollama

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestGeneratePreparedExactIdentityFailureNeverDispatchesGeneration(t *testing.T) {
	tests := map[string]identityRoundTripFunc{
		"transport": func(*http.Request) (*http.Response, error) {
			return nil, errors.New("identity transport failed")
		},
		"http": func(*http.Request) (*http.Response, error) {
			return identityResponse(http.StatusServiceUnavailable, `{}`), nil
		},
		"invalid body": func(*http.Request) (*http.Response, error) {
			return identityResponse(http.StatusOK, `{`), nil
		},
	}
	for name, transport := range tests {
		t.Run(name, func(t *testing.T) {
			expected := ollamaIdentityExpectation()
			client := New(
				"http://ollama.test", expected.Model, "", 5*time.Second,
				expected.NativeContextLimit,
			)
			generateRequests := 0
			client.httpClient.Transport = identityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path == "/api/generate" {
					generateRequests++
				}
				return transport(request)
			})
			prepared := exactPreparedRequest(expected)
			generation, err := client.GeneratePreparedExact(context.Background(), prepared)
			if err == nil || generation.ProviderRequestDisposition != llm.ProviderRequestNotDispatched ||
				generation.ProviderIdentityEvidence.Validate() != nil || generateRequests != 0 {
				t.Fatalf("generation=%+v generate_requests=%d error=%v", generation, generateRequests, err)
			}
		})
	}
}

func TestGeneratePreparedExactIdentityDriftReturnsEvidenceWithoutObservation(t *testing.T) {
	expected := ollamaIdentityExpectation()
	seen := make(map[string]int)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, exactRawBody(), seen, make(map[string][]byte),
	)
	base := client.httpClient.Transport
	rawGenerationCalls := 0
	client.httpClient.Transport = identityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/api/version" {
			return identityResponse(http.StatusOK, `{"version":"0.24.1"}`), nil
		}
		if request.URL.Path == "/api/generate" {
			raw, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			request.Body = io.NopCloser(strings.NewReader(string(raw)))
			if strings.Contains(string(raw), `"raw":true`) {
				rawGenerationCalls++
			}
		}
		return base.RoundTrip(request)
	})
	prepared := exactPreparedRequest(expected)
	generation, err := client.GeneratePreparedExact(context.Background(), prepared)
	selection := llm.ProviderIdentitySelection{
		Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
	}
	if err == nil || generation.ProviderRequestDisposition != llm.ProviderRequestNotDispatched ||
		generation.ProviderObservation != (llm.ProviderIdentityObservation{}) || rawGenerationCalls != 0 ||
		generation.ProviderIdentityEvidence.ValidateFailure(selection, &expected) != nil {
		t.Fatalf("generation=%+v raw_calls=%d error=%v", generation, rawGenerationCalls, err)
	}
}

func identityResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
