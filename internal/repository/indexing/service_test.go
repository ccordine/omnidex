package indexing

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestCaptureStoresCompleteSnapshotWithoutRunningCompilerAnalysis(t *testing.T) {
	root := newGoRepository(t, "package sample\n\nfunc Value() int { return 1 }\n")
	store := &recordingStore{}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Capture(context.Background(), 42, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Snapshot.ID == "" || len(result.Analyses) != 0 {
		t.Fatalf("result=%#v", result)
	}
	if store.snapshot.ID != result.Snapshot.ID || len(store.analyses) != 0 {
		t.Fatalf("store snapshot=%q analyses=%#v", store.snapshot.ID, store.analyses)
	}
}

func TestCaptureMixedLanguageRepositoryDoesNotRequireUnrelatedAdapters(t *testing.T) {
	root := newGoRepository(t, "package sample\n\nfunc Value() int { return 1 }\n")
	for relative, source := range map[string]string{
		"legacy.java": "final class Legacy {}\n",
		"legacy.js":   "export const legacy = 1;\n",
		"legacy.php":  "<?php function legacy(): int { return 1; }\n",
		"legacy.rs":   "pub fn legacy() -> i32 { 1 }\n",
		"legacy.ts":   "export const legacy: number = 1;\n",
	} {
		if err := os.WriteFile(filepath.Join(root, relative), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store := &recordingStore{}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Capture(context.Background(), 42, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(result.Snapshot.Files) != 7 || len(result.Analyses) != 0 || len(store.analyses) != 0 {
		t.Fatalf("capture result=%#v stored=%d", result, len(store.analyses))
	}
}

func TestCaptureDefersUnrelatedGoAnalyzerFailureUntilGoIsDemanded(t *testing.T) {
	root := newGoRepository(t, "package sample\n\nfunc Value() int { return 1 }\n")
	if err := os.MkdirAll(filepath.Join(root, "internal/api/web/dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeIndexingFile(t, root, "internal/api/ui.go", `package api

import _ "embed"

//go:embed web/dist/*
var ui string
`)
	writeIndexingFile(t, root, "internal/api/web/.gitignore", "dist/\n")
	writeIndexingFile(t, root, "internal/api/web/dist/index.html", "<!doctype html>\n")
	runIndexingGit(t, root, "add", "internal/api/ui.go", "internal/api/web/.gitignore")
	runIndexingGit(t, root, "commit", "-m", "embedded fixture")

	store := &recordingStore{}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Capture(context.Background(), 42, root)
	if err != nil {
		t.Fatalf("capture failed before an adapter was demanded: %v", err)
	}
	if len(result.Analyses) != 0 || len(store.analyses) != 0 {
		t.Fatalf("capture invoked analyzer: result=%#v stored=%#v", result, store.analyses)
	}
	if _, err := service.Analyze(context.Background(), 42, result.Snapshot, "go"); err == nil ||
		!strings.Contains(err.Error(), "embedded build input outside the exact repository snapshot") {
		t.Fatalf("demanded Go analyzer error=%v", err)
	}
}

func TestAnalyzeRunsOnlyExplicitlyDemandedAdapter(t *testing.T) {
	root := newGoRepository(t, "package sample\n\nfunc Value() int { return 1 }\n")
	if err := os.WriteFile(filepath.Join(root, "legacy.py"), []byte("def legacy():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := &recordingStore{}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Capture(context.Background(), 42, root)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := service.Analyze(context.Background(), 42, result.Snapshot, "go")
	if err != nil {
		t.Fatal(err)
	}
	if !analysis.Complete || analysis.Adapter.Name != "go" || len(store.analyses) != 1 {
		t.Fatalf("analysis=%#v stored=%#v", analysis, store.analyses)
	}
}

func TestAnalyzeDoesNotInvokeAnUnrelatedRegisteredAdapter(t *testing.T) {
	root := newGoRepository(t, "package sample\n\nfunc Value() int { return 1 }\n")
	store := &recordingStore{}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Capture(context.Background(), 42, root)
	if err != nil {
		t.Fatal(err)
	}
	goCalls, typeScriptCalls := 0, 0
	typeScriptIdentity := repositoryfacts.AdapterIdentity{Name: "typescript", Version: "test-v1"}
	service.analyzers = map[string]analyzer{
		"go": {
			identity: repositoryfacts.AdapterIdentity{Name: "go", Version: "test-v1"},
			analyze: func(context.Context, repositoryfacts.Snapshot) (repositoryfacts.Analysis, error) {
				goCalls++
				return repositoryfacts.Analysis{}, nil
			},
		},
		"typescript": {
			identity: typeScriptIdentity,
			analyze: func(_ context.Context, snapshot repositoryfacts.Snapshot) (repositoryfacts.Analysis, error) {
				typeScriptCalls++
				analysis := repositoryfacts.Analysis{
					Schema: repositoryfacts.AnalysisSchemaV1, SnapshotID: snapshot.ID,
					Adapter: typeScriptIdentity, Complete: true, GeneratedAt: time.Now().UTC(),
					Symbols: []repositoryfacts.Symbol{}, Artifacts: []repositoryfacts.Artifact{},
					Edges: []repositoryfacts.Edge{}, Diagnostics: []repositoryfacts.AnalysisDiagnostic{},
				}
				if err := repositoryfacts.FinalizeAnalysis(&analysis); err != nil {
					return repositoryfacts.Analysis{}, err
				}
				return analysis, nil
			},
		},
	}
	analysis, err := service.Analyze(
		context.Background(), 42, result.Snapshot, "typescript",
	)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Adapter != typeScriptIdentity || goCalls != 0 || typeScriptCalls != 1 {
		t.Fatalf(
			"analysis=%#v go_calls=%d typescript_calls=%d",
			analysis, goCalls, typeScriptCalls,
		)
	}
}

func TestAnalyzePersistsCompilerDiagnosticsAndFailsIncompleteDemand(t *testing.T) {
	root := newGoRepository(t, "package sample\n\nfunc Broken(value MissingType) {}\n")
	store := &recordingStore{}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Capture(context.Background(), 42, root)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := service.Analyze(context.Background(), 42, result.Snapshot, "go")
	if err == nil || !strings.Contains(err.Error(), "go analysis is incomplete") {
		t.Fatalf("error=%v", err)
	}
	if analysis.Complete || len(store.analyses) != 1 || store.analyses[0].Complete || len(store.analyses[0].Diagnostics) == 0 {
		t.Fatalf("analysis=%#v stored=%#v", analysis, store.analyses)
	}
}

func TestAnalyzeRejectsUndemandedOrUnregisteredAdapter(t *testing.T) {
	root := newGoRepository(t, "package sample\n\nfunc Value() int { return 1 }\n")
	store := &recordingStore{}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Capture(context.Background(), 42, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, adapterID := range []string{"", " go", "typescript"} {
		if _, err := service.Analyze(context.Background(), 42, result.Snapshot, adapterID); err == nil {
			t.Fatalf("adapter %q was accepted", adapterID)
		}
	}
	if len(store.analyses) != 0 {
		t.Fatalf("unregistered demand stored analyses=%#v", store.analyses)
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
