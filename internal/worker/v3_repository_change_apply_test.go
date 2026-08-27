package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
	"github.com/gryph/omnidex/internal/repository/changeapply"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

func TestRefreshedRepositoryChangeRejectsOutOfContractInventory(t *testing.T) {
	t.Parallel()
	before, analysis := existingRepositoryVerificationFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	if err := os.WriteFile(
		filepath.Join(before.Root, "first.go"),
		[]byte("package verification\n\nfunc First() int { return 2 }\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	changed := existingRepositoryRefreshedIndex(t, before.Root)
	expected := expectedRepositoryFileState(t, changed, first.FileID)
	if err := validateRefreshedRepositoryChange(before, changed, []changeapply.ExpectedFileState{expected}); err != nil {
		t.Fatal(err)
	}
	wrongRoot := changed
	wrongRoot.Snapshot.Root = t.TempDir()
	if err := validateRefreshedRepositoryChange(
		before, wrongRoot, []changeapply.ExpectedFileState{expected},
	); err == nil || !strings.Contains(err.Error(), "changed worktree") {
		t.Fatalf("wrong sibling worktree error=%v", err)
	}
	if err := os.WriteFile(
		filepath.Join(before.Root, "first.go"),
		[]byte("package verification\n\nfunc First() int { return 3 }\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	tampered := existingRepositoryRefreshedIndex(t, before.Root)
	if err := validateRefreshedRepositoryChange(
		before, tampered, []changeapply.ExpectedFileState{expected},
	); err == nil || !strings.Contains(err.Error(), "exact contract") {
		t.Fatalf("unexpected post-patch target bytes error=%v", err)
	}
	if err := os.WriteFile(filepath.Join(before.Root, "unexpected.txt"), []byte("unexpected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unexpected := existingRepositoryRefreshedIndex(t, before.Root)
	if err := validateRefreshedRepositoryChange(
		before, unexpected, []changeapply.ExpectedFileState{expected},
	); err == nil ||
		!strings.Contains(err.Error(), "inventory") {
		t.Fatalf("out-of-contract inventory error=%v", err)
	}
}

func exactRepositorySnapshotFile(
	t *testing.T,
	snapshot repositoryfacts.Snapshot,
	fileID string,
) repositoryfacts.File {
	t.Helper()
	for _, file := range snapshot.Files {
		if file.ID == fileID {
			return file
		}
	}
	t.Fatalf("repository snapshot omitted file %q", fileID)
	return repositoryfacts.File{}
}

func expectedRepositoryFileState(
	t *testing.T,
	result repositoryindex.Result,
	fileID string,
) changeapply.ExpectedFileState {
	t.Helper()
	for _, file := range result.Snapshot.Files {
		if file.ID == fileID {
			return changeapply.ExpectedFileState{
				FileID: file.ID, Path: file.Path, Present: true,
				SHA256: file.SHA256, Size: file.Size, Mode: file.Mode,
			}
		}
	}
	t.Fatalf("refreshed index omitted file %q", fileID)
	return changeapply.ExpectedFileState{}
}

func existingRepositoryRefreshedIndex(t *testing.T, root string) repositoryindex.Result {
	t.Helper()
	snapshot, err := repositoryfacts.BuildGitSnapshot(context.Background(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := golangadapter.Analyze(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return repositoryindex.Result{
		Snapshot: snapshot, Analyses: []repositoryfacts.Analysis{analysis}, Complete: true,
	}
}
