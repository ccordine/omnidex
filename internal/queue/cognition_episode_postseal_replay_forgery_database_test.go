package queue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type cognitionPostSealReplayForgeryFixture struct {
	pool      *pgxpool.Pool
	ctx       context.Context
	episodeID cognition.EpisodeID
	actor     model.StepAttemptAuthority
}

func TestPostgresPostSealReplayBootstrapRejectsCoherentObservationForgeries(t *testing.T) {
	tests := []struct {
		name     string
		mutation string
	}{
		{
			name: "semantic SHA differs from raw observation",
			mutation: `WITH changed AS (
				SELECT saved.*,repeat('f',64) AS changed_observation_sha
				FROM saved_postseal_replay_audit saved
			), authority AS (
				SELECT changed.*,cognition_canonical_jsonb(jsonb_set(
					changed.authority_json::jsonb,'{observation_sha256}',
					to_jsonb(changed.changed_observation_sha),TRUE
				)) AS changed_authority FROM changed
			)
			SELECT 'cognition_postseal_replay_bootstrap_'||
				encode(digest(changed_authority,'sha256'),'hex'),episode_id,evidence_id,
				job_id,generation,step_id,step_attempt,worker_id,provider_observation_json,
				provider_observation_json_sha256,changed_observation_sha,observed_at,
				terminal_trace_sha256,process_observation_id,process_chain_sha256,
				changed_authority,encode(digest(changed_authority,'sha256'),'hex'),created_at
			FROM authority`,
		},
		{
			name: "raw observation JSON is noncanonical",
			mutation: `SELECT audit_id,episode_id,evidence_id,job_id,generation,step_id,
				step_attempt,worker_id,' '||provider_observation_json,
				encode(digest(' '||provider_observation_json,'sha256'),'hex'),
				provider_observation_sha256,observed_at,terminal_trace_sha256,
				process_observation_id,process_chain_sha256,authority_json,authority_sha256,
				created_at FROM saved_postseal_replay_audit`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCognitionPostSealReplayForgeryFixture(t)
			tx := beginPostSealReplayForgeryTx(t, fixture, false)
			defer tx.Rollback(fixture.ctx)
			insert := `INSERT INTO cognition_episode_postseal_replay_bootstrap_audits (
				audit_id,episode_id,evidence_id,job_id,generation,step_id,step_attempt,worker_id,
				provider_observation_json,provider_observation_json_sha256,
				provider_observation_sha256,observed_at,terminal_trace_sha256,
				process_observation_id,process_chain_sha256,authority_json,authority_sha256,created_at
			) ` + test.mutation
			if _, err := tx.Exec(fixture.ctx, insert); err != nil {
				t.Fatal(err)
			}
			_, err := tx.Exec(fixture.ctx,
				"SET CONSTRAINTS cognition_postseal_replay_bootstrap_audits_exact IMMEDIATE",
			)
			if err == nil || !strings.Contains(err.Error(), "terminal invocation authority") {
				t.Fatalf("coherent post-seal bootstrap forgery error=%v", err)
			}
		})
	}
}

func TestPostgresPostSealReplayObservationsCannotPredateClaimedAttempt(t *testing.T) {
	for _, boundary := range []string{"bootstrap audit", "process observation"} {
		t.Run(boundary, func(t *testing.T) {
			fixture := newCognitionPostSealReplayForgeryFixture(t)
			replaceProcess := boundary == "process observation"
			tx := beginPostSealReplayForgeryTx(t, fixture, replaceProcess)
			defer tx.Rollback(fixture.ctx)
			advancePostSealReplayAttemptClaim(t, tx, fixture, replaceProcess)
			if replaceProcess {
				insertSavedPostSealProcessObservation(t, tx, fixture)
			}
			insertSavedPostSealReplayAudit(t, tx, fixture)
			constraint := "cognition_postseal_replay_bootstrap_audits_exact"
			want := "terminal invocation authority"
			if replaceProcess {
				constraint = "cognition_provider_postseal_observations_exact"
				want = "live actor authority"
			}
			_, err := tx.Exec(fixture.ctx, "SET CONSTRAINTS "+constraint+" IMMEDIATE")
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s accepted observation before attempt claim: %v", boundary, err)
			}
		})
	}
}

func newCognitionPostSealReplayForgeryFixture(
	t *testing.T,
) cognitionPostSealReplayForgeryFixture {
	t.Helper()
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "postseal-replay-forgery",
	)
	if _, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	); err != nil {
		t.Fatal(err)
	}
	actor := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	replay := cognitionReplayStartForTest(t, fixture.Start, actor)
	if _, err := repository.StartCognitionEpisode(
		ctx, replay, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	return cognitionPostSealReplayForgeryFixture{
		pool: pool, ctx: ctx, episodeID: fixture.EpisodeID, actor: actor,
	}
}

func beginPostSealReplayForgeryTx(
	t *testing.T, fixture cognitionPostSealReplayForgeryFixture, includeProcess bool,
) pgx.Tx {
	t.Helper()
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.ctx, `CREATE TEMP TABLE saved_postseal_replay_audit
		ON COMMIT DROP AS SELECT * FROM cognition_episode_postseal_replay_bootstrap_audits
		WHERE episode_id=$1 AND step_attempt=$2`, fixture.episodeID, fixture.actor.Attempt); err != nil {
		t.Fatal(err)
	}
	if includeProcess {
		if _, err := tx.Exec(fixture.ctx, `CREATE TEMP TABLE saved_postseal_replay_process
			ON COMMIT DROP AS SELECT * FROM cognition_provider_postseal_observations
			WHERE episode_id=$1 AND step_attempt=$2 AND source_kind='episode_replay'`,
			fixture.episodeID, fixture.actor.Attempt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(fixture.ctx, `ALTER TABLE cognition_episode_postseal_replay_bootstrap_audits
		DISABLE TRIGGER cognition_postseal_replay_bootstrap_audits_immutable`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.ctx, `DELETE FROM cognition_episode_postseal_replay_bootstrap_audits
		WHERE episode_id=$1 AND step_attempt=$2`, fixture.episodeID, fixture.actor.Attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.ctx, `ALTER TABLE cognition_episode_postseal_replay_bootstrap_audits
		ENABLE TRIGGER cognition_postseal_replay_bootstrap_audits_immutable`); err != nil {
		t.Fatal(err)
	}
	if includeProcess {
		if _, err := tx.Exec(fixture.ctx, `ALTER TABLE cognition_provider_postseal_observations
			DISABLE TRIGGER cognition_provider_postseal_observations_immutable`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(fixture.ctx, `DELETE FROM cognition_provider_postseal_observations
			WHERE episode_id=$1 AND step_attempt=$2 AND source_kind='episode_replay'`,
			fixture.episodeID, fixture.actor.Attempt); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(fixture.ctx, `ALTER TABLE cognition_provider_postseal_observations
			ENABLE TRIGGER cognition_provider_postseal_observations_immutable`); err != nil {
			t.Fatal(err)
		}
	}
	return tx
}

func advancePostSealReplayAttemptClaim(
	t *testing.T, tx pgx.Tx, fixture cognitionPostSealReplayForgeryFixture,
	useProcessObservation bool,
) {
	t.Helper()
	if _, err := tx.Exec(fixture.ctx, `ALTER TABLE job_step_attempts
		DISABLE TRIGGER job_step_attempt_change_validate`); err != nil {
		t.Fatal(err)
	}
	source := `SELECT observed_at,job_id,generation,step_id,step_attempt,worker_id
		FROM saved_postseal_replay_audit`
	if useProcessObservation {
		source = `SELECT observed_at,job_id,generation,step_id,step_attempt,worker_id
			FROM saved_postseal_replay_process`
	}
	if _, err := tx.Exec(fixture.ctx, `UPDATE job_step_attempts attempts
		SET claimed_at=saved.observed_at+INTERVAL '1 microsecond',
			renewed_at=saved.observed_at+INTERVAL '1 microsecond',
			expires_at=saved.observed_at+INTERVAL '75 seconds 1 microsecond'
		FROM (`+source+`) saved
		WHERE attempts.job_id=saved.job_id AND attempts.generation=saved.generation
		  AND attempts.step_id=saved.step_id AND attempts.attempt=saved.step_attempt
		  AND attempts.worker_id=saved.worker_id`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.ctx, `ALTER TABLE job_step_attempts
		ENABLE TRIGGER job_step_attempt_change_validate`); err != nil {
		t.Fatal(err)
	}
}

func insertSavedPostSealProcessObservation(
	t *testing.T, tx pgx.Tx, fixture cognitionPostSealReplayForgeryFixture,
) {
	t.Helper()
	_, err := tx.Exec(fixture.ctx, `INSERT INTO cognition_provider_postseal_observations (
		observation_id,evidence_id,episode_id,job_id,generation,step_id,step_attempt,
		worker_id,purpose,sequence,source_kind,terminal_trace_sha256,previous_chain_sha256,
		chain_sha256,stable_brain_json,stable_brain_json_sha256,stable_brain_sha256,
		provider_attestation_sha256,provider_observation_json,provider_observation_json_sha256,
		provider_observation_sha256,observed_at,challenge_sha256,receipt_json,receipt_sha256,
		created_at
	) SELECT observation_id,evidence_id,episode_id,job_id,generation,step_id,step_attempt,
		worker_id,purpose,sequence,source_kind,terminal_trace_sha256,previous_chain_sha256,
		chain_sha256,stable_brain_json,stable_brain_json_sha256,stable_brain_sha256,
		provider_attestation_sha256,provider_observation_json,provider_observation_json_sha256,
		provider_observation_sha256,observed_at,challenge_sha256,receipt_json,receipt_sha256,
		created_at FROM saved_postseal_replay_process`)
	if err != nil {
		t.Fatal(err)
	}
}

func insertSavedPostSealReplayAudit(
	t *testing.T, tx pgx.Tx, fixture cognitionPostSealReplayForgeryFixture,
) {
	t.Helper()
	_, err := tx.Exec(fixture.ctx, `INSERT INTO cognition_episode_postseal_replay_bootstrap_audits (
		audit_id,episode_id,evidence_id,job_id,generation,step_id,step_attempt,worker_id,
		provider_observation_json,provider_observation_json_sha256,
		provider_observation_sha256,observed_at,terminal_trace_sha256,
		process_observation_id,process_chain_sha256,authority_json,authority_sha256,created_at
	) SELECT audit_id,episode_id,evidence_id,job_id,generation,step_id,step_attempt,worker_id,
		provider_observation_json,provider_observation_json_sha256,
		provider_observation_sha256,observed_at,terminal_trace_sha256,
		process_observation_id,process_chain_sha256,authority_json,authority_sha256,created_at
	FROM saved_postseal_replay_audit`)
	if err != nil {
		t.Fatal(err)
	}
}
