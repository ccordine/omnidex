package queue

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
)

func TestRepositorySnapshotStoreRejectsMissingAuthority(t *testing.T) {
	repository := &Repository{}
	if err := repository.StoreRepositorySnapshot(context.Background(), 0, repositoryfacts.Snapshot{}); err == nil || !strings.Contains(err.Error(), "project ID") {
		t.Fatalf("invalid project authority error=%v", err)
	}
	if _, err := repository.RepositorySnapshot(context.Background(), 1, ""); err == nil || !strings.Contains(err.Error(), "ID") {
		t.Fatalf("missing snapshot identity error=%v", err)
	}
	if err := repository.StoreRepositoryAnalysis(context.Background(), 0, repositoryfacts.Snapshot{}, repositoryfacts.Analysis{}); err == nil || !strings.Contains(err.Error(), "project ID") {
		t.Fatalf("invalid analysis authority error=%v", err)
	}
	if _, err := repository.RepositoryAnalysis(context.Background(), 1, ""); err == nil || !strings.Contains(err.Error(), "ID") {
		t.Fatalf("missing analysis identity error=%v", err)
	}
	if _, err := repository.SearchRepositorySymbols(context.Background(), 0, "analysis_x", "main", 5); err == nil || !strings.Contains(err.Error(), "project ID") {
		t.Fatalf("invalid symbol search authority error=%v", err)
	}
	if _, err := repository.RepositoryGraphNeighborhood(context.Background(), 1, "analysis_x", nil, 5); err == nil || !strings.Contains(err.Error(), "subject") {
		t.Fatalf("missing graph subject error=%v", err)
	}
}

func TestPostgresRepositorySnapshotsAreExactAndImmutable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for repository intelligence tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	runQueueRepositoryGit(t, root, "init")
	runQueueRepositoryGit(t, root, "config", "user.email", "repository@example.test")
	runQueueRepositoryGit(t, root, "config", "user.name", "Repository Test")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/repository\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"Zebra.txt", "alpha.txt", // regular files
		"Zebra.key", "alpha.key", // sensitive exclusions
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runQueueRepositoryGit(t, root, "add", "main.go", "go.mod", "Zebra.txt", "alpha.txt", "Zebra.key", "alpha.key")
	runQueueRepositoryGit(t, root, "commit", "-m", "initial")
	snapshot, err := repositoryfacts.BuildGitSnapshot(ctx, root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := golangadapter.Analyze(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !analysis.Complete || len(analysis.Symbols) == 0 {
		t.Fatalf("analysis did not produce complete compiler facts: %#v", analysis)
	}
	project, err := repository.CreateProject(ctx, "repository-intelligence-test", root, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, project.ID)
	})
	if err := repository.StoreRepositorySnapshot(ctx, project.ID, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := repository.StoreRepositorySnapshot(ctx, project.ID, snapshot); err != nil {
		t.Fatalf("idempotent snapshot store: %v", err)
	}
	if err := repository.StoreRepositoryAnalysis(ctx, project.ID, snapshot, analysis); err != nil {
		t.Fatal(err)
	}
	if err := repository.StoreRepositoryAnalysis(ctx, project.ID, snapshot, analysis); err != nil {
		t.Fatalf("idempotent analysis store: %v", err)
	}
	stored, err := repository.LatestRepositorySnapshot(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != snapshot.ID || len(stored.Files) != len(snapshot.Files) {
		t.Fatalf("stored snapshot=%#v want ID=%s files=%d", stored, snapshot.ID, len(snapshot.Files))
	}
	if got, want := stored.GeneratedAt.Format(time.RFC3339Nano), snapshot.GeneratedAt.Format(time.RFC3339Nano); got != want {
		t.Fatalf("stored snapshot generated time=%s want exact %s", got, want)
	}
	storedAnalysis, err := repository.RepositoryAnalysis(ctx, project.ID, analysis.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedAnalysis.ID != analysis.ID || len(storedAnalysis.Symbols) != len(analysis.Symbols) {
		t.Fatalf("stored analysis=%#v want ID=%s symbols=%d", storedAnalysis, analysis.ID, len(analysis.Symbols))
	}
	if got, want := storedAnalysis.GeneratedAt.Format(time.RFC3339Nano), analysis.GeneratedAt.Format(time.RFC3339Nano); got != want {
		t.Fatalf("stored analysis generated time=%s want exact %s", got, want)
	}
	matches, err := repository.SearchRepositorySymbols(ctx, project.ID, analysis.ID, "main", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 || matches[0].Symbol.Name != "main" || matches[0].Symbol.FileID == "" {
		t.Fatalf("symbol matches=%#v", matches)
	}
	neighborhood, err := repository.RepositoryGraphNeighborhood(ctx, project.ID, analysis.ID, []string{matches[0].Symbol.ID}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if neighborhood.AnalysisID != analysis.ID || len(neighborhood.Edges) == 0 {
		t.Fatalf("graph neighborhood=%#v", neighborhood)
	}
	if _, err := pool.Exec(ctx, `UPDATE repository_snapshots SET root='/tampered' WHERE id=$1`, snapshot.ID); err == nil {
		t.Fatal("database allowed an immutable repository snapshot to be edited")
	}
	if _, err := pool.Exec(ctx, `UPDATE repository_analyses SET complete=FALSE WHERE id=$1`, analysis.ID); err == nil {
		t.Fatal("database allowed an immutable repository analysis to be edited")
	}
}

func runQueueRepositoryGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
}
