package queue

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresInitialCognitionBootstrapRejectsObservationBeforeAttemptClaim(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(
		t, repository, pool, ctx, "initial-bootstrap-before-claim",
	)
	claimedAt := cognitionAttemptClaimedAt(t, pool, fixture.Authority)
	fixture.Start.BrainBootstrap = cognitionBootstrapAt(
		t, fixture.Start.BrainBootstrap,
		claimedAt.UTC().Truncate(time.Microsecond).Add(-time.Microsecond),
	)
	fixture.Start.ProviderProcessActivation = cognitionGuardProviderProcessActivationFor(
		t, ctx, fixture.EpisodeID, fixture.Authority,
		fixture.Start.BrainBootstrap.AttestedBrain,
	)

	_, err := repository.StartCognitionEpisode(
		ctx, fixture.Start, cognitionTestFactAuthority(),
	)
	if err == nil || !strings.Contains(err.Error(), "bootstrap") {
		t.Fatalf("initial bootstrap one microsecond before claim error=%v", err)
	}
}

func TestPostgresActiveReplayBootstrapRejectsObservationBeforeAttemptClaim(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "replay-bootstrap-before-claim",
	)
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	claimedAt := cognitionAttemptClaimedAt(t, pool, replacement)
	replay := cognitionReplayStartForTest(t, fixture.Start, replacement)
	replay.BrainBootstrap = cognitionBootstrapAt(
		t, replay.BrainBootstrap,
		claimedAt.UTC().Truncate(time.Microsecond).Add(-time.Microsecond),
	)
	replay.ProviderProcessActivation = cognitionGuardProviderProcessActivationFor(
		t, ctx, fixture.EpisodeID, replacement, replay.BrainBootstrap.AttestedBrain,
	)

	_, err := repository.StartCognitionEpisode(ctx, replay, cognitionTestFactAuthority())
	if err == nil || !strings.Contains(err.Error(), "replay bootstrap") {
		t.Fatalf("replay bootstrap one microsecond before claim error=%v", err)
	}
}

func cognitionAttemptClaimedAt(
	t *testing.T, pool *pgxpool.Pool, authority model.StepAttemptAuthority,
) time.Time {
	t.Helper()
	var claimedAt time.Time
	if err := pool.QueryRow(t.Context(), `SELECT claimed_at FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5`,
		authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID,
	).Scan(&claimedAt); err != nil {
		t.Fatal(err)
	}
	return claimedAt
}
