package indexing

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestRefreshStoresCompleteSnapshotAndCompilerAnalysis(t *testing.T) {
	root := newGoRepository(t, "package sample\n\nfunc Value() int { return 1 }\n")
	store := &recordingStore{}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Refresh(context.Background(), 42, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Snapshot.ID == "" || len(result.Analyses) != 1 {
		t.Fatalf("result=%#v", result)
	}
	if store.snapshot.ID != result.Snapshot.ID || len(store.analyses) != 1 || !store.analyses[0].Complete {
		t.Fatalf("store snapshot=%q analyses=%#v", store.snapshot.ID, store.analyses)
	}
}

func TestRefreshPersistsSupportedFactsAndFailsForUnsupportedSourceLanguages(t *testing.T) {
	root := newGoRepository(t, "package sample\n\nfunc Value() int { return 1 }\n")
	if err := os.WriteFile(filepath.Join(root, "legacy.py"), []byte("def legacy():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := &recordingStore{}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Refresh(context.Background(), 42, root)
	if err == nil || !strings.Contains(err.Error(), "unsupported source languages: python") {
		t.Fatalf("error=%v", err)
	}
	if result.Complete || len(result.Analyses) != 1 || len(store.analyses) != 1 {
		t.Fatalf("partial result=%#v stored=%d", result, len(store.analyses))
	}
}

func TestRefreshPersistsCompilerDiagnosticsAndFailsIncompleteAnalysis(t *testing.T) {
	root := newGoRepository(t, "package sample\n\nfunc Broken(value MissingType) {}\n")
	store := &recordingStore{}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Refresh(context.Background(), 42, root)
	if err == nil || !strings.Contains(err.Error(), "go analysis is incomplete") {
		t.Fatalf("error=%v", err)
	}
	if result.Complete || len(store.analyses) != 1 || store.analyses[0].Complete || len(store.analyses[0].Diagnostics) == 0 {
		t.Fatalf("partial result=%#v stored=%#v", result, store.analyses)
	}
}

type recordingStore struct {
	snapshot repositoryfacts.Snapshot
	analyses []repositoryfacts.Analysis
}

func (store *recordingStore) StoreRepositorySnapshot(_ context.Context, projectID int64, snapshot repositoryfacts.Snapshot) error {
	if projectID < 1 {
		return nil
	}
	store.snapshot = snapshot
	return nil
}

func (store *recordingStore) StoreRepositoryAnalysis(_ context.Context, projectID int64, snapshot repositoryfacts.Snapshot, analysis repositoryfacts.Analysis) error {
	if projectID < 1 || snapshot.ID != store.snapshot.ID {
		return nil
	}
	store.analyses = append(store.analyses, analysis)
	return nil
}

func newGoRepository(t *testing.T, source string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	root := t.TempDir()
	writeIndexingFile(t, root, "go.mod", "module example.test/sample\n\ngo 1.24.1\n")
	writeIndexingFile(t, root, "sample.go", source)
	runIndexingGit(t, root, "init")
	runIndexingGit(t, root, "config", "user.email", "indexing@example.test")
	runIndexingGit(t, root, "config", "user.name", "Indexing Test")
	runIndexingGit(t, root, "add", ".")
	runIndexingGit(t, root, "commit", "-m", "fixture")
	return root
}

func writeIndexingFile(t *testing.T, root, relative, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, relative), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runIndexingGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
}
