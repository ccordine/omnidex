package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestWrapConnectivityErrorAddsDockerHint(t *testing.T) {
	client := &Client{baseURL: "http://host.docker.internal:11434"}
	baseErr := errors.New(`Post "http://host.docker.internal:11434/api/generate": dial tcp 172.17.0.1:11434: connect: connection refused`)

	got := client.wrapConnectivityError(baseErr, "/api/generate")
	if got == nil {
		t.Fatalf("expected wrapped error")
	}
	if !strings.Contains(got.Error(), "OLLAMA_HOST=0.0.0.0:11434") {
		t.Fatalf("expected docker/ollama hint in error, got: %v", got)
	}
}

func TestWrapConnectivityErrorNoHintForOtherHosts(t *testing.T) {
	client := &Client{baseURL: "http://localhost:11434"}
	baseErr := errors.New(`dial tcp 127.0.0.1:11434: connect: connection refused`)

	got := client.wrapConnectivityError(baseErr, "/api/generate")
	if got == nil {
		t.Fatalf("expected error")
	}
	if got.Error() != baseErr.Error() {
		t.Fatalf("expected unchanged error, got: %v", got)
	}
}

func TestGenerateUsesOneDirectChatRequestWithoutEphemeralModel(t *testing.T) {
	requestCount := 0
	model := ""
	messages := []chatMessage{}
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestCount++
		if r.URL.Path != "/api/chat" {
			t.Fatalf("Generate() called forbidden context-model endpoint %q", r.URL.Path)
		}
		var payload chatRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode chat payload: %v", err)
		}
		if payload.Think != nil {
			t.Fatalf("ordinary Generate request unexpectedly forced think=%t", *payload.Think)
		}
		model = payload.Model
		messages = payload.Messages
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","content":"ok"}}`), nil
	})

	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	got, err := client.Generate(context.Background(), "qwen3:14b", "SYSTEM BLOCK\nUSER BLOCK")
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if got != "ok" || requestCount != 1 || model != "qwen3:14b" {
		t.Fatalf("result=%q requests=%d model=%q", got, requestCount, model)
	}
	if len(messages) != 2 || messages[0].Role != "system" || messages[0].Content != "SYSTEM BLOCK\nUSER BLOCK" {
		t.Fatalf("chat messages=%+v", messages)
	}
	if messages[1].Role != "user" || messages[1].Content != minimalGeneratePrompt {
		t.Fatalf("chat user prompt=%+v", messages[1])
	}
}

func TestGenerateFailsDirectlyWithoutCreateOrPullFallback(t *testing.T) {
	requestCount := 0
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestCount++
		if r.URL.Path != "/api/chat" {
			t.Fatalf("Generate() called forbidden fallback endpoint %q", r.URL.Path)
		}
		return jsonResponse(http.StatusNotFound, `{"error":"model not found"}`), nil
	})

	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	_, err := client.Generate(context.Background(), "missing-model", "test")
	if err == nil || !strings.Contains(err.Error(), "ollama chat failed") {
		t.Fatalf("Generate() error=%v", err)
	}
	if requestCount != 1 {
		t.Fatalf("request count=%d, want one direct failure", requestCount)
	}
}

func TestGeneratePreparedRejectsEmptyMessageContent(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","content":""},"done":true}`), nil
	})
	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}

	_, err := client.GeneratePrepared(context.Background(), llm.PreparedModel{
		BaseModel:       "qwen3:4b-thinking",
		ContextModel:    "qwen3:4b-thinking",
		Prompt:          "Return one typed object.",
		PromptHint:      "Begin.",
		MaxOutputTokens: 512,
		ContextTokens:   8192,
		ResponseFormat:  llm.ResponseFormatJSON,
	})
	if err == nil || !strings.Contains(err.Error(), "missing message content") {
		t.Fatalf("GeneratePrepared() error=%v, want explicit empty-content failure", err)
	}
}

func TestChatEnforcesBoundedJSONForControlPlaneContract(t *testing.T) {
	format := ""
	numPredict := 0
	temperature := -1.0
	thinkValue := true
	thinkPresent := false
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode chat payload: %v", err)
		}
		format = asString(payload["format"])
		thinkValue, thinkPresent = payload["think"].(bool)
		options, _ := payload["options"].(map[string]any)
		numPredict = int(options["num_predict"].(float64))
		temperature = options["temperature"].(float64)
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","content":"{}"}}`), nil
	})

	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	_, err := client.Chat(
		context.Background(),
		"qwen2.5-coder:7b",
		"SPECIALIST_INVOCATION:\n{}\n\nCONTROL_PLANE_COMMAND: Return raw JSON.",
		"Begin the control-plane-assigned work now.",
	)
	if err != nil {
		t.Fatal(err)
	}
	if format != "json" || !thinkPresent || thinkValue || numPredict != controlPlaneMaxOutputTokens || temperature != 0 {
		t.Fatalf("format=%q think_present=%t think=%t num_predict=%d temperature=%v", format, thinkPresent, thinkValue, numPredict, temperature)
	}
}

func TestGeneratePreparedUsesRoleSpecificOutputBudget(t *testing.T) {
	numPredict := 0
	numCtx := 0
	format := ""
	thinkValue := true
	thinkPresent := false
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		options := payload["options"].(map[string]any)
		numPredict = int(options["num_predict"].(float64))
		numCtx = int(options["num_ctx"].(float64))
		format = asString(payload["format"])
		thinkValue, thinkPresent = payload["think"].(bool)
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","content":"{}"}}`), nil
	})
	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}

	_, err := client.GeneratePrepared(context.Background(), llm.PreparedModel{
		BaseModel:       "qwen2.5-coder:14b",
		ContextModel:    "qwen2.5-coder:14b",
		Prompt:          "Return one typed object.",
		PromptHint:      "Begin.",
		MaxOutputTokens: 512,
		ContextTokens:   16384,
		ResponseFormat:  llm.ResponseFormatJSON,
	})
	if err != nil {
		t.Fatal(err)
	}
	if numPredict != 512 {
		t.Fatalf("num_predict=%d, want role-specific bound", numPredict)
	}
	if numCtx != 16384 {
		t.Fatalf("num_ctx=%d, want explicit context window", numCtx)
	}
	if format != "json" {
		t.Fatalf("format=%q, want provider-enforced JSON", format)
	}
	if !thinkPresent || thinkValue {
		t.Fatal("structured JSON request must set think=false so hidden reasoning cannot consume the response budget")
	}
}

func TestGeneratePreparedRejectsExhaustedContextBeforeRequest(t *testing.T) {
	requestCount := 0
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestCount++
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","content":"{}"}}`), nil
	})
	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}

	_, err := client.GeneratePrepared(context.Background(), llm.PreparedModel{
		BaseModel:       "qwen2.5-coder:14b",
		ContextModel:    "qwen2.5-coder:14b",
		Prompt:          "CONTROL_PLANE_COMMAND:\n" + strings.Repeat("x", 9000),
		PromptHint:      "Begin.",
		MaxOutputTokens: 512,
		ContextTokens:   8192,
	})
	if err == nil || !strings.Contains(err.Error(), "inference context exhausted before request") {
		t.Fatalf("GeneratePrepared() error=%v, want explicit context exhaustion", err)
	}
	if requestCount != 0 {
		t.Fatalf("request count=%d, want no truncated model request", requestCount)
	}
}

func TestGenerateUsesDerivedPromptHintWhenAuthoritativeInstructionExists(t *testing.T) {
	userPrompt := ""

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload chatRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode chat payload: %v", err)
		}
		userPrompt = payload.Messages[len(payload.Messages)-1].Content
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","content":"ok"}}`), nil
	})

	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}
	prompt := strings.Join([]string{
		"<AUTHORITATIVE_USER_INSTRUCTION_END>",
		"create a test file",
		"</AUTHORITATIVE_USER_INSTRUCTION_END>",
	}, "\n")
	if _, err := client.Generate(context.Background(), "qwen3:14b", prompt); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if userPrompt != "User request: create a test file" {
		t.Fatalf("user prompt=%q want %q", userPrompt, "User request: create a test file")
	}
}

func TestDerivePreparedModelPromptHintFromAuthoritativeInstruction(t *testing.T) {
	input := strings.Join([]string{
		"<USER_INSTRUCTION>",
		"(empty)",
		"</USER_INSTRUCTION>",
		"<AUTHORITATIVE_USER_INSTRUCTION_END>",
		"create a test file in current directory",
		"</AUTHORITATIVE_USER_INSTRUCTION_END>",
	}, "\n")
	got := derivePreparedModelPromptHint(input)
	want := "User request: create a test file in current directory"
	if got != want {
		t.Fatalf("derivePreparedModelPromptHint()=%q want %q", got, want)
	}
}

func TestDerivePreparedModelPromptHintFallback(t *testing.T) {
	got := derivePreparedModelPromptHint("no prompt blocks here")
	if got != minimalGeneratePrompt {
		t.Fatalf("derivePreparedModelPromptHint fallback=%q want %q", got, minimalGeneratePrompt)
	}
}

func asString(value any) string {
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(status int, body string) *http.Response {
	resp := &http.Response{
		StatusCode: status,
		Status:     strconv.Itoa(status) + " " + http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	resp.Header.Set("Content-Type", "application/json")
	return resp
}
