package codexrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookPathInEnvAcceptsAbsoluteExecutable(t *testing.T) {
	tmp := t.TempDir()
	codexPath := filepath.Join(tmp, "codex")
	if err := os.WriteFile(codexPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := lookPathInEnv(codexPath, []string{"PATH=/does/not/exist"})
	if err != nil {
		t.Fatalf("expected absolute executable to resolve: %v", err)
	}
	if got != codexPath {
		t.Fatalf("lookPathInEnv()=%q want %q", got, codexPath)
	}
}

func TestCommandUsesResolvedNodeFromAugmentedEnv(t *testing.T) {
	tmp := t.TempDir()
	nodePath := filepath.Join(tmp, "node")
	if err := os.WriteFile(nodePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNI_CODEX_NODE_BIN", nodePath)
	cmd, err := Command(t.Context(), t.TempDir(), filepath.Join(t.TempDir(), "request.json"))
	if err != nil {
		t.Fatalf("Command() failed: %v", err)
	}
	if cmd.Path != nodePath {
		t.Fatalf("Command path=%q want %q", cmd.Path, nodePath)
	}
}

func TestResolveCodexPathUsesExplicitBinary(t *testing.T) {
	tmp := t.TempDir()
	codexPath := filepath.Join(tmp, "codex")
	if err := os.WriteFile(codexPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveCodexPath(codexPath)
	if err != nil {
		t.Fatalf("ResolveCodexPath() failed: %v", err)
	}
	if got != codexPath {
		t.Fatalf("ResolveCodexPath()=%q want %q", got, codexPath)
	}
}

func TestPreflightReportsMissingCodex(t *testing.T) {
	tmp := t.TempDir()
	nodePath := filepath.Join(tmp, "node")
	npmPath := filepath.Join(tmp, "npm")
	for _, path := range []string{nodePath, npmPath} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	missingCodex := filepath.Join(tmp, "missing-codex")
	issues := PreflightFor(nodePath, npmPath, missingCodex)
	if len(issues) != 1 {
		t.Fatalf("expected one preflight issue, got %#v", issues)
	}
	if !strings.Contains(issues[0].Tool, "missing-codex") {
		t.Fatalf("expected missing codex issue, got %#v", issues[0])
	}
}

func TestRunnerScriptRequiresResolvedCodexPath(t *testing.T) {
	if strings.Contains(RunnerScript, `request.codex_path || "codex"`) {
		t.Fatal("runner must not fall back to plain codex; server preflight must provide a resolved path")
	}
	if !strings.Contains(RunnerScript, "codex_path is required") {
		t.Fatal("runner must fail loudly when codex_path is missing")
	}
}
