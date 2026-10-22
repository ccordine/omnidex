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

func TestClientHasNoRequestDeadline(t *testing.T) {
	client := newClient("http://localhost:11434", "", "")
	if client.httpClient.Timeout != 0 {
		t.Fatal("provider imposes a wall-clock deadline")
	}
}

func TestExactGenerationObeysCallerCancellation(t *testing.T) {
	requestCanceled := make(chan struct{})
	client := New("http://fixture.invalid", "", "")
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(20*time.Millisecond, cancel)
	started := time.Now()
	_, err := client.GeneratePreparedExact(
		ctx, prepared,
	)
	if err == nil {
		t.Fatal("generation ignored caller cancellation")
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
