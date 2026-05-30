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

func TestDefaultModelUsesComposer25(t *testing.T) {
	t.Setenv("OMNI_CURSOR_MODEL", "")
	if got := DefaultModel(); got != "composer-2.5" {
		t.Fatalf("DefaultModel() = %q, want composer-2.5", got)
	}
}
