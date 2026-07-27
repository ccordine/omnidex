package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopedResearchUsesOnlyRequestedWorkspace(t *testing.T) {
	configuredRoot := t.TempDir()
	requestedRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(configuredRoot, "music-routing.txt"), []byte("remembered music application"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(requestedRoot, "agent-routing.txt"), []byte("authoritative agent routing"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := New(true, configuredRoot, 100, 4000)
	scoped, err := base.Scoped(requestedRoot)
	if err != nil {
		t.Fatal(err)
	}
	result, err := scoped.Research("agent routing")
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, err := filepath.Abs(requestedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if result.Root != wantRoot {
		t.Fatalf("root=%q want %q", result.Root, wantRoot)
	}
	if strings.Contains(result.Summary, "music-routing") || !strings.Contains(result.Summary, "agent-routing") {
		t.Fatalf("scoped research crossed workspace boundary: %s", result.Summary)
	}
}

func TestScopedRejectsInvalidRootWithoutConfiguredFallback(t *testing.T) {
	base := New(true, t.TempDir(), 100, 4000)
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := base.Scoped(missing); err == nil || !strings.Contains(err.Error(), "inspect workspace root") {
		t.Fatalf("invalid requested root err=%v", err)
	}
}
