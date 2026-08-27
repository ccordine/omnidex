package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const conversationObjectiveCutoverMigration = "066_conversation_objective_cutover.sql"

func installConversationSchemaThrough065(
	t *testing.T,
) (context.Context, *pgxpool.Pool, *Repository) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "065")); err != nil {
		t.Fatal(err)
	}
	return ctx, pool, repository
}

func applyConversationCutover(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
) error {
	t.Helper()
	return repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "066"))
}

func enqueueConversationCutoverJob(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	label string,
) (int64, int64) {
	t.Helper()
	fixture := seedPreInlineExecutionMigrationJob(
		t, ctx, repository.pool, label, "chat", "objective_resolve", nil,
	)
	return fixture.Job.ID, fixture.Step.ID
}

func seedAcceptedIntentProjection(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	jobID, stepID int64,
) int64 {
	t.Helper()
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	const objectiveID = "legacy-accepted-intent-objective"
	var artifactID, ledgerVersion int64
	var payloadSHA, ledgerID string
	if err := tx.QueryRow(ctx, `
		UPDATE task_ledgers SET version=version+1,updated_at=NOW()
		WHERE job_id=$1 RETURNING id,version
	`, jobID).Scan(&ledgerID, &ledgerVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_nodes (
			ledger_id,job_id,id,kind,title,status,priority,created_by,created_step_id,
			acceptance_criteria,metadata,created_version,updated_version
		) VALUES (
			$1,$2,$3,'objective','Legacy accepted intent objective','pending',50,'code',$4,
			'[]'::jsonb,'{}'::jsonb,$5,$5
		)
	`, ledgerID, jobID, objectiveID, stepID, ledgerVersion); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO artifacts (job_id,step_id,kind,version,payload_json)
		VALUES ($1,$2,'intent','1',jsonb_build_object('objective','legacy'))
		RETURNING id, encode(digest(payload_json::text,'sha256'),'hex')
	`, jobID, stepID).Scan(&artifactID, &payloadSHA); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_artifact_projections (
			artifact_id,job_id,step_id,job_generation,ledger_id,artifact_kind,
			artifact_version,projection_schema,payload_sha256,objective_node_id,
			ledger_start_version,ledger_end_version
		) VALUES ($1,$2,$3,1,$4,'intent','1',
			'omnidex.accepted-intent-projection.v1',$5,$6,$7,$8)
	`, artifactID, jobID, stepID, ledgerID, payloadSHA, objectiveID,
		ledgerVersion-1, ledgerVersion); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return artifactID
}

func seedLegacyPlanningGeneration(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	jobID int64,
) {
	t.Helper()
	const feedback = "Retain exact legacy planning history."
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (
			job_id,generation,purpose,predecessor_generation,boundary_action,
			feedback,feedback_sha256
		) VALUES ($1,2,'replan',1,'v3_planning',$2,
			encode(digest($2,'sha256'),'hex'))
	`, jobID, feedback); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET current_generation=2 WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func assertConversationCutoverRejectedAtomically(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()
	assertAppliedMigrationCount(t, pool, conversationObjectiveCutoverMigration, 0)
	var columns, relations, retiredFunctions, oldConstraint, newConstraint int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='ai_channels'
		  AND column_name IN ('persona','system','provider','model','context')
	`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema()
		  AND c.relname IN ('task_artifact_projections','task_artifact_projection_items')
	`).Scan(&relations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
		WHERE n.nspname=current_schema() AND p.proname=ANY($1::text[])
	`, []string{
		"validate_task_artifact_projection", "validate_task_artifact_projection_item",
		"prevent_task_artifact_projection_mutation", "prevent_projected_artifact_mutation",
		"require_intent_artifact_projection",
	}).Scan(&retiredFunctions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE conname='job_generations_check'),
		       COUNT(*) FILTER (WHERE conname='job_generations_authoritative_shape')
		FROM pg_constraint WHERE conrelid='job_generations'::regclass
	`).Scan(&oldConstraint, &newConstraint); err != nil {
		t.Fatal(err)
	}
	if columns != 5 || relations != 2 || retiredFunctions != 5 || oldConstraint != 1 || newConstraint != 0 {
		t.Fatalf("rejected 066 changed catalog columns/relations/functions/constraints=%d/%d/%d/%d/%d",
			columns, relations, retiredFunctions, oldConstraint, newConstraint)
	}
}

func assertPostgresFailureContains(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	statement, want string,
	arguments ...any,
) {
	t.Helper()
	const savepoint = "conversation_cutover_expected_failure"
	if _, err := tx.Exec(ctx, "SAVEPOINT "+savepoint); err != nil {
		t.Fatal(err)
	}
	_, operationErr := tx.Exec(ctx, statement, arguments...)
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+savepoint); err != nil {
		t.Fatalf("recover expected PostgreSQL failure: %v (operation error: %v)", err, operationErr)
	}
	if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		t.Fatal(err)
	}
	if operationErr == nil || !strings.Contains(operationErr.Error(), want) {
		t.Fatalf("PostgreSQL failure=%v want containing %q for %s", operationErr, want, fmt.Sprint(statement))
	}
}

func conversationCutoverLedgerSnapshot(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) string {
	t.Helper()
	var snapshot string
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(jsonb_agg(
			jsonb_build_array(filename,sha256,applied_at) ORDER BY filename
		)::text,'[]')
		FROM schema_migrations
		WHERE filename<>$1
	`, conversationObjectiveCutoverMigration).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}
