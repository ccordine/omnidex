package queue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestPostgresClassifierFailureLeavesPreparedJournalAndBlocksReplan(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "classifier-failure")
	want := errors.New("repository inventory unavailable")
	var applied atomic.Bool
	err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, fixture.command,
		func(context.Context, RepositoryMutationCommand) (RepositoryMutationState, error) {
			return "", want
		},
		func(context.Context) error {
			applied.Store(true)
			return nil
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("classifier error=%v", err)
	}
	if applied.Load() {
		t.Fatal("filesystem callback ran after classifier failure")
	}
	status, attempts, evidenceID := repositoryMutationJournalStatus(t, fixture)
	if status != repositoryMutationPrepared || attempts != 0 || evidenceID != nil {
		t.Fatalf("classifier failure journal=%s/%d/%v", status, attempts, evidenceID)
	}
	_, err = fixture.repository.ReplanJob(fixture.ctx, testReplanCommand(
		t, fixture.job.ID, "mutation-prepared", "Do not supersede a prepared mutation.",
	))
	if !errors.Is(err, ErrRepositoryMutationUnresolved) {
		t.Fatalf("replan with prepared mutation error=%v", err)
	}
}

func TestPostgresCallbackErrorWithExactPostFinalizesAsSuccess(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "error-after-post")
	var state atomic.Value
	state.Store(RepositoryMutationSource)
	err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, fixture.command, stateClassifier(&state),
		func(context.Context) error {
			state.Store(RepositoryMutationPost)
			return errors.New("callback lost its response after exact write")
		},
	)
	if err != nil {
		t.Fatalf("exact post state did not finalize: %v", err)
	}
	status, attempts, evidenceID := repositoryMutationJournalStatus(t, fixture)
	if status != repositoryMutationApplied || attempts != 1 || evidenceID == nil {
		t.Fatalf("exact-post recovery journal=%s/%d/%v", status, attempts, evidenceID)
	}
	assertRepositoryMutationEvidenceCount(t, fixture.pool, fixture.job.ID, 1)
	assertRepositoryMutationEventCount(t, fixture, "repository_mutation_prepared", 1)
	assertRepositoryMutationEventCount(t, fixture, "repository_mutation_applying", 1)
	assertRepositoryMutationEventCount(t, fixture, "repository_mutation_applied", 1)

	var replayApplied atomic.Bool
	err = fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, fixture.command, stateClassifier(&state),
		func(context.Context) error {
			replayApplied.Store(true)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if replayApplied.Load() {
		t.Fatal("exact applied replay invoked filesystem callback")
	}
	assertRepositoryMutationEvidenceCount(t, fixture.pool, fixture.job.ID, 1)
	assertRepositoryMutationEventCount(t, fixture, "repository_mutation_applied", 1)
}

func TestPostgresMutationSourceFactsMustMatchDurableSnapshot(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "source-mismatch")
	wrong := fixture.command
	wrong.ChangedFiles = append([]RepositoryMutationFile(nil), fixture.command.ChangedFiles...)
	wrong.ChangedFiles[0].SourceSize++
	var classified, applied atomic.Bool
	err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, wrong,
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
		t.Fatal("repository mutation accepted source facts absent from its durable snapshot")
	}
	if classified.Load() || applied.Load() {
		t.Fatal("source-mismatched mutation reached repository callbacks")
	}
	var operations int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT COUNT(*) FROM repository_mutation_operations WHERE job_id=$1
	`, fixture.job.ID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 0 {
		t.Fatalf("source-mismatched mutation persisted %d operations", operations)
	}
}

func TestPostgresAppliedMutationAuthorityAndEvidenceAreImmutable(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "immutable")
	var state atomic.Value
	state.Store(RepositoryMutationSource)
	if err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, fixture.command, stateClassifier(&state),
		func(context.Context) error {
			state.Store(RepositoryMutationPost)
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	operation, err := repositoryMutationOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	for name, statement := range map[string]string{
		"operation": `UPDATE repository_mutation_operations SET patch=patch || 'x' WHERE id=$1`,
		"file":      `UPDATE repository_mutation_files SET expected_size=expected_size+1 WHERE operation_id=$1`,
		"evidence": `UPDATE evidence SET payload_json='{}'::jsonb WHERE id=(
			SELECT evidence_id FROM repository_mutation_operations WHERE id=$1
		)`,
	} {
		if _, err := fixture.pool.Exec(fixture.ctx, statement, operation.ID); err == nil {
			t.Fatalf("database allowed accepted repository mutation %s to change", name)
		}
	}
}
