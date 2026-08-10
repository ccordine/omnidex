package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func lifecycleRetirementTestRepository(
	t *testing.T,
) (*Repository, *pgxpool.Pool, context.Context) {
	t.Helper()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	ctx := context.Background()
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	return repository, pool, ctx
}

func startLifecycleRetirementFixture(
	t *testing.T,
	label string,
) taskGenerationRetirementFixture {
	t.Helper()
	repository, pool, ctx := lifecycleRetirementTestRepository(t)
	return startTaskGenerationRetirementFixtureIn(t, repository, pool, ctx, label)
}

type lifecycleCognitionRowCounts struct {
	retirements   int
	cancellations int
	terminalSeals int
	sealSets      int
	sealEntries   int
	operations    int
}

func lifecycleCognitionCounts(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	operationID LifecycleOperationID,
) lifecycleCognitionRowCounts {
	t.Helper()
	var counts lifecycleCognitionRowCounts
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT
		 (SELECT COUNT(*) FROM cognition_lifecycle_retirements WHERE operation_id=$1),
		 (SELECT COUNT(*) FROM cognition_episode_cancellations WHERE episode_id=$2),
		 (SELECT COUNT(*) FROM cognition_terminal_seals WHERE episode_id=$2),
		 (SELECT COUNT(*) FROM cognition_lifecycle_operation_seals WHERE operation_id=$1),
		 (SELECT COUNT(*) FROM cognition_lifecycle_operation_seal_episodes WHERE operation_id=$1),
		 (SELECT COUNT(*) FROM job_lifecycle_operations WHERE operation_id=$1)
	`, operationID, fixture.EpisodeID).Scan(
		&counts.retirements, &counts.cancellations, &counts.terminalSeals,
		&counts.sealSets, &counts.sealEntries, &counts.operations,
	); err != nil {
		t.Fatal(err)
	}
	return counts
}

func assertLifecycleCognitionRetired(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	operationID LifecycleOperationID,
	kind LifecycleOperationKind,
) {
	t.Helper()
	var episodeStatus CognitionEpisodeStatus
	var attemptStatus model.StepAttemptStatus
	var authorityKind string
	var lifecycleOperationID *string
	var episodeCount int
	var retirementCount int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT episodes.status,attempts.status,seals.authority_kind,seals.lifecycle_operation_id,
		       sets.episode_count,
		       (SELECT COUNT(*) FROM cognition_lifecycle_retirements retirements
		        WHERE retirements.operation_id=$2 AND retirements.episode_id=$1)
		FROM cognition_episodes episodes
		JOIN job_step_attempts attempts ON attempts.job_id=episodes.job_id
		 AND attempts.generation=episodes.generation AND attempts.step_id=episodes.step_id
		 AND attempts.attempt=$3
		JOIN cognition_terminal_seals seals ON seals.episode_id=episodes.episode_id
		JOIN cognition_lifecycle_operation_seals sets ON sets.operation_id=$2
		WHERE episodes.episode_id=$1
	`, fixture.EpisodeID, operationID, fixture.Authority.Attempt).Scan(
		&episodeStatus, &attemptStatus, &authorityKind, &lifecycleOperationID,
		&episodeCount, &retirementCount,
	); err != nil {
		t.Fatal(err)
	}
	wantAttempt := model.StepAttemptCanceled
	if kind == LifecycleReplanJob {
		wantAttempt = model.StepAttemptSuperseded
	}
	if episodeStatus != CognitionEpisodeCanceled || attemptStatus != wantAttempt ||
		authorityKind != cognitionTerminalAuthorityLifecycle || lifecycleOperationID == nil ||
		*lifecycleOperationID != string(operationID) || episodeCount != 1 || retirementCount != 1 {
		t.Fatalf(
			"retired cognition status=%s attempt=%s authority=%s operation=%v episodes=%d retirements=%d",
			episodeStatus, attemptStatus, authorityKind, lifecycleOperationID, episodeCount, retirementCount,
		)
	}
	page, err := fixture.Repository.ReadCognitionSealedTrace(fixture.Context, fixture.EpisodeID,
		CognitionTracePageRequest{Limit: MaxCognitionTracePageSize})
	if err != nil {
		t.Fatal(err)
	}
	foundRetirement, foundCancellation := false, false
	for _, record := range page.Records {
		foundRetirement = foundRetirement || record.Kind == "lifecycle_retirement"
		foundCancellation = foundCancellation || record.Kind == "cancellation_evidence"
	}
	if !foundRetirement || !foundCancellation {
		t.Fatalf("sealed lifecycle trace lacks retirement/cancellation records: %+v", page.Records)
	}
}

func assertLifecycleCognitionRollback(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	operationID LifecycleOperationID,
) {
	t.Helper()
	counts := lifecycleCognitionCounts(t, fixture, operationID)
	if counts != (lifecycleCognitionRowCounts{}) {
		t.Fatalf("failed lifecycle operation left durable rows: %+v", counts)
	}
	var jobStatus string
	var episodeStatus CognitionEpisodeStatus
	var attemptStatus model.StepAttemptStatus
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT jobs.status,episodes.status,attempts.status
		FROM jobs
		JOIN cognition_episodes episodes ON episodes.job_id=jobs.id
		JOIN job_step_attempts attempts ON attempts.job_id=episodes.job_id
		 AND attempts.generation=episodes.generation AND attempts.step_id=episodes.step_id
		 AND attempts.attempt=$2
		WHERE jobs.id=$1 AND episodes.episode_id=$3
	`, fixture.Job.ID, fixture.Authority.Attempt, fixture.EpisodeID).Scan(
		&jobStatus, &episodeStatus, &attemptStatus,
	); err != nil {
		t.Fatal(err)
	}
	if jobStatus != model.JobStatusRunning || episodeStatus != CognitionEpisodeActive ||
		attemptStatus != model.StepAttemptActive {
		t.Fatalf("rollback status job=%s episode=%s attempt=%s", jobStatus, episodeStatus, attemptStatus)
	}
}
