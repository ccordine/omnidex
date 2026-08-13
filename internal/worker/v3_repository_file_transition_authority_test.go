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
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestFileTransitionClassifierRequiresExactCreateAndDeleteInventory(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		fixture func(*testing.T) (repositoryfacts.Snapshot, repositoryfacts.Analysis)
		stage   func(*testing.T, repositoryfacts.Snapshot, repositoryfacts.Analysis) *changeapply.StagedChange
	}{
		{name: "create", fixture: existingRepositoryVerificationFixture, stage: desiredVerificationCreateStage},
		{name: "delete", fixture: desiredVerificationDeletionFixture, stage: desiredVerificationDeleteStage},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source, analysis := test.fixture(t)
			stage := test.stage(t, source, analysis)
			t.Cleanup(func() { _ = stage.Cleanup() })
			changed, err := repositoryMutationFilesForExpectedState(source, stage.ExpectedFiles())
			if err != nil {
				t.Fatal(err)
			}
			command := queue.RepositoryMutationCommand{
				SourceSnapshotID: source.ID, ChangedFiles: changed,
			}
			state, err := classifyRepositoryMutationSnapshots(source, source, command)
			if err != nil || state != queue.RepositoryMutationSource {
				t.Fatalf("source classification=%q error=%v", state, err)
			}
			if _, err := stage.ApplyVerified(context.Background()); err != nil {
				t.Fatal(err)
			}
			post := existingRepositoryRefreshedIndex(t, source.Root).Snapshot
			state, err = classifyRepositoryMutationSnapshots(source, post, command)
			if err != nil || state != queue.RepositoryMutationPost {
				t.Fatalf("post classification=%q error=%v", state, err)
			}
			if err := os.WriteFile(filepath.Join(source.Root, "unauthorized.txt"), []byte("drift\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			drifted := existingRepositoryRefreshedIndex(t, source.Root).Snapshot
			state, err = classifyRepositoryMutationSnapshots(source, drifted, command)
			if err != nil || state != queue.RepositoryMutationIndeterminate {
				t.Fatalf("drift classification=%q error=%v", state, err)
			}
		})
	}
}

func TestExactAuthoritativeRepositoryPostSupportsCreateAndDelete(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		fixture func(*testing.T) (repositoryfacts.Snapshot, repositoryfacts.Analysis)
		stage   func(*testing.T, repositoryfacts.Snapshot, repositoryfacts.Analysis) *changeapply.StagedChange
	}{
		{name: "create", fixture: existingRepositoryVerificationFixture, stage: desiredVerificationCreateStage},
		{name: "delete", fixture: desiredVerificationDeletionFixture, stage: desiredVerificationDeleteStage},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source, analysis := test.fixture(t)
			stage := test.stage(t, source, analysis)
			t.Cleanup(func() { _ = stage.Cleanup() })
			graph := desiredVerificationGraph(t, source, analysis, []repositoryfacts.DesiredGoArtifact{{
				RequirementQuote:  "The exact repository inventory must satisfy its acceptance state.",
				PackageArtifactID: desiredVerificationPackageID(t, analysis, "verification"),
				Signature:         "func AuthorityOnly() int",
				MustExist:         true,
			}})
			commands, err := desiredArtifactGraphGoVerificationCommands(source, analysis, graph)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := newVerifiedRepositoryChangeStage(graph.ID, commands, stage)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := stage.ApplyVerified(context.Background()); err != nil {
				t.Fatal(err)
			}
			post, err := exactAuthoritativeRepositoryPostSnapshot(
				context.Background(), source.Root, source, graph.ID, commands, prepared,
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateExactRepositoryPostInventory(source, post, stage.ExpectedFiles()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFileStatePatchResultRequiresCodeDerivedAction(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		fixture func(*testing.T) (repositoryfacts.Snapshot, repositoryfacts.Analysis)
		stage   func(*testing.T, repositoryfacts.Snapshot, repositoryfacts.Analysis) *changeapply.StagedChange
		action  string
	}{
		{name: "create", fixture: existingRepositoryVerificationFixture, stage: desiredVerificationCreateStage, action: "create"},
		{name: "delete", fixture: desiredVerificationDeletionFixture, stage: desiredVerificationDeleteStage, action: "delete"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, analysis := test.fixture(t)
			stage := test.stage(t, source, analysis)
			t.Cleanup(func() { _ = stage.Cleanup() })
			expected := stage.ExpectedFiles()
			result := []omni.PatchFileResult{{Path: expected[0].Path, Action: test.action}}
			if err := validateRepositoryFileStatePatchResult(source, expected, result); err != nil {
				t.Fatal(err)
			}
			result[0].Action = map[string]string{"create": "delete", "delete": "create"}[test.action]
			if err := validateRepositoryFileStatePatchResult(source, expected, result); err == nil ||
				!strings.Contains(err.Error(), "unexpected action") {
				t.Fatalf("wrong physical action error=%v", err)
			}
		})
	}
}
