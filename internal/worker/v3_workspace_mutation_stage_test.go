package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestRepositorySemanticStageConvergesIntoGenericWorkspaceMutation(t *testing.T) {
	source, contract, commands, prepared := repositoryMutationExecutionFixture(t)
	stage, err := stageWorkspaceMutationFromRepositoryChange(
		t.Context(), source, contract.ID, commands, prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	plan := stage.Plan()
	if plan.OwnerID != contract.ID || plan.GitSourceSnapshotID != source.ID ||
		plan.WorkspaceRoot != source.Root || len(plan.Files) != 1 {
		t.Fatalf("generic workspace plan=%+v", plan)
	}
	projection, err := newWorkspaceStagedProjection(t.Context(), stage)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.VerifyExact(t.Context()); err != nil {
		t.Fatal(err)
	}
	command := queue.WorkspaceMutationCommand{Plan: plan}
	observed, err := observeWorkspaceMutation(t.Context(), command)
	if err != nil || observed != queue.WorkspaceMutationSource {
		t.Fatalf("source observation=%q error=%v", observed, err)
	}
	if _, err := stage.ApplyVerified(t.Context()); err != nil {
		t.Fatal(err)
	}
	observed, err = observeWorkspaceMutation(t.Context(), command)
	if err != nil || observed != queue.WorkspaceMutationPost {
		t.Fatalf("post observation=%q error=%v", observed, err)
	}
	if err := os.WriteFile(filepath.Join(source.Root, "unexpected.txt"), []byte("drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	observed, err = observeWorkspaceMutation(t.Context(), command)
	if err != nil || observed != queue.WorkspaceMutationIndeterminate {
		t.Fatalf("drift observation=%q error=%v", observed, err)
	}
}
