package cursorrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAugmentPathIncludesStandardDirs(t *testing.T) {
	path := augmentPath("")
	for _, want := range []string{"/usr/bin", "/bin"} {
		if !strings.Contains(path, want) {
			t.Fatalf("augmented PATH missing %q: %q", want, path)
		}
	}
}

func TestLookPathInEnvFindsBase64(t *testing.T) {
	env := CommandEnv()
	if _, err := lookPathInEnv("base64", env); err != nil {
		t.Fatalf("expected base64 on augmented PATH: %v", err)
	}
}

func TestLookPathInEnvAcceptsAbsoluteExecutable(t *testing.T) {
	tmp := t.TempDir()
	nodePath := filepath.Join(tmp, "node")
	if err := os.WriteFile(nodePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := lookPathInEnv(nodePath, []string{"PATH=/does/not/exist"})
	if err != nil {
		t.Fatalf("expected absolute executable to resolve: %v", err)
	}
	if got != nodePath {
		t.Fatalf("lookPathInEnv()=%q want %q", got, nodePath)
	}
}

func TestLookPathInEnvRejectsNonExecutable(t *testing.T) {
	tmp := t.TempDir()
	nodePath := filepath.Join(tmp, "node")
	if err := os.WriteFile(nodePath, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := lookPathInEnv(nodePath, []string{"PATH=/does/not/exist"}); err == nil {
		t.Fatal("expected non-executable path to be rejected")
	}
}

func TestCommandEnvUsesExplicitNodeBinDir(t *testing.T) {
	tmp := t.TempDir()
	nodePath := filepath.Join(tmp, "node")
	if err := os.WriteFile(nodePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNI_CURSOR_NODE_BIN", nodePath)
	env := CommandEnv()
	if _, err := lookPathInEnv("node", env); err != nil {
		t.Fatalf("expected node on PATH via OMNI_CURSOR_NODE_BIN: %v", err)
	}
}

func TestCommandUsesResolvedNodeFromAugmentedEnv(t *testing.T) {
	tmp := t.TempDir()
	nodePath := filepath.Join(tmp, "node")
	if err := os.WriteFile(nodePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNI_CURSOR_NODE_BIN", nodePath)
	cmd, err := Command(t.Context(), t.TempDir(), filepath.Join(t.TempDir(), "request.json"))
	if err != nil {
		t.Fatalf("Command() failed: %v", err)
	}
	if cmd.Path != nodePath {
		t.Fatalf("Command path=%q want %q", cmd.Path, nodePath)
	}
}

func TestDefaultModelUsesComposer25(t *testing.T) {
	t.Setenv("OMNI_CURSOR_MODEL", "")
	if got := DefaultModel(); got != "composer-2.5" {
		t.Fatalf("DefaultModel() = %q, want composer-2.5", got)
	}
}
