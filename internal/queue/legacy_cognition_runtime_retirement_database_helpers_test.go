package queue

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func legacyRetirementMigrationLedgerSnapshot(
	t testing.TB,
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
	`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func legacyRetirementMigrationLedgerSnapshotBefore065(
	t testing.TB,
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
		WHERE filename<>'065_legacy_cognition_runtime_retirement.sql'
	`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func assertLegacyCognitionCatalogRetired(
	t testing.TB,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()
	var relations, routines, triggers, actorKeys int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema() AND c.relname LIKE 'cognition\_%' ESCAPE '\'
	`).Scan(&relations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		WITH candidates AS (
			SELECT p.oid,p.proname
			FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
			WHERE n.nspname=current_schema() AND p.prokind IN ('f','p')
		)
		SELECT COUNT(*) FROM candidates
		WHERE proname LIKE '%cognition%'
		   OR proname='require_exact_accepted_decision_recovery'
		   OR pg_get_functiondef(oid) LIKE '%cognition\_%' ESCAPE '\'
	`).Scan(&routines); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_trigger t
		JOIN pg_class c ON c.oid=t.tgrelid
		JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema() AND t.tgname=ANY($1::text[])
	`, legacyCognitionCoreTriggerNames()).Scan(&triggers); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_constraint co
		JOIN pg_class c ON c.oid=co.conrelid
		JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema()
		  AND c.relname='job_step_attempts'
		  AND co.conname='job_step_attempts_exact_actor_unique'
	`).Scan(&actorKeys); err != nil {
		t.Fatal(err)
	}
	if relations != 0 || routines != 0 || triggers != 0 || actorKeys != 0 {
		t.Fatalf("legacy catalog remains relations/routines/triggers/actor_keys=%d/%d/%d/%d",
			relations, routines, triggers, actorKeys)
	}

	var genericAuthority int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_trigger t
		JOIN pg_class c ON c.oid=t.tgrelid
		JOIN pg_namespace n ON n.oid=c.relnamespace
		JOIN pg_proc p ON p.oid=t.tgfoid
		WHERE n.nspname=current_schema()
		  AND c.relname='task_node_generation_supersessions'
		  AND t.tgname='task_node_supersessions_require_event'
		  AND p.proname='require_task_node_supersession_event'
		  AND NOT t.tgisinternal
	`).Scan(&genericAuthority); err != nil {
		t.Fatal(err)
	}
	if genericAuthority != 1 {
		t.Fatalf("generic supersession authority count=%d want 1", genericAuthority)
	}
}

func assertLegacyCognitionCatalogPreservedBefore065(
	t testing.TB,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()
	var relations, routines, triggers, actorKeys int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema() AND c.relkind='r'
		  AND c.relname LIKE 'cognition\_%' ESCAPE '\'
	`).Scan(&relations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		WITH candidates AS (
			SELECT p.oid,p.proname
			FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
			WHERE n.nspname=current_schema() AND p.prokind IN ('f','p')
		)
		SELECT COUNT(*) FROM candidates
		WHERE proname LIKE '%cognition%'
		   OR proname='require_exact_accepted_decision_recovery'
		   OR pg_get_functiondef(oid) LIKE '%cognition\_%' ESCAPE '\'
	`).Scan(&routines); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_trigger t
		JOIN pg_class c ON c.oid=t.tgrelid
		JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema() AND t.tgname=ANY($1::text[])
	`, legacyCognitionCoreTriggerNames()).Scan(&triggers); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_constraint co
		JOIN pg_class c ON c.oid=co.conrelid
		JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema()
		  AND c.relname='job_step_attempts'
		  AND co.conname='job_step_attempts_exact_actor_unique'
	`).Scan(&actorKeys); err != nil {
		t.Fatal(err)
	}
	if relations != 51 || routines != 168 || triggers != 6 || actorKeys != 1 {
		t.Fatalf("rolled-back catalog relations/routines/triggers/actor_keys=%d/%d/%d/%d want 51/168/6/1",
			relations, routines, triggers, actorKeys)
	}

	var contextDefinition string
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_constraintdef(co.oid,true)
		FROM pg_constraint co
		JOIN pg_class c ON c.oid=co.conrelid
		JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema()
		  AND c.relname='context_projections'
		  AND co.conname='context_projections_usage_mode_check'
	`).Scan(&contextDefinition); err != nil {
		t.Fatal(err)
	}
	if contextDefinition != "CHECK (usage_mode = ANY (ARRAY['shadow'::text, 'live'::text]))" {
		t.Fatalf("rolled-back context usage constraint=%q", contextDefinition)
	}
}

func assertLiveOnlyContextProjectionConstraint(
	t testing.TB,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()
	var definition string
	var constraints int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*),MIN(pg_get_constraintdef(co.oid,true))
		FROM pg_constraint co
		JOIN pg_class c ON c.oid=co.conrelid
		JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema()
		  AND c.relname='context_projections'
		  AND co.conname='context_projections_usage_mode_check'
		  AND co.contype='c' AND co.convalidated AND NOT co.connoinherit
	`).Scan(&constraints, &definition); err != nil {
		t.Fatal(err)
	}
	if constraints != 1 || definition != "CHECK (usage_mode = 'live'::text)" {
		t.Fatalf("live-only context constraint count/definition=%d/%q", constraints, definition)
	}
}

func legacyCognitionCoreTriggerNames() []string {
	return []string{
		"job_lifecycle_operations_require_cognition_seals",
		"task_entries_require_cognition_accepted_fact",
		"task_entries_require_cognition_proposal_disposition",
		"task_entries_require_cognition_proposal_materialization",
		"task_entries_require_cognition_selected_decision",
		"task_events_require_cognition_belief_revision",
	}
}
