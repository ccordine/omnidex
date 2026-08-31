package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveV3WorkspaceRootMapsDistinctConfiguredHostRoot(t *testing.T) {
	temporary := t.TempDir()
	runtimeRoot := filepath.Join(temporary, "runtime")
	hostRoot := filepath.Join(temporary, "host")
	runtimeProject := filepath.Join(runtimeRoot, "projects", "example")
	hostProject := filepath.Join(hostRoot, "projects", "example")
	for _, directory := range []string{runtimeProject, hostProject} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create workspace fixture: %v", err)
		}
	}

	resolved, err := resolveV3WorkspaceRoot(runtimeRoot, hostRoot, hostProject)
	if err != nil {
		t.Fatalf("map configured host workspace: %v", err)
	}
	expected, err := filepath.EvalSymlinks(runtimeProject)
	if err != nil {
		t.Fatalf("resolve expected runtime workspace: %v", err)
	}
	if resolved != expected {
		t.Fatalf("resolved workspace=%q, want %q", resolved, expected)
	}
}

func TestResolveV3WorkspaceRootRejectsPathOutsideConfiguredBoundaries(t *testing.T) {
	temporary := t.TempDir()
	runtimeRoot := filepath.Join(temporary, "runtime")
	hostRoot := filepath.Join(temporary, "host")
	outside := filepath.Join(temporary, "outside")
	for _, directory := range []string{runtimeRoot, hostRoot, outside} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create workspace fixture: %v", err)
		}
	}

	_, err := resolveV3WorkspaceRoot(runtimeRoot, hostRoot, outside)
	if err == nil || !strings.Contains(err.Error(), "outside the configured workspace boundary") {
		t.Fatalf("outside workspace was not rejected explicitly: %v", err)
	}
}
