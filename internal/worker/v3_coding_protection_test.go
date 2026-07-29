package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectCodingServerProtectsExplicitlyImmutablePaths(t *testing.T) {
	root := t.TempDir()
	requestPath := filepath.Join(root, "REQUEST.md")
	if err := os.WriteFile(requestPath, []byte("authoritative\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	protected, err := snapshotDirectCodingProtectedPathList(root, []string{"REQUEST.md"})
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectDirectCodingProtectedMutation("REQUEST.md", protected); err == nil {
		t.Fatal("planned mutation of protected file was accepted")
	}
	assembly := directCodingAssembly{Files: []directCodingFileTask{{Path: "REQUEST.md", Content: "replacement\n"}}}
	if err := validateDirectCodingAssemblyProtection(assembly, protected); err == nil {
		t.Fatal("assembly mutation of protected file was accepted")
	}
	if err := validateDirectCodingProtectedPaths(root, protected); err != nil {
		t.Fatalf("unchanged protected file rejected: %v", err)
	}
	if err := os.WriteFile(requestPath, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingProtectedPaths(root, protected); err == nil || !strings.Contains(err.Error(), "REQUEST.md") {
		t.Fatalf("protected mutation error=%v", err)
	}
}

func TestDirectCodingServerProtectsAnInitiallyMissingPath(t *testing.T) {
	root := t.TempDir()
	protected, err := snapshotDirectCodingProtectedPathList(root, []string{"generated.lock"})
	if err != nil {
		t.Fatal(err)
	}
	if len(protected) != 1 {
		t.Fatalf("protected=%v", protected)
	}
	if err := os.WriteFile(filepath.Join(root, "generated.lock"), []byte("created"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingProtectedPaths(root, protected); err == nil || !strings.Contains(err.Error(), "was created") {
		t.Fatalf("missing-path protection error=%v", err)
	}
}
