package queue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestPostgresRepositoryMutationDurablyRecoversCreationAndDeletion(t *testing.T) {
	for _, test := range []struct {
		name    string
		command func(*testing.T, repositoryMutationDatabaseFixture) RepositoryMutationCommand
	}{
		{name: "create", command: repositoryMutationCreationCommand},
		{name: "delete", command: repositoryMutationDeletionCommand},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			fixture := newRepositoryMutationDatabaseFixture(t, "file-state-"+test.name)
			command := test.command(t, fixture)
			identity, err := repositoryMutationOperation(command)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.repository.prepareRepositoryMutation(
				fixture.ctx, fixture.authority, command, identity,
			); err != nil {
				t.Fatal(err)
			}
			loaded, err := fixture.repository.UnresolvedRepositoryMutation(
				fixture.ctx, fixture.job.ID, command.Generation,
			)
			if err != nil {
				t.Fatal(err)
			}
			if loaded == nil || len(loaded.ChangedFiles) != 1 ||
				loaded.ChangedFiles[0] != command.ChangedFiles[0] {
				t.Fatalf("durable %s command=%+v want %+v", test.name, loaded, command.ChangedFiles[0])
			}
		})
	}
}

func TestPostgresRepositoryMutationCreationRejectsSnapshotCollisionBeforeCallbacks(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "create-collision")
	command := repositoryMutationCreationCommand(t, fixture)
	command.ChangedFiles[0].FileID = fixture.snapshot.Files[0].ID
	command.ChangedFiles[0].Path = fixture.snapshot.Files[0].Path
	var classified, applied atomic.Bool
	err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, command,
		func(context.Context, RepositoryMutationCommand) (RepositoryMutationState, error) {
			classified.Store(true)
			return RepositoryMutationSource, nil
		},
		func(context.Context) error {
			applied.Store(true)
			return nil
		},
	)
	if err == nil {
		t.Fatal("repository mutation journal accepted creation over an indexed source path")
	}
	if classified.Load() || applied.Load() {
		t.Fatal("colliding creation reached repository callbacks")
	}
}

func TestPostgresRepositoryMutationDeletionRetryFinalizesExactAbsenceOnce(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "delete-retry")
	command := repositoryMutationDeletionCommand(t, fixture)
	var state atomic.Value
	state.Store(RepositoryMutationSource)
	var calls atomic.Int32
	if err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, command, stateClassifier(&state),
		func(context.Context) error {
			calls.Add(1)
			state.Store(RepositoryMutationPost)
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, command, stateClassifier(&state),
		func(context.Context) error {
			calls.Add(1)
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("deletion callback count=%d want 1", calls.Load())
	}
	assertRepositoryMutationEvidenceCount(t, fixture.pool, fixture.job.ID, 1)
}

func TestPostgresRepositoryMutationFailedCreationLeavesExactSourceAbsenceRetryable(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "create-rollback")
	command := repositoryMutationCreationCommand(t, fixture)
	var state atomic.Value
	state.Store(RepositoryMutationSource)
	want := errors.New("isolated creation did not apply")
	err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, command, stateClassifier(&state),
		func(context.Context) error { return want },
	)
	if !errors.Is(err, want) {
		t.Fatalf("failed creation error=%v", err)
	}
	operation, err := repositoryMutationOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT status,attempt_count FROM repository_mutation_operations WHERE id=$1
	`, operation.ID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != repositoryMutationPrepared || attempts != 1 {
		t.Fatalf("failed creation journal=%s/%d want prepared/1", status, attempts)
	}
	if err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, command, stateClassifier(&state),
		func(context.Context) error {
			state.Store(RepositoryMutationPost)
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	assertRepositoryMutationEvidenceCount(t, fixture.pool, fixture.job.ID, 1)
}

func repositoryMutationCreationCommand(
	t *testing.T,
	fixture repositoryMutationDatabaseFixture,
) RepositoryMutationCommand {
	t.Helper()
	command := fixture.command
	path := "created.go"
	fileID, err := repositoryfacts.FileIDForAbsentPath(fixture.snapshot, path)
	if err != nil {
		t.Fatal(err)
	}
	content := "package created\n"
	command.ChangedFiles = []RepositoryMutationFile{{
		FileID: fileID, Path: path,
		ExpectedPresent: true, ExpectedSHA256: repositoryMutationDigest(content),
		ExpectedSize: int64(len(content)), ExpectedMode: 0o644,
	}}
	return repositoryMutationCommandWithPatch(command,
		"diff --git a/created.go b/created.go\nnew file mode 100644\n--- /dev/null\n+++ b/created.go\n@@ -0,0 +1 @@\n+package created\n",
	)
}

func repositoryMutationDeletionCommand(
	t *testing.T,
	fixture repositoryMutationDatabaseFixture,
) RepositoryMutationCommand {
	t.Helper()
	command := fixture.command
	command.ChangedFiles[0].ExpectedPresent = false
	command.ChangedFiles[0].ExpectedSHA256 = ""
	command.ChangedFiles[0].ExpectedSize = 0
	command.ChangedFiles[0].ExpectedMode = 0
	return repositoryMutationCommandWithPatch(command,
		"diff --git a/value.go b/value.go\ndeleted file mode 100644\n--- a/value.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-source-value\n",
	)
}

func repositoryMutationCommandWithPatch(
	command RepositoryMutationCommand,
	patch string,
) RepositoryMutationCommand {
	command.Patch = patch
	command.PatchSHA256 = repositoryMutationDigest(patch)
	command.StageID = "repository_change_stage_" + repositoryMutationDigest(patch)
	return command
}
