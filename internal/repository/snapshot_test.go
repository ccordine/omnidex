package repository

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildGitSnapshotCapturesCompleteHashBoundWorktree(t *testing.T) {
	root := newGitRepository(t)
	writeRepositoryTestFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	writeRepositoryTestFile(t, root, "README.md", "# Example\n")
	writeRepositoryTestFile(t, root, ".env", "SECRET=must-not-be-indexed\n")
	runRepositoryGit(t, root, "add", "main.go", "README.md", ".env")
	runRepositoryGit(t, root, "commit", "-m", "initial")
	writeRepositoryTestFile(t, root, "internal/new.go", "package internal\n")

	snapshot, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{})
	if err != nil {
		t.Fatalf("BuildGitSnapshot: %v", err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("snapshot.Validate: %v", err)
	}
	if snapshot.Schema != SnapshotSchemaV1 || snapshot.ID == "" || snapshot.RepositoryID == "" {
		t.Fatalf("snapshot identity is incomplete: %#v", snapshot)
	}
	if snapshot.GeneratedAt.Location() != time.UTC || snapshot.GeneratedAt.Nanosecond()%int(time.Microsecond) != 0 {
		t.Fatalf("snapshot generated time is not canonical: %s", snapshot.GeneratedAt.Format(time.RFC3339Nano))
	}
	if snapshot.HeadCommit == "" || !snapshot.Dirty {
		t.Fatalf("snapshot Git state is incomplete: %#v", snapshot)
	}
	if got := repositoryTestFilePaths(snapshot.Files); strings.Join(got, ",") != "README.md,internal/new.go,main.go" {
		t.Fatalf("indexed files=%v", got)
	}
	if len(snapshot.Exclusions) != 1 || snapshot.Exclusions[0].Path != ".env" || snapshot.Exclusions[0].Reason != ExclusionSensitive {
		t.Fatalf("exclusions=%#v", snapshot.Exclusions)
	}
	for _, file := range snapshot.Files {
		if file.ID == "" || file.SHA256 == "" || file.Size < 0 {
			t.Fatalf("file identity is incomplete: %#v", file)
		}
	}

	firstID := snapshot.ID
	writeRepositoryTestFile(t, root, "README.md", "# Changed\n")
	changed, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{})
	if err != nil {
		t.Fatalf("BuildGitSnapshot after mutation: %v", err)
	}
	if changed.ID == firstID {
		t.Fatal("snapshot ID did not change after authoritative source changed")
	}
}

func TestBuildGitSnapshotFailsInsteadOfReturningTruncatedKnowledge(t *testing.T) {
	root := newGitRepository(t)
	writeRepositoryTestFile(t, root, "a.go", "package sample\n")
	writeRepositoryTestFile(t, root, "b.go", "package sample\n")
	runRepositoryGit(t, root, "add", "a.go", "b.go")
	runRepositoryGit(t, root, "commit", "-m", "initial")

	_, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{MaxFiles: 1})
	if err == nil || !strings.Contains(err.Error(), "repository index incomplete") {
		t.Fatalf("expected explicit incomplete-index failure, got %v", err)
	}
}

func TestBuildGitSnapshotExcludesOmnidexRunProjectionsFromRepositoryFacts(t *testing.T) {
	root := newGitRepository(t)
	writeRepositoryTestFile(t, root, "main.go", "package main\n")
	writeRepositoryTestFile(t, root, ".omni/runs/481/status.txt", "TASK ACTIVE task-7\n")
	runRepositoryGit(t, root, "add", "main.go")
	runRepositoryGit(t, root, "commit", "-m", "initial")

	snapshot, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{})
	if err != nil {
		t.Fatalf("BuildGitSnapshot: %v", err)
	}
	if got := repositoryTestFilePaths(snapshot.Files); strings.Join(got, ",") != "main.go" {
		t.Fatalf("internal run projection entered repository facts: %v", got)
	}
	if snapshot.Dirty || len(snapshot.Exclusions) != 0 {
		t.Fatalf("internal run projection affected repository state: dirty=%t exclusions=%#v", snapshot.Dirty, snapshot.Exclusions)
	}
	firstID := snapshot.ID
	writeRepositoryTestFile(t, root, ".omni/runs/481/status.txt", "TASK DONE task-7\n")
	writeRepositoryTestFile(t, root, ".omni/runs/481/events.jsonl", "{}\n")
	again, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{MaxFiles: 1})
	if err != nil {
		t.Fatalf("BuildGitSnapshot after projection update: %v", err)
	}
	if again.ID != firstID || again.GitStateSHA256 != snapshot.GitStateSHA256 || again.Dirty {
		t.Fatalf("internal projection changed snapshot identity: before=%s after=%s dirty=%t", firstID, again.ID, again.Dirty)
	}
}

func TestBuildGitSnapshotRecordsDeletedTrackedFilesWithoutGuessing(t *testing.T) {
	root := newGitRepository(t)
	writeRepositoryTestFile(t, root, "removed.go", "package sample\n")
	runRepositoryGit(t, root, "add", "removed.go")
	runRepositoryGit(t, root, "commit", "-m", "initial")
	if err := os.Remove(filepath.Join(root, "removed.go")); err != nil {
		t.Fatal(err)
	}

	snapshot, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{})
	if err != nil {
		t.Fatalf("BuildGitSnapshot: %v", err)
	}
	if len(snapshot.Files) != 0 {
		t.Fatalf("deleted file entered live inventory: %#v", snapshot.Files)
	}
	if len(snapshot.Exclusions) != 1 || snapshot.Exclusions[0].Path != "removed.go" || snapshot.Exclusions[0].Reason != ExclusionAbsent {
		t.Fatalf("deleted file state was not recorded: %#v", snapshot.Exclusions)
	}
}

func TestSnapshotValidationRejectsReorderedOrTamperedFacts(t *testing.T) {
	root := newGitRepository(t)
	writeRepositoryTestFile(t, root, "a.go", "package sample\n")
	writeRepositoryTestFile(t, root, "b.go", "package sample\n")
	runRepositoryGit(t, root, "add", "a.go", "b.go")
	runRepositoryGit(t, root, "commit", "-m", "initial")
	snapshot, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}

	snapshot.Files[0], snapshot.Files[1] = snapshot.Files[1], snapshot.Files[0]
	if err := snapshot.Validate(); err == nil || !strings.Contains(err.Error(), "sorted") {
		t.Fatalf("reordered snapshot was accepted: %v", err)
	}
}

func TestSnapshotValidationRejectsNonCanonicalGeneratedTime(t *testing.T) {
	root := newGitRepository(t)
	writeRepositoryTestFile(t, root, "main.go", "package main\n")
	runRepositoryGit(t, root, "add", "main.go")
	runRepositoryGit(t, root, "commit", "-m", "initial")
	snapshot, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}

	snapshot.GeneratedAt = snapshot.GeneratedAt.Add(time.Nanosecond)
	if err := snapshot.Validate(); err == nil || !strings.Contains(err.Error(), "microsecond UTC") {
		t.Fatalf("non-canonical generated time was accepted: %v", err)
	}
}

func TestSnapshotValidationRejectsNonCanonicalEmptyCollections(t *testing.T) {
	root := newGitRepository(t)
	writeRepositoryTestFile(t, root, "main.go", "package main\n")
	runRepositoryGit(t, root, "add", "main.go")
	runRepositoryGit(t, root, "commit", "-m", "initial")
	snapshot, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}

	snapshot.Exclusions = nil
	if err := snapshot.Validate(); err == nil || !strings.Contains(err.Error(), "canonical non-nil collections") {
		t.Fatalf("non-canonical empty exclusions were accepted: %v", err)
	}
}

func newGitRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for repository snapshot tests")
	}
	root := t.TempDir()
	runRepositoryGit(t, root, "init")
	runRepositoryGit(t, root, "config", "user.email", "snapshot@example.test")
	runRepositoryGit(t, root, "config", "user.name", "Snapshot Test")
	return root
}

func runRepositoryGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}

func writeRepositoryTestFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func repositoryTestFilePaths(files []File) []string {
	paths := make([]string, len(files))
	for index, file := range files {
		paths[index] = file.Path
	}
	return paths
}
