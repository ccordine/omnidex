package queue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresPreparedMutationWithExactPostFinalizesWithoutReapplying(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "prepared-post")
	identity, err := repositoryMutationOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.prepareRepositoryMutation(
		fixture.ctx, fixture.command, identity,
	); err != nil {
		t.Fatal(err)
	}
	var applied atomic.Bool
	err = fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.command,
		func(context.Context, RepositoryMutationCommand) (RepositoryMutationState, error) {
			return RepositoryMutationPost, nil
		},
		func(context.Context) error {
			applied.Store(true)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Load() {
		t.Fatal("prepared exact-post recovery reapplied the patch")
	}
	status, attempts, evidenceID := repositoryMutationJournalStatus(t, fixture)
	if status != repositoryMutationApplied || attempts != 0 || evidenceID == nil {
		t.Fatalf("recovered journal=%s/%d/%v", status, attempts, evidenceID)
	}
}

func TestPostgresUnresolvedMutationLoadsTheExactDurableCommand(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "durable-load")
	identity, err := repositoryMutationOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.prepareRepositoryMutation(
		fixture.ctx, fixture.command, identity,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := fixture.repository.UnresolvedRepositoryMutation(
		fixture.ctx, fixture.job.ID, fixture.command.Generation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("prepared repository mutation was absent from durable recovery query")
	}
	loadedIdentity, err := repositoryMutationOperation(*loaded)
	if err != nil {
		t.Fatal(err)
	}
	if loadedIdentity != identity || loaded.Patch != fixture.command.Patch ||
		len(loaded.ChangedFiles) != 1 || loaded.ChangedFiles[0] != fixture.command.ChangedFiles[0] {
		t.Fatalf("loaded mutation=%+v want exact identity %+v", loaded, identity)
	}
	var status, workerID string
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT status, COALESCE(worker_id, '') FROM job_steps WHERE id=$1
	`, fixture.stepID).Scan(&status, &workerID); err != nil {
		t.Fatal(err)
	}
	if status != model.StepStatusRunning || workerID != fixture.workerID {
		t.Fatalf("recovery read changed step claim to %s/%q", status, workerID)
	}
}

func TestPostgresApplyingMutationWithExactPostRecoversAfterCrashBoundary(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "applying-post")
	identity, err := repositoryMutationOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.prepareRepositoryMutation(
		fixture.ctx, fixture.command, identity,
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.markRepositoryMutationApplying(
		fixture.ctx, fixture.command, identity,
	); err != nil {
		t.Fatal(err)
	}
	var applied atomic.Bool
	err = fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.command,
		func(context.Context, RepositoryMutationCommand) (RepositoryMutationState, error) {
			return RepositoryMutationPost, nil
		},
		func(context.Context) error {
			applied.Store(true)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Load() {
		t.Fatal("applying exact-post crash recovery reapplied the patch")
	}
	status, attempts, evidenceID := repositoryMutationJournalStatus(t, fixture)
	if status != repositoryMutationApplied || attempts != 1 || evidenceID == nil {
		t.Fatalf("crash recovery journal=%s/%d/%v", status, attempts, evidenceID)
	}
}

func TestPostgresExactPostFinalizesAfterCancellationRacesWithApplying(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "canceled-post")
	identity, err := repositoryMutationOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.prepareRepositoryMutation(
		fixture.ctx, fixture.command, identity,
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.markRepositoryMutationApplying(
		fixture.ctx, fixture.command, identity,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.CancelJob(
		fixture.ctx, testCancelCommand(
			t, fixture.job.ID, "mutation-recovery-cleanup", "cancel after filesystem mutation began",
		),
	); err != nil {
		t.Fatal(err)
	}
	var applied atomic.Bool
	err = fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.command,
		func(context.Context, RepositoryMutationCommand) (RepositoryMutationState, error) {
			return RepositoryMutationPost, nil
		},
		func(context.Context) error {
			applied.Store(true)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Load() {
		t.Fatal("canceled exact-post recovery invoked a new filesystem mutation")
	}
	status, _, evidenceID := repositoryMutationJournalStatus(t, fixture)
	if status != repositoryMutationApplied || evidenceID == nil {
		t.Fatalf("canceled exact-post journal=%s/%v", status, evidenceID)
	}
}

func TestPostgresIndeterminateMutationPersistsAndBlocksReplan(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "indeterminate")
	var applied atomic.Bool
	err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.command,
		func(context.Context, RepositoryMutationCommand) (RepositoryMutationState, error) {
			return RepositoryMutationIndeterminate, nil
		},
		func(context.Context) error {
			applied.Store(true)
			return nil
		},
	)
	if !errors.Is(err, ErrRepositoryMutationUnresolved) {
		t.Fatalf("indeterminate mutation error=%v", err)
	}
	if applied.Load() {
		t.Fatal("indeterminate repository state invoked mutation callback")
	}
	status, attempts, evidenceID := repositoryMutationJournalStatus(t, fixture)
	if status != repositoryMutationIndeterminate || attempts != 0 || evidenceID != nil {
		t.Fatalf("indeterminate journal=%s/%d/%v", status, attempts, evidenceID)
	}
	assertRepositoryMutationEventCount(t, fixture, "repository_mutation_prepared", 1)
	assertRepositoryMutationEventCount(t, fixture, "repository_mutation_indeterminate", 1)
	assertRepositoryMutationEventCount(t, fixture, "repository_mutation_applied", 0)
	_, err = fixture.repository.ReplanJob(fixture.ctx, testReplanCommand(
		t, fixture.job.ID, "mutation-indeterminate", "Do not supersede unknown filesystem state.",
	))
	if !errors.Is(err, ErrRepositoryMutationUnresolved) {
		t.Fatalf("replan with indeterminate mutation error=%v", err)
	}
}

func TestPostgresMutationCommandConflictCannotReuseAStage(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "identity-conflict")
	identity, err := repositoryMutationOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.prepareRepositoryMutation(
		fixture.ctx, fixture.command, identity,
	); err != nil {
		t.Fatal(err)
	}
	changed := fixture.command
	changed.ChangedFiles = append([]RepositoryMutationFile(nil), fixture.command.ChangedFiles...)
	changed.ChangedFiles[0].ExpectedSize++
	var called atomic.Bool
	err = fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, changed,
		func(context.Context, RepositoryMutationCommand) (RepositoryMutationState, error) {
			called.Store(true)
			return RepositoryMutationPost, nil
		}, repositoryMutationSuccessCallback,
	)
	if !errors.Is(err, ErrRepositoryMutationConflict) {
		t.Fatalf("stage identity conflict error=%v", err)
	}
	if called.Load() {
		t.Fatal("conflicting stage command reached repository classifier")
	}
}
