package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPreparedSourceCorrectionTransmitsCapturedNativeContext(t *testing.T) {
	client := New("http://ollama.invalid", "", "", time.Minute)
	var requests []map[string]json.RawMessage
	client.httpClient.Transport = preparedRawRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/generate" {
			return nil, fmt.Errorf("unexpected endpoint: %s %s", request.Method, request.URL.Path)
		}
		var fields map[string]json.RawMessage
		if err := json.NewDecoder(request.Body).Decode(&fields); err != nil {
			return nil, err
		}
		requests = append(requests, fields)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: request,
			Body: io.NopCloser(strings.NewReader(`{"created_at":"2026-09-12T12:00:00Z","response":"value","done":true,"done_reason":"stop","prompt_eval_count":11,"eval_count":7,"context":[77,18,29]}`))}, nil
	})
	prepared := llm.PreparedModel{Protocol: llm.ExactPreparedProtocolPlainCompletionV4,
		BaseModel: "fixture-model", ContextModel: "fixture-model", Prompt: "Return one implementation body.",
		MaxOutputTokens: -1, OutputLimitMode: llm.ExactPreparedOutputLimitExplicit, ContextTokens: 8192}
	initial, err := client.GeneratePreparedExact(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	prepared.RetainedContext, err = llm.DecodeExactPreparedRetainedContext(prepared.Protocol,
		initial.ProviderResponseCapture, prepared.ContextTokens)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Prompt = "Which local value satisfies the requirement?\n\nwrong"
	if _, err := client.GeneratePreparedExact(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests=%d", len(requests))
	}
	if _, exists := requests[0]["context"]; exists {
		t.Fatal("initial request inherited context")
	}
	var prompt string
	if err := json.Unmarshal(requests[1]["prompt"], &prompt); err != nil {
		t.Fatal(err)
	}
	if string(requests[1]["context"]) != "[77,18,29]" || prompt != prepared.Prompt ||
		string(requests[1]["raw"]) != "false" || string(requests[1]["truncate"]) != "false" ||
		string(requests[1]["shift"]) != "false" {
		t.Fatalf("correction request=%v", requests[1])
	}
}
