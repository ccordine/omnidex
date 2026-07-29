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

func TestGeneratePreparedStreamReportsGrowingOutput(t *testing.T) {
	streamRequested := false
	numPredict := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		streamRequested, _ = payload["stream"].(bool)
		options, _ := payload["options"].(map[string]any)
		numPredict = int(options["num_predict"].(float64))
		body := strings.Join([]string{
			`{"message":{"role":"assistant","content":"{\"action\":"},"done":false}`,
			`{"message":{"role":"assistant","content":"\"finish\"}"},"done":true}`,
		}, "\n") + "\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/x-ndjson"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})
	client := New("http://ollama.local", "qwen3-coder:30b", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	progress := []int{}
	result, err := client.GeneratePreparedStream(context.Background(), llm.PreparedModel{
		BaseModel:       "qwen3-coder:30b",
		ContextModel:    "qwen3-coder:30b",
		Prompt:          "Return one JSON action.",
		PromptHint:      llm.MinimalGeneratePrompt,
		MaxOutputTokens: 4096,
		ContextTokens:   32768,
		ResponseFormat:  llm.ResponseFormatJSON,
	}, func(update llm.GenerationProgress) error {
		progress = append(progress, update.OutputBytes)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != `{"action":"finish"}` {
		t.Fatalf("result=%q", result)
	}
	if !streamRequested || numPredict != 4096 {
		t.Fatalf("stream=%t num_predict=%d", streamRequested, numPredict)
	}
	if len(progress) != 2 || progress[0] >= progress[1] || progress[1] != len(result) {
		t.Fatalf("progress=%v result_chars=%d", progress, len(result))
	}
}

func TestGeneratePreparedStreamRejectsMissingDoneFrame(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"message":{"role":"assistant","content":"partial"},"done":false}` + "\n",
			)),
			Request: request,
		}, nil
	})
	client := New("http://ollama.local", "qwen3-coder:30b", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	_, err := client.GeneratePreparedStream(context.Background(), llm.PreparedModel{
		BaseModel:       "qwen3-coder:30b",
		Prompt:          "Return one JSON action.",
		MaxOutputTokens: 4096,
		ContextTokens:   32768,
		ResponseFormat:  llm.ResponseFormatJSON,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "before the done frame") {
		t.Fatalf("error=%v", err)
	}
}

func TestGeneratePreparedStreamRawTextHonorsBudgetAndDisablesThinking(t *testing.T) {
	var payload map[string]any
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"message":{"role":"assistant","content":"package main\n"},"done":true}` + "\n",
			)),
			Request: request,
		}, nil
	})
	client := New("http://ollama.local", "qwen3-coder:30b", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	result, err := client.GeneratePreparedStream(context.Background(), llm.PreparedModel{
		BaseModel:       "qwen3-coder:30b",
		Prompt:          "Return exact raw file content.",
		MaxOutputTokens: 4096,
		ContextTokens:   32768,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "package main" {
		t.Fatalf("result=%q", result)
	}
	if _, exists := payload["format"]; exists {
		t.Fatalf("raw file request unexpectedly set format: %#v", payload["format"])
	}
	if think, ok := payload["think"].(bool); !ok || think {
		t.Fatalf("raw file request think=%#v, want false", payload["think"])
	}
	options, _ := payload["options"].(map[string]any)
	if got := int(options["num_predict"].(float64)); got != 4096 {
		t.Fatalf("num_predict=%d want 4096", got)
	}
	if got := options["temperature"].(float64); got != 0 {
		t.Fatalf("temperature=%v want 0", got)
	}
}
