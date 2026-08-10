package queue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresCognitionNormalizedAndTerminalJSONHaveHardByteCaps(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "json-byte-caps")
	action := prepareCognitionGuardAction(t, fixture, "json-byte-caps")
	oversized := `{"content":"` + strings.Repeat("x", 2*1024*1024+1) + `"}`

	t.Run("observation", func(t *testing.T) {
		expectCognitionJSONCheckViolation(t, fixture, func(tx pgx.Tx) error {
			transitionID := insertUnsealedCognitionByteLimitTransition(t, fixture, action, tx)
			_, err := tx.Exec(fixture.Context, `
				INSERT INTO cognition_transition_observations (
					transition_id,position,observation_id,content_sha256,observation_json
				) VALUES ($1,0,'oversized-observation',$2,$3)
			`, transitionID, cognitionTestDigest("a"), oversized)
			return err
		})
	})

	t.Run("effect", func(t *testing.T) {
		expectCognitionJSONCheckViolation(t, fixture, func(tx pgx.Tx) error {
			transitionID := insertUnsealedCognitionByteLimitTransition(t, fixture, action, tx)
			_, err := tx.Exec(fixture.Context, `
				INSERT INTO cognition_transition_effects (
					transition_id,position,effect_kind,content_sha256,effect_json
				) VALUES ($1,0,'no_change',$2,$3)
			`, transitionID, cognitionTestDigest("b"), oversized)
			return err
		})
	})

	t.Run("terminal completion", func(t *testing.T) {
		trace := `{"schema":"omnidex.cognition-trace-authority.v2","records":[]}`
		var graphSHA string
		if err := fixture.Pool.QueryRow(fixture.Context, `
			SELECT graph_sha256 FROM cognition_obligation_graphs
			WHERE episode_id=$1 ORDER BY graph_version DESC LIMIT 1
		`, fixture.EpisodeID).Scan(&graphSHA); err != nil {
			t.Fatal(err)
		}
		expectCognitionJSONCheckViolation(t, fixture, func(tx pgx.Tx) error {
			_, err := tx.Exec(fixture.Context, `
					INSERT INTO cognition_terminal_seals (
						episode_id,job_id,generation,step_id,final_revision,final_revision_sha256,
						outcome,completion_json,completion_sha256,obligation_graph_sha256,
						ledger_version,working_set_version,trace_json,trace_sha256,
						authority_kind,sealed_attempt,sealed_worker_id,lifecycle_operation_id
					) SELECT $1,$2,$3,$4,1,$5,'failed',$6,encode(digest($6,'sha256'),'hex'),$7,
					         ledgers.version,sets.version,$11,encode(digest($11,'sha256'),'hex'),
					         'worker',$8,$9,NULL
					  FROM task_ledgers ledgers,working_sets sets
					 WHERE ledgers.job_id=$2 AND sets.id=$10
				`, fixture.EpisodeID, fixture.Authority.JobID, fixture.Authority.Generation,
				fixture.Authority.StepID, fixture.Start.Transition.Current.SHA256, oversized,
				graphSHA, fixture.Authority.Attempt, fixture.Authority.WorkerID,
				fixture.WorkingSet, trace)
			return err
		})
	})
}

func insertUnsealedCognitionByteLimitTransition(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	action CognitionActionRecord,
	tx pgx.Tx,
) string {
	t.Helper()
	next, err := cognition.NewWorldRevision(fixture.EpisodeID, 2, cognitionTestDigest("e"))
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision),
		Current: next, Observations: []cognition.Observation{}, Effects: []cognition.Effect{},
	}
	raw, digest, err := cognitionJSON(transition)
	if err != nil {
		t.Fatal(err)
	}
	transitionID := cognitionTransitionID(fixture.EpisodeID, digest)
	if _, err := tx.Exec(fixture.Context, `
		INSERT INTO cognition_transitions (
			transition_id,episode_id,job_id,generation,step_id,revision,previous_revision,
			previous_revision_sha256,current_revision_sha256,action_id,actor_attempt,
			actor_worker_id,transition_json,transition_sha256,cost,terminal,public_outcome
		) VALUES ($1,$2,$3,$4,$5,2,1,$6,$7,$8,$9,$10,$11,$12,0,FALSE,'')
	`, transitionID, fixture.EpisodeID, fixture.Authority.JobID, fixture.Authority.Generation,
		fixture.Authority.StepID, action.ExpectedRevision.SHA256, next.SHA256, action.Action.ID,
		fixture.Authority.Attempt, fixture.Authority.WorkerID, string(raw), digest); err != nil {
		t.Fatal(err)
	}
	return transitionID
}

func expectCognitionJSONCheckViolation(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	operation func(pgx.Tx) error,
) {
	t.Helper()
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	err = operation(tx)
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23514" {
		t.Fatalf("oversized cognition JSON error=%v, want PostgreSQL check violation", err)
	}
}
