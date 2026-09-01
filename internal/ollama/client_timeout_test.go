package ollama

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

type timeoutRoundTripper func(*http.Request) (*http.Response, error)

func (transport timeoutRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return transport(request)
}

func TestClientCannotDisableOrExceedModelRequestTimeout(t *testing.T) {
	for _, test := range []struct {
		name      string
		requested time.Duration
		want      time.Duration
	}{
		{name: "shorter", requested: 9 * time.Minute, want: 9 * time.Minute},
		{name: "maximum", requested: llm.MaximumModelRequestDuration, want: llm.MaximumModelRequestDuration},
		{name: "over maximum", requested: 45 * time.Minute, want: llm.MaximumModelRequestDuration},
		{name: "zero", requested: 0, want: llm.MaximumModelRequestDuration},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newClient("http://localhost:11434", "", "", test.requested)
			if got := client.httpClient.Timeout; got != test.want {
				t.Fatalf("http timeout=%s, want %s", got, test.want)
			}
		})
	}
}

func TestExactGenerationCancelsAtConfiguredRequestTimeout(t *testing.T) {
	requestCanceled := make(chan struct{})
	client := New("http://fixture.invalid", "", "", 20*time.Millisecond)
	client.httpClient.Transport = timeoutRoundTripper(func(
		request *http.Request,
	) (*http.Response, error) {
		<-request.Context().Done()
		close(requestCanceled)
		return nil, request.Context().Err()
	})

	temperature := llm.ExactPreparedTemperature(0)
	prepared := llm.PreparedModel{
		Protocol:        llm.ExactPreparedProtocolPlainCompletionV4,
		BaseModel:       "fixture-model",
		ContextModel:    "fixture-model",
		Prompt:          "Return one plain-text result.",
		MaxOutputTokens: 8,
		OutputLimitMode: llm.ExactPreparedOutputLimitExplicit,
		ContextTokens:   llm.MinInferenceContextTokens,
		Temperature:     &temperature,
	}
	started := time.Now()
	_, err := client.GeneratePreparedExact(
		context.Background(), prepared,
	)
	if err == nil {
		t.Fatal("generation unexpectedly outlived its request timeout")
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("request cancellation took %s, want less than one second", elapsed)
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("provider request context was not canceled")
	}
}
