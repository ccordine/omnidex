package queue

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostgresRepositoryMutationBlocksReplanUntilExactPostIsDurable(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "replan-boundary")
	var state atomic.Value
	state.Store(RepositoryMutationSource)
	entered := make(chan struct{})
	release := make(chan struct{})
	mutationResult := make(chan error, 1)
	go func() {
		mutationResult <- fixture.repository.ApplyRepositoryMutation(
			fixture.ctx, fixture.authority, fixture.command, stateClassifier(&state),
			func(callCtx context.Context) error {
				close(entered)
				select {
				case <-callCtx.Done():
					return callCtx.Err()
				case <-release:
					state.Store(RepositoryMutationPost)
					return nil
				}
			},
		)
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("repository mutation callback did not start")
	}
	status, attempts, evidenceID := repositoryMutationJournalStatus(t, fixture)
	if status != repositoryMutationApplying || attempts != 1 || evidenceID != nil {
		t.Fatalf("applying journal=%s/%d/%v", status, attempts, evidenceID)
	}
	assertRepositoryMutationEvidenceCount(t, fixture.pool, fixture.job.ID, 0)
	_, err := fixture.repository.ReplanJob(fixture.ctx, testReplanCommand(
		t, fixture.job.ID, "mutation-applying", "Do not supersede an applying mutation.",
	))
	if !errors.Is(err, ErrRepositoryMutationUnresolved) {
		t.Fatalf("replan during applying mutation error=%v", err)
	}
	close(release)
	if err := <-mutationResult; err != nil {
		t.Fatal(err)
	}
	status, attempts, evidenceID = repositoryMutationJournalStatus(t, fixture)
	if status != repositoryMutationApplied || attempts != 1 || evidenceID == nil {
		t.Fatalf("applied journal=%s/%d/%v", status, attempts, evidenceID)
	}
	assertRepositoryMutationEvidenceCount(t, fixture.pool, fixture.job.ID, 1)
	replanned, err := fixture.repository.ReplanJob(fixture.ctx, testReplanCommand(
		t, fixture.job.ID, "mutation-applied", "Replan after exact mutation finalization.",
	))
	if err != nil {
		t.Fatal(err)
	}
	if replanned.CurrentGeneration != 2 {
		t.Fatalf("replanned generation=%d want 2", replanned.CurrentGeneration)
	}
}

func TestPostgresRepositoryMutationRejectsWrongAuthorityBeforeCallbacks(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "wrong-authority")
	wrong := fixture.command
	wrong.WorkerID += "-stale"
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
	if !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("wrong worker error=%v", err)
	}
	if classified.Load() || applied.Load() {
		t.Fatal("repository callbacks ran without queue claim authority")
	}
	assertRepositoryMutationEvidenceCount(t, fixture.pool, fixture.job.ID, 0)
	var operations int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT COUNT(*) FROM repository_mutation_operations WHERE job_id=$1
	`, fixture.job.ID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 0 {
		t.Fatalf("rejected repository mutation persisted %d operations", operations)
	}
}

func TestPostgresRepositoryMutationFailureWithExactSourceRemainsPrepared(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "failed-apply")
	var state atomic.Value
	state.Store(RepositoryMutationSource)
	want := errors.New("authoritative filesystem mutation failed")
	err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, fixture.command, stateClassifier(&state),
		func(context.Context) error { return want },
	)
	if !errors.Is(err, want) {
		t.Fatalf("callback failure=%v", err)
	}
	status, attempts, evidenceID := repositoryMutationJournalStatus(t, fixture)
	if status != repositoryMutationPrepared || attempts != 1 || evidenceID != nil {
		t.Fatalf("failed journal=%s/%d/%v", status, attempts, evidenceID)
	}
	assertRepositoryMutationEvidenceCount(t, fixture.pool, fixture.job.ID, 0)
	assertRepositoryMutationEventCount(t, fixture, "repository_mutation_prepared", 1)
	assertRepositoryMutationEventCount(t, fixture, "repository_mutation_applying", 1)
	assertRepositoryMutationEventCount(t, fixture, "repository_mutation_deferred", 1)
	assertRepositoryMutationEventCount(t, fixture, "repository_mutation_applied", 0)
	var payload string
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT event.payload::text
		FROM omni_run_events AS event
		WHERE event.run_id=NULLIF((
			SELECT metadata->>'telemetry_run_id' FROM jobs WHERE id=$1
		), '')::uuid AND event.event_type='repository_mutation_deferred'
	`, fixture.job.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, want.Error()) ||
		!strings.Contains(payload, repositoryMutationFailureSHA(
			"apply authoritative repository mutation: "+want.Error(),
		)) {
		t.Fatalf("repository mutation telemetry exposed or omitted failure identity: %s", payload)
	}
}
