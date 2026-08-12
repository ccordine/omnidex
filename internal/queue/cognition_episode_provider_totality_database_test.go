package queue

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const cognitionEpisodeProviderStartConstraint = "cognition_episodes_provider_start_totality"

func TestCognitionEpisodeProviderStartTotalityMigrationIsDeferredAndReverseComplete(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/060_cognition_provider_process_observation_zzz_episode_start_totality.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"AFTER INSERT ON cognition_episodes DEFERRABLE INITIALLY DEFERRED",
		"cognition_episode_provider_identity_evidence", "cognition_provider_process_observations",
		"COUNT(*)", "sequence=1", "episode_invocation", "created_attempt",
		"process.job_id=NEW.job_id", "process.generation=NEW.generation",
		"process.step_id=NEW.step_id", "process.worker_id=NEW.created_worker_id",
		"process.observed_at>=",
		"cognition_provider_observed_identity_is_exact", "cognition_provider_process_receipt_is_exact",
	} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("episode-start provider totality migration lacks %q", required)
		}
	}
}

func TestPostgresCognitionEpisodeRequiresOneExactInitialProviderOutcome(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	donor := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(ctx, donor.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatalf("valid episode start: %v", err)
	}
	var triggerCount int
	var deferred, initiallyDeferred bool
	if err := pool.QueryRow(ctx, `SELECT COUNT(*),bool_and(tgdeferrable),bool_and(tginitdeferred)
		FROM pg_trigger WHERE tgrelid='cognition_episodes'::regclass
		  AND tgname=$1 AND NOT tgisinternal`, cognitionEpisodeProviderStartConstraint).Scan(
		&triggerCount, &deferred, &initiallyDeferred,
	); err != nil || triggerCount != 1 || !deferred || !initiallyDeferred {
		t.Fatalf("episode provider trigger count/deferred/initial=%d/%v/%v error=%v",
			triggerCount, deferred, initiallyDeferred, err)
	}

	tests := []struct {
		name    string
		prepare func(*cognitionDatabaseFixture)
		created func(cognitionDatabaseFixture) model.StepAttemptAuthority
		stage   func(*testing.T, context.Context, pgx.Tx, cognitionDatabaseFixture) error
	}{
		{"bootstrap association omitted", nil, nil, stageExactInitialProcess},
		{"process observation omitted", nil, nil, stageExactInitialBootstrap},
		{"bootstrap evidence substituted", nil, nil, stageSubstitutedInitialBootstrap},
		{"stored stable Brain substituted", func(target *cognitionDatabaseFixture) {
			target.Start.BrainBootstrap = cognitionTestBrainBootstrapWithCPU("d")
		}, nil, stageSubstitutedInitialBrain},
		{"initial sequence substituted", nil, nil, stageSubstitutedInitialSequence},
		{"episode invocation receipt substituted", nil, nil, stageSubstitutedInitialReceipt},
		{"created actor substituted", nil, func(target cognitionDatabaseFixture) model.StepAttemptAuthority {
			return replaceCognitionAttemptForTest(t, pool, target.Authority)
		}, stageExactInitialProviderOutcome},
		{"more than one initial observation", nil, nil, stageTwoInitialProcessObservations},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := newCognitionDatabaseFixture(t, repository)
			if test.prepare != nil {
				test.prepare(&target)
			}
			created := target.Authority
			if test.created != nil {
				created = test.created(target)
			}
			tx := insertDirectEpisodeForProviderTotalityTest(t, ctx, pool, donor, target, created)
			defer tx.Rollback(context.Background())
			if err := test.stage(t, ctx, tx, target); err != nil {
				t.Fatalf("stage direct provider mutation: %v", err)
			}
			_, err := tx.Exec(ctx, "SET CONSTRAINTS "+cognitionEpisodeProviderStartConstraint+" IMMEDIATE")
			if err == nil || !strings.Contains(err.Error(), "cognition episode requires exactly one exact initial") {
				t.Fatalf("direct episode provider mutation error=%v", err)
			}
		})
	}
}

func TestPostgresCognitionPreEpisodeBootstrapFailureNeedsNoEpisode(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, "provider-totality-pre-episode")
	failure := cognitionBrainBootstrapFailure(t, fixture, llm.ProviderIdentityTokenizer)
	if err := repository.RecordCognitionBrainBootstrapFailure(
		ctx, fixture.Authority, fixture.EpisodeID, failure,
	); err != nil {
		t.Fatal(err)
	}
	var episodes, failures int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM cognition_episodes WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_activation_failures WHERE episode_id=$1)`,
		fixture.EpisodeID).Scan(&episodes, &failures); err != nil || episodes != 0 || failures != 1 {
		t.Fatalf("pre-episode episode/failure=%d/%d error=%v", episodes, failures, err)
	}
}

func insertDirectEpisodeForProviderTotalityTest(
	t *testing.T, ctx context.Context, pool interface {
		Begin(context.Context) (pgx.Tx, error)
	},
	donor, target cognitionDatabaseFixture, created model.StepAttemptAuthority,
) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, target.Authority.JobID, false)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	set, err := loadWorkingSetSnapshotTx(ctx, tx, header, target.Authority.Generation, false)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	brainJSON, brainSHA, err := cognitionJSON(target.Start.BrainBootstrap.AttestedBrain)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO cognition_episodes (
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
		target.EpisodeID, created.JobID, created.Generation, created.StepID, created.Attempt,
		created.WorkerID, header.ID, set.ID, string(brainJSON), brainSHA, donor.EpisodeID)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	return tx
}

func stageExactInitialBootstrap(
	_ *testing.T, ctx context.Context, tx pgx.Tx, target cognitionDatabaseFixture,
) error {
	return insertCognitionEpisodeBootstrapEvidenceTx(ctx, tx, target.EpisodeID, target.Start.BrainBootstrap)
}

func stageExactInitialProcess(
	_ *testing.T, ctx context.Context, tx pgx.Tx, target cognitionDatabaseFixture,
) error {
	return persistCognitionProviderProcessActivationTx(ctx, tx, target.Authority,
		providerTotalityEpisode(target, target.Authority, target.Start.BrainBootstrap.AttestedBrain),
		target.Start.ProviderProcessActivation, "")
}

func stageExactInitialProviderOutcome(
	t *testing.T, ctx context.Context, tx pgx.Tx, target cognitionDatabaseFixture,
) error {
	if err := stageExactInitialBootstrap(t, ctx, tx, target); err != nil {
		return err
	}
	return stageExactInitialProcess(t, ctx, tx, target)
}

func stageSubstitutedInitialBootstrap(
	t *testing.T, ctx context.Context, tx pgx.Tx, target cognitionDatabaseFixture,
) error {
	evidence := cognitionProviderFailureEvidence(t, target.Start.BrainBootstrap.AttestedBrain, llm.ProviderIdentityTokenizer)
	if err := insertCognitionProviderIdentityEvidenceBodyTx(ctx, tx, evidence); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO cognition_episode_provider_identity_evidence
		(episode_id,evidence_id) VALUES ($1,$2)`, target.EpisodeID, evidence.Ref.ID); err != nil {
		return err
	}
	return stageExactInitialProcess(t, ctx, tx, target)
}

func stageSubstitutedInitialBrain(
	t *testing.T, ctx context.Context, tx pgx.Tx, target cognitionDatabaseFixture,
) error {
	if err := stageExactInitialBootstrap(t, ctx, tx, target); err != nil {
		return err
	}
	brain := cognitionTestBrain()
	activation := target.Start.ProviderProcessActivation
	return persistCognitionProviderProcessActivationTx(ctx, tx, target.Authority,
		providerTotalityEpisode(target, target.Authority, brain), activation, "")
}

func stageSubstitutedInitialReceipt(
	t *testing.T, ctx context.Context, tx pgx.Tx, target cognitionDatabaseFixture,
) error {
	if err := makeInitialProcessMutable(t, ctx, tx, target); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `WITH changed AS (
		SELECT observation_id,cognition_canonical_jsonb(jsonb_set(
			receipt_json::jsonb,'{purpose}',to_jsonb('forged'::TEXT))) AS body
		FROM cognition_provider_process_observations WHERE episode_id=$1
	) UPDATE cognition_provider_process_observations observations
		SET receipt_json=changed.body,receipt_sha256=encode(digest(changed.body,'sha256'),'hex')
		FROM changed WHERE observations.observation_id=changed.observation_id`, target.EpisodeID)
	if err != nil {
		return err
	}
	return restoreInitialProcessImmutability(ctx, tx)
}

func stageSubstitutedInitialSequence(
	t *testing.T, ctx context.Context, tx pgx.Tx, target cognitionDatabaseFixture,
) error {
	if err := makeInitialProcessMutable(t, ctx, tx, target); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE cognition_provider_process_observations
		SET sequence=2 WHERE episode_id=$1`, target.EpisodeID); err != nil {
		return err
	}
	return restoreInitialProcessImmutability(ctx, tx)
}

func makeInitialProcessMutable(
	t *testing.T, ctx context.Context, tx pgx.Tx, target cognitionDatabaseFixture,
) error {
	if err := stageExactInitialProviderOutcome(t, ctx, tx, target); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS
		cognition_provider_process_observations_exact,
		cognition_provider_process_failure_outcome_exclusive,
		cognition_provider_process_observation_cross_table_unique IMMEDIATE`); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `ALTER TABLE cognition_provider_process_observations
		DISABLE TRIGGER cognition_provider_process_observations_immutable`)
	return err
}

func restoreInitialProcessImmutability(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `ALTER TABLE cognition_provider_process_observations
		ENABLE TRIGGER cognition_provider_process_observations_immutable`)
	return err
}

func stageTwoInitialProcessObservations(
	t *testing.T, ctx context.Context, tx pgx.Tx, target cognitionDatabaseFixture,
) error {
	if err := stageExactInitialProviderOutcome(t, ctx, tx, target); err != nil {
		return err
	}
	activation := cognitionGuardProviderProcessActivationFor(t, ctx, target.EpisodeID,
		target.Authority, target.Start.BrainBootstrap.AttestedBrain)
	return persistCognitionProviderProcessActivationTx(ctx, tx, target.Authority,
		providerTotalityEpisode(target, target.Authority, target.Start.BrainBootstrap.AttestedBrain), activation, "")
}

func providerTotalityEpisode(
	target cognitionDatabaseFixture, authority model.StepAttemptAuthority, brain cognitionpolicy.AttestedBrain,
) CognitionEpisode {
	return CognitionEpisode{EpisodeID: target.EpisodeID, Authority: authority,
		AttestedBrain: brain, Status: CognitionEpisodeActive}
}
