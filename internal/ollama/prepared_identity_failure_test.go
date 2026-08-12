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

func identityResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
