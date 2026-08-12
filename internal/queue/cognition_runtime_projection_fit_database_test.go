package queue

import (
	"errors"
	"testing"
)

func TestPostgresRequiredEnvelopeOverflowPersistsNoProjectionOrSnapshot(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, "required-envelope-overflow")
	fixture.Start.Budget.MaxInputBytes = 1
	fixture.Start.Budget.MaxInputTokens = 1 +
		fixture.Start.BrainBootstrap.AttestedBrain.Ref.Sampling.InputSpecialTokenReserve
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}

	_, err := repository.PrepareCognitionRuntimeSnapshot(ctx, CognitionRuntimeSnapshotCommand{
		Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
	})
	if !errors.Is(err, ErrCognitionEnvelopeBudget) {
		t.Fatalf("required envelope overflow error=%v", err)
	}

	var projections, snapshots int
	if err := pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM context_projections
		   WHERE job_id=$1 AND generation=$2 AND step_id=$3
		     AND work_kind='cognition_runtime_decision'),
		  (SELECT COUNT(*) FROM cognition_runtime_snapshots WHERE episode_id=$4)
	`, fixture.Authority.JobID, fixture.Authority.Generation,
		fixture.Authority.StepID, fixture.EpisodeID).Scan(&projections, &snapshots); err != nil {
		t.Fatal(err)
	}
	if projections != 0 || snapshots != 0 {
		t.Fatalf("overflow persisted projections/snapshots=%d/%d", projections, snapshots)
	}
}
