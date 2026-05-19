package odn

import (
	"bytes"
	"context"
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

func TestRunShellCommandUsesPipefail(t *testing.T) {
	_, stderr, err := runShellCommand(context.Background(), t.TempDir(), "printf 'not-json' | jq -r '.datetime'")

	if err == nil {
		t.Fatal("expected pipeline to fail when jq fails")
	}
	if !strings.Contains(stderr, "parse error") {
		t.Fatalf("stderr = %q, want jq parse error", stderr)
	}
}
