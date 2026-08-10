package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
	"github.com/gryph/omnidex/internal/repository/changeapply"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

func TestExactRepositoryCandidateDeclarationsRejectMissingAndExtraCandidates(t *testing.T) {
	t.Parallel()
	contract := repositoryfacts.ChangeContract{Targets: []repositoryfacts.ChangeTarget{
		{SymbolID: "symbol-one"}, {SymbolID: "symbol-two"},
	}}
	if _, err := exactRepositoryCandidateDeclarations(contract, map[string]string{
		"symbol-one": "func One() {}",
	}); err == nil || !strings.Contains(err.Error(), "1 declarations for 2") {
		t.Fatalf("missing declaration error=%v", err)
	}
	if _, err := exactRepositoryCandidateDeclarations(contract, map[string]string{
		"symbol-one": "func One() {}", "symbol-extra": "func Extra() {}",
	}); err == nil || !strings.Contains(err.Error(), "no candidate") {
		t.Fatalf("extra declaration error=%v", err)
	}
}

func TestRepositoryPatchResultRequiresExactChangedFiles(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	if err := validateRepositoryPatchResult(snapshot, []string{first.FileID}, []omni.PatchFileResult{
		{Path: "first.go", Action: "update"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := validateRepositoryPatchResult(snapshot, []string{first.FileID}, []omni.PatchFileResult{
		{Path: "first.go", Action: "created"},
	}); err == nil {
		t.Fatal("created action was accepted for an exact declaration replacement")
	}
}

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

func TestRepositoryMutationClassifierRequiresExactCompleteInventory(t *testing.T) {
	t.Parallel()
	source, analysis := existingRepositoryVerificationFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	sourceFile := exactRepositorySnapshotFile(t, source, first.FileID)
	postContent := []byte("package verification\n\nfunc First() int { return 2 }\n")
	if err := os.WriteFile(filepath.Join(source.Root, sourceFile.Path), postContent, 0o600); err != nil {
		t.Fatal(err)
	}
	post, err := repositoryfacts.BuildGitSnapshot(
		context.Background(), source.Root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	postFile := exactRepositorySnapshotFile(t, post, first.FileID)
	command := queue.RepositoryMutationCommand{
		SourceSnapshotID: source.ID,
		ChangedFiles: []queue.RepositoryMutationFile{{
			FileID: first.FileID, Path: sourceFile.Path,
			SourceSHA256: sourceFile.SHA256, SourceSize: sourceFile.Size,
			ExpectedSHA256: postFile.SHA256, ExpectedSize: postFile.Size,
		}},
	}
	state, err := classifyRepositoryMutationSnapshots(source, source, command)
	if err != nil || state != queue.RepositoryMutationSource {
		t.Fatalf("source state=%q error=%v", state, err)
	}
	state, err = classifyRepositoryMutationSnapshots(source, post, command)
	if err != nil || state != queue.RepositoryMutationPost {
		t.Fatalf("post state=%q error=%v", state, err)
	}
	if err := os.WriteFile(
		filepath.Join(source.Root, "unexpected.txt"), []byte("unexpected\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	drifted, err := repositoryfacts.BuildGitSnapshot(
		context.Background(), source.Root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	state, err = classifyRepositoryMutationSnapshots(source, drifted, command)
	if err != nil || state != queue.RepositoryMutationIndeterminate {
		t.Fatalf("drifted state=%q error=%v", state, err)
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
				FileID: file.ID,
				SHA256: file.SHA256,
				Size:   file.Size,
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
