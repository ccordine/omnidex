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
	base, err := New(true, configuredRoot, 100, 4000)
	if err != nil {
		t.Fatal(err)
	}
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

func TestSnapshotExposesRelativeTreeWithoutHostRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scanner, err := New(true, root, 100, 4000)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := scanner.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot, root) {
		t.Fatalf("snapshot leaked authoritative host root: %s", snapshot)
	}
	if !strings.Contains(snapshot, "- main.go") {
		t.Fatalf("snapshot omitted relative file tree: %s", snapshot)
	}
}

func TestScopedRejectsInvalidRootWithoutConfiguredFallback(t *testing.T) {
	base, err := New(true, t.TempDir(), 100, 4000)
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := base.Scoped(missing); err == nil || !strings.Contains(err.Error(), "inspect workspace root") {
		t.Fatalf("invalid requested root err=%v", err)
	}
}

func TestNewRejectsInvalidLimits(t *testing.T) {
	if _, err := New(true, t.TempDir(), 0, 4000); err == nil || !strings.Contains(err.Error(), "max files") {
		t.Fatalf("max files error=%v", err)
	}
	if _, err := New(true, t.TempDir(), 100, 0); err == nil || !strings.Contains(err.Error(), "context budget") {
		t.Fatalf("context budget error=%v", err)
	}
}

func TestDisabledScannerFailsInsteadOfReturningSyntheticContext(t *testing.T) {
	scanner, err := New(false, "", 100, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.Snapshot(); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("Snapshot() error=%v", err)
	}
	if _, err := scanner.Research("anything"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("Research() error=%v", err)
	}
}

func TestResearchDoesNotExpandNoMatchIntoBroadSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "music-app.txt"), []byte("remembered music application"), 0o600); err != nil {
		t.Fatal(err)
	}
	scanner, err := New(true, root, 100, 4000)
	if err != nil {
		t.Fatal(err)
	}
	result, err := scanner.Research("unrelated zebra objective")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Summary, "No workspace files matched") {
		t.Fatalf("expected explicit no-match result, got %q", result.Summary)
	}
	if strings.Contains(result.Context, "remembered music application") {
		t.Fatalf("no-match research leaked unrelated workspace content: %q", result.Context)
	}
}

func TestLoadExcerptReturnsReadFailure(t *testing.T) {
	root := t.TempDir()
	scanner, err := New(true, root, 100, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := scanner.loadExcerpt("missing.go", []string{"missing"}, 1, true); err == nil || !strings.Contains(err.Error(), "read workspace excerpt") {
		t.Fatalf("loadExcerpt() error=%v", err)
	}
}

func TestTokenizeKeepsRelevantShortTechnologyNames(t *testing.T) {
	if got := strings.Join(tokenize("Build Go API and TS UI"), ","); got != "build,go,api,and,ts,ui" {
		t.Fatalf("tokens=%q", got)
	}
}
