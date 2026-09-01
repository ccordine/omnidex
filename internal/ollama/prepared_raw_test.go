package ollama

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPreparedRawCaptureAllowsJSONEscapingBeyondDecodedContentBound(t *testing.T) {
	const escapedByte = `\u0001`
	decodedBytes := (llm.MaxExactPreparedModelContentBytes / len(escapedByte)) + 1
	writes := make(chan error, 1)
	client := New("http://ollama.invalid", "", "", time.Minute)
	client.httpClient.Transport = preparedRawRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/generate" {
			return nil, fmt.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		if _, err := io.Copy(io.Discard, request.Body); err != nil {
			return nil, fmt.Errorf("read prepared request: %w", err)
		}
		reader, writer := io.Pipe()
		go func() {
			writes <- writeEscapedPreparedResponse(writer, escapedByte, decodedBytes)
			close(writes)
		}()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       reader,
			Request:    request,
		}, nil
	})

	temperature := llm.ExactPreparedTemperature(0)
	prepared := llm.PreparedModel{
		Protocol:        llm.ExactPreparedProtocolPlainCompletionV4,
		BaseModel:       "fixture-model",
		ContextModel:    "fixture-model",
		Prompt:          "Return one plain-text result.",
		MaxOutputTokens: 512,
		OutputLimitMode: llm.ExactPreparedOutputLimitExplicit,
		ContextTokens:   llm.MinInferenceContextTokens,
		Temperature:     &temperature,
	}
	generation, err := client.GeneratePreparedExact(
		context.Background(), prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-writes; err != nil {
		t.Fatal(err)
	}
	if generation.ProviderResponseCapturedBytes <= llm.MaxExactPreparedModelContentBytes {
		t.Fatalf(
			"escaped raw capture=%d, want greater than the former 16 MiB conflated bound",
			generation.ProviderResponseCapturedBytes,
		)
	}
	if generation.ProviderResponseCapturedBytes > llm.MaxExactPreparedProviderResponseBytes {
		t.Fatalf("escaped raw capture exceeded raw authority: %d", generation.ProviderResponseCapturedBytes)
	}
	if len(generation.Content) != decodedBytes ||
		len(generation.Content) > llm.MaxExactPreparedModelContentBytes {
		t.Fatalf("decoded content bytes=%d, want %d within semantic bound", len(generation.Content), decodedBytes)
	}
	if _, err := llm.OwnBoundedPreparedGeneration(generation); err != nil {
		t.Fatalf("bounded escaped response could not be owned: %v", err)
	}
	if err := llm.ValidateExactPreparedGenerationForRequest(prepared, generation); err != nil {
		t.Fatalf("bounded escaped response did not validate: %v", err)
	}
}

type preparedRawRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip preparedRawRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return roundTrip(request)
}

func writeEscapedPreparedResponse(
	writer *io.PipeWriter,
	escapedByte string,
	decodedBytes int,
) (err error) {
	defer func() {
		closeErr := writer.CloseWithError(err)
		if err == nil {
			err = closeErr
		}
	}()
	if _, err = io.WriteString(
		writer, `{"created_at":"2026-09-01T12:00:00Z","response":"`,
	); err != nil {
		return err
	}
	chunk := strings.Repeat(escapedByte, 4096)
	for decodedBytes >= 4096 {
		if _, err = io.WriteString(writer, chunk); err != nil {
			return err
		}
		decodedBytes -= 4096
	}
	if _, err = io.WriteString(writer, strings.Repeat(escapedByte, decodedBytes)); err != nil {
		return err
	}
	_, err = io.WriteString(
		writer,
		`","done":true,"done_reason":"stop","total_duration":19,"load_duration":2,"prompt_eval_count":1,"prompt_eval_duration":3,"eval_count":1,"eval_duration":5}`,
	)
	return err
}
