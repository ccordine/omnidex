package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectCodingDetectsExistingImplementationWithoutTreatingProtectedRequestAsCode(t *testing.T) {
	root := t.TempDir()
	requestPath := filepath.Join(root, "REQUEST.md")
	if err := os.WriteFile(requestPath, []byte("authoritative request\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	protected := map[string]directCodingProtectedPath{
		"REQUEST.md": {Path: "REQUEST.md"},
	}
	hasImplementation, err := directCodingWorkspaceHasImplementation(root, protected)
	if err != nil {
		t.Fatal(err)
	}
	if hasImplementation {
		t.Fatal("protected request was treated as an existing implementation")
	}
	internalState := filepath.Join(root, ".omni", "runs", "481")
	if err := os.MkdirAll(internalState, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(internalState, "projection.ts"), []byte("derived inspection only\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hasImplementation, err = directCodingWorkspaceHasImplementation(root, protected)
	if err != nil || hasImplementation {
		t.Fatalf("internal task-state projection became implementation=%t error=%v", hasImplementation, err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hasImplementation, err = directCodingWorkspaceHasImplementation(root, protected)
	if err != nil || !hasImplementation {
		t.Fatalf("existing implementation=%t error=%v", hasImplementation, err)
	}
}
