package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
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

func TestGenerateUsesContextModelfileAndMinimalPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var mu sync.Mutex
	callOrder := make([]string, 0, 3)
	createModel := ""
	generateModel := ""
	deleteModel := ""
	generatePrompt := ""
	createModelfile := ""
	var createRequestModelFile string

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/create":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create payload: %v", err)
			}
			mu.Lock()
			callOrder = append(callOrder, "create")
			createModel = strings.TrimSpace(asString(payload["name"]))
			createModelfile = asString(payload["modelfile"])
			createRequestModelFile = filepath.Join(home, ".omnidex", "modelfiles", sanitizeModelNameComponent(createModel)+".Modelfile")
			mu.Unlock()
			return jsonResponse(http.StatusOK, `{"status":"ok"}`), nil
		case "/api/generate":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode generate payload: %v", err)
			}
			mu.Lock()
			callOrder = append(callOrder, "generate")
			generateModel = strings.TrimSpace(asString(payload["model"]))
			generatePrompt = asString(payload["prompt"])
			mu.Unlock()
			return jsonResponse(http.StatusOK, `{"response":"ok"}`), nil
		case "/api/delete":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode delete payload: %v", err)
			}
			mu.Lock()
			callOrder = append(callOrder, "delete")
			deleteModel = strings.TrimSpace(asString(payload["name"]))
			mu.Unlock()
			return jsonResponse(http.StatusOK, `{"status":"ok"}`), nil
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
	})

	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second)
	client.httpClient = &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}
	got, err := client.Generate(context.Background(), "qwen3:14b", "SYSTEM BLOCK\nUSER BLOCK")
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("Generate()=%q want ok", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(callOrder, []string{"create", "generate", "delete"}) {
		t.Fatalf("call order=%v want [create generate delete]", callOrder)
	}
	if createModel == "" {
		t.Fatalf("expected create model name")
	}
	if generateModel != createModel {
		t.Fatalf("generate model=%q want %q", generateModel, createModel)
	}
	if deleteModel != createModel {
		t.Fatalf("delete model=%q want %q", deleteModel, createModel)
	}
	if generatePrompt != minimalGeneratePrompt {
		t.Fatalf("generate prompt=%q want %q", generatePrompt, minimalGeneratePrompt)
	}
	if !strings.Contains(createModelfile, "FROM qwen3:14b") {
		t.Fatalf("expected base model in modelfile, got: %q", createModelfile)
	}
	if !strings.Contains(createModelfile, "SYSTEM \"\"\"") || !strings.Contains(createModelfile, "USER BLOCK") {
		t.Fatalf("expected system context in modelfile, got: %q", createModelfile)
	}
	raw, err := os.ReadFile(createRequestModelFile)
	if err != nil {
		t.Fatalf("expected persisted modelfile at %s: %v", createRequestModelFile, err)
	}
	if strings.TrimSpace(string(raw)) != strings.TrimSpace(createModelfile) {
		t.Fatalf("persisted modelfile mismatch:\n got: %q\nwant: %q", strings.TrimSpace(string(raw)), strings.TrimSpace(createModelfile))
	}
}

func TestGenerateStillDeletesContextModelWhenGenerateFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var mu sync.Mutex
	callOrder := make([]string, 0, 3)

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/create":
			mu.Lock()
			callOrder = append(callOrder, "create")
			mu.Unlock()
			return jsonResponse(http.StatusOK, `{"status":"ok"}`), nil
		case "/api/generate":
			mu.Lock()
			callOrder = append(callOrder, "generate")
			mu.Unlock()
			return jsonResponse(http.StatusInternalServerError, "boom"), nil
		case "/api/delete":
			mu.Lock()
			callOrder = append(callOrder, "delete")
			mu.Unlock()
			return jsonResponse(http.StatusOK, `{"status":"ok"}`), nil
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
	})

	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second)
	client.httpClient = &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}
	if _, err := client.Generate(context.Background(), "qwen3:14b", "test"); err == nil {
		t.Fatalf("expected generate error")
	}

	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(callOrder, []string{"create", "generate", "delete"}) {
		t.Fatalf("call order=%v want [create generate delete]", callOrder)
	}
}

func TestGenerateUsesDerivedPromptHintWhenAuthoritativeInstructionExists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	generatePrompt := ""

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/create":
			return jsonResponse(http.StatusOK, `{"status":"ok"}`), nil
		case "/api/generate":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode generate payload: %v", err)
			}
			generatePrompt = asString(payload["prompt"])
			return jsonResponse(http.StatusOK, `{"response":"ok"}`), nil
		case "/api/delete":
			return jsonResponse(http.StatusOK, `{"status":"ok"}`), nil
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
	})

	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second)
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
	if generatePrompt != "User request: create a test file" {
		t.Fatalf("generate prompt=%q want %q", generatePrompt, "User request: create a test file")
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
