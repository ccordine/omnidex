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
