package omni

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDeterministicCommandLines(t *testing.T) {
	got := extractDeterministicCommandLines("pwd\n$ ls -la")
	want := []string{"pwd", "ls -la"}

	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("command %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExtractDeterministicCommandLinesDoesNotParseNaturalLanguage(t *testing.T) {
	got := extractDeterministicCommandLines("run pwd\nexecute ls -la")
	if len(got) != 0 {
		t.Fatalf("commands = %#v, want none", got)
	}
}

func TestExecuteLinuxCommandToolWithoutOllamaRunsExplicitCommand(t *testing.T) {
	runLogger, err := NewRunLogger(t.TempDir(), "test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer runLogger.Close()

	result, err := ExecuteLinuxCommandTool(
		context.Background(),
		nil,
		"pwd",
		PermissionFull,
		strings.NewReader(""),
		&bytes.Buffer{},
		t.TempDir(),
		func() string { return "evt" },
		runLogger,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExecutedCount != 1 {
		t.Fatalf("executed = %d, want 1; result = %#v", result.ExecutedCount, result)
	}
	if result.BlockedCount != 0 || result.FailedCount != 0 {
		t.Fatalf("blocked=%d failed=%d, want 0/0", result.BlockedCount, result.FailedCount)
	}
}

func TestExecuteLinuxCommandToolDoesNotExecuteFallbackAfterOllamaFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	workspacePath := t.TempDir()
	runLogger, err := NewRunLogger(t.TempDir(), "test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer runLogger.Close()

	result, err := ExecuteLinuxCommandTool(
		context.Background(),
		NewOllamaClient(server.URL, "test-model"),
		"touch should-not-exist",
		PermissionFull,
		strings.NewReader(""),
		&bytes.Buffer{},
		workspacePath,
		func() string { return "evt" },
		runLogger,
	)
	if err == nil {
		t.Fatalf("expected Ollama failure, got result %#v", result)
	}
	if !strings.Contains(err.Error(), "linux_command generation failed") {
		t.Fatalf("error = %q, want explicit generation failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(workspacePath, "should-not-exist")); !os.IsNotExist(statErr) {
		t.Fatalf("fallback command ran after Ollama failure: %v", statErr)
	}
}

func TestRunShellCommandUsesPipefail(t *testing.T) {
	_, stderr, err := runShellCommand(context.Background(), t.TempDir(), "printf 'not-json' | jq -r '.datetime'")

	if err == nil {
		t.Fatal("expected pipeline to fail when jq fails")
	}
	if !strings.Contains(stderr, "parse error") {
		t.Fatalf("stderr = %q, want jq parse error", stderr)
	}
}

func TestEvaluateCommandPolicyAllowsNPXAsDeclaredPackageRunner(t *testing.T) {
	decision := EvaluateCommandPolicy("npx create-react-app note-app", t.TempDir())
	if !decision.Allowed {
		t.Fatalf("npx was advertised by the runtime but rejected by policy: %#v", decision)
	}
}
