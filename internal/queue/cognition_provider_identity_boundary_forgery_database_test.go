package queue

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresProviderIdentityBoundariesRejectRehashedTimestampAndNull(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	donor := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(ctx, donor.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	boundaries := []struct {
		name   string
		assert func(*testing.T, providerBoundaryObservationMutation)
	}{
		{"call", func(t *testing.T, mutation providerBoundaryObservationMutation) {
			fixture := startTaskGenerationRetirementFixtureIn(t, repository, pool, ctx,
				providerBoundaryLabel("call", mutation))
			assertDirectCallObservationForgeryRejected(t, fixture, mutation)
		}},
		{"bootstrap", func(t *testing.T, mutation providerBoundaryObservationMutation) {
			target := newCognitionDatabaseFixture(t, repository)
			assertDirectBootstrapObservationForgeryRejected(t, pool, donor, target, mutation)
		}},
		{"replay", func(t *testing.T, mutation providerBoundaryObservationMutation) {
			fixture := startTaskGenerationRetirementFixtureIn(t, repository, pool, ctx,
				providerBoundaryLabel("replay", mutation))
			assertDirectReplayObservationForgeryRejected(t, repository, fixture, mutation)
		}},
		{"process", func(t *testing.T, mutation providerBoundaryObservationMutation) {
			fixture := startTaskGenerationRetirementFixtureIn(t, repository, pool, ctx,
				providerBoundaryLabel("process", mutation))
			assertDirectProcessObservationForgeryRejected(t, repository, fixture, false, mutation, "")
		}},
		{"postseal", func(t *testing.T, mutation providerBoundaryObservationMutation) {
			fixture := startTaskGenerationRetirementFixtureIn(t, repository, pool, ctx,
				providerBoundaryLabel("postseal", mutation))
			assertDirectProcessObservationForgeryRejected(t, repository, fixture, true, mutation, "")
		}},
	}
	for _, boundary := range boundaries {
		boundary := boundary
		for _, mutation := range []providerBoundaryObservationMutation{
			providerBoundaryNoncanonicalTimestamp, providerBoundaryNullTimestamp,
		} {
			mutation := mutation
			t.Run(boundary.name+"/"+string(mutation), func(t *testing.T) {
				boundary.assert(t, mutation)
			})
		}
	}
}

func TestPostgresProcessBoundariesRejectRehashedStableBrainForgeries(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	for _, postseal := range []bool{false, true} {
		boundary := "process"
		if postseal {
			boundary = "postseal"
		}
		for _, mutation := range []string{"schema", "self_hash", "substituted_hardware"} {
			postseal, boundary, mutation := postseal, boundary, mutation
			t.Run(boundary+"/stable_brain_"+mutation, func(t *testing.T) {
				fixture := startTaskGenerationRetirementFixtureIn(t, repository, pool, ctx,
					fmt.Sprintf("boundary-%s-stable-%s", boundary, mutation))
				assertDirectProcessObservationForgeryRejected(
					t, repository, fixture, postseal, "", mutation,
				)
			})
		}
	}
}

func providerBoundaryLabel(boundary string, mutation providerBoundaryObservationMutation) string {
	return fmt.Sprintf("boundary-%s-%s", boundary, mutation)
}

func assertDirectCallObservationForgeryRejected(
	t *testing.T, fixture taskGenerationRetirementFixture, mutation providerBoundaryObservationMutation,
) {
	t.Helper()
	journal := captureAcceptedPolicyResult(t, fixture)
	result := providerBoundaryObject(t, journal.result)
	observation := result["provider_observation"].(map[string]any)
	mutateProviderBoundaryObservation(t, observation, journal.result.ProviderObservation.ObservedAt, mutation)
	result["provider_observation"] = observation
	raw := providerBoundaryCanonical(t, result)
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	authority, err := cognitionPolicyCallAuthority(journal.attempt)
	if err == nil {
		err = insertCognitionProviderIdentityEvidenceTx(fixture.Context, tx, authority,
			journal.attempt, journal.result, journal.evidence.ProviderIdentity)
	}
	if err == nil {
		err = insertCognitionResponseEvidenceTx(fixture.Context, tx, authority,
			journal.attempt, journal.result, journal.evidence.Response)
	}
	if err == nil {
		err = insertCognitionProviderResponseCaptureTx(fixture.Context, tx, authority,
			journal.attempt, journal.result, journal.evidence.ProviderResponseCapture)
	}
	if err == nil {
		_, err = tx.Exec(fixture.Context, `UPDATE cognition_policy_calls
			SET status=$2,result_json=$3,result_sha256=$4,finished_at=clock_timestamp()
			WHERE call_id=$1`, journal.attempt.ID, journal.result.Status,
			string(raw), cognitionPayloadSHA(raw))
	}
	assertProviderBoundaryCommitRejected(t, tx, err)
}

func assertDirectBootstrapObservationForgeryRejected(
	t *testing.T, pool *pgxpool.Pool, donor, target cognitionDatabaseFixture,
	mutation providerBoundaryObservationMutation,
) {
	t.Helper()
	brain := providerBoundaryObject(t, target.Start.BrainBootstrap.AttestedBrain)
	observation := brain["bootstrap_provider_observation"].(map[string]any)
	mutateProviderBoundaryObservation(t, observation,
		target.Start.BrainBootstrap.AttestedBrain.BootstrapObservation.ObservedAt, mutation)
	brain["bootstrap_provider_observation"] = observation
	raw := providerBoundaryCanonical(t, brain)
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	header, err := loadTaskLedgerHeaderTx(t.Context(), tx, target.Authority.JobID, false)
	if err != nil {
		t.Fatal(err)
	}
	set, err := loadWorkingSetSnapshotTx(t.Context(), tx, header, target.Authority.Generation, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(t.Context(), `INSERT INTO cognition_episodes (
		episode_id,schema_name,job_id,generation,step_id,created_attempt,created_worker_id,
		ledger_id,working_set_id,scenario_id,scenario_sha256,goal_json,goal_sha256,
		completion_authority_json,completion_authority_sha256,action_catalog_json,
		action_catalog_id,action_catalog_version,action_catalog_sha256,runtime_budget_json,
		runtime_budget_sha256,attested_brain_json,attested_brain_sha256,fact_authority_json,
		fact_authority_sha256,fact_authority_identity_json,fact_authority_identity_sha256,
		current_revision,current_revision_sha256,status
	) SELECT $1,schema_name,$2,$3,$4,$5,$6,$7,$8,scenario_id,scenario_sha256,goal_json,
		goal_sha256,completion_authority_json,completion_authority_sha256,action_catalog_json,
		action_catalog_id,action_catalog_version,action_catalog_sha256,runtime_budget_json,
		runtime_budget_sha256,$9,$10,fact_authority_json,fact_authority_sha256,
		fact_authority_identity_json,fact_authority_identity_sha256,current_revision,
		current_revision_sha256,'active' FROM cognition_episodes WHERE episode_id=$11`,
		target.EpisodeID, target.Authority.JobID, target.Authority.Generation,
		target.Authority.StepID, target.Authority.Attempt, target.Authority.WorkerID,
		header.ID, set.ID, string(raw), cognitionPayloadSHA(raw), donor.EpisodeID)
	if err == nil {
		t.Fatal("direct SQL accepted a forged bootstrap observation before provider associations")
	}
}
