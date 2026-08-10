package queue

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPostgresCognitionRuntimeJournalMigrationRejectsLegacyActions(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, cognitionRuntimeJournalPreflightSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO cognition_actions DEFAULT VALUES`); err != nil {
		t.Fatal(err)
	}
	migration := readCognitionMigration(t, "044_cognition_runtime_journal.sql")
	if _, err := pool.Exec(ctx, migration); err == nil ||
		!strings.Contains(err.Error(), "reconciliation authority cannot be backfilled") {
		t.Fatalf("migration error=%v, want explicit legacy-action rejection", err)
	}
	assertMigrationRelationExists(t, pool, "cognition_runtime_snapshots", false)
}

func TestPostgresCognitionPolicyCallMigrationRejectsLegacyEvidence(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE cognition_policy_evidence (evidence_id TEXT PRIMARY KEY);
		INSERT INTO cognition_policy_evidence VALUES ('legacy-evidence');
	`); err != nil {
		t.Fatal(err)
	}
	migration := readCognitionMigration(t, "045_cognition_policy_call_journal.sql")
	if _, err := pool.Exec(ctx, migration); err == nil ||
		!strings.Contains(err.Error(), "refuses to discard existing cognition policy evidence") {
		t.Fatalf("migration error=%v, want explicit legacy-evidence rejection", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM cognition_policy_evidence`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("legacy evidence count=%d, want rollback-preserved row", count)
	}
}

func TestCognitionPolicyCallMigrationCreatesReferencedUniqueBeforeForeignKey(t *testing.T) {
	migration := readCognitionMigration(t, "045_cognition_policy_call_journal.sql")
	unique := strings.Index(migration, "ADD CONSTRAINT cognition_policy_calls_exact_actor_unique")
	foreignKey := strings.Index(migration, "ADD CONSTRAINT cognition_actions_policy_call_fk")
	if unique < 0 || foreignKey < 0 || unique > foreignKey {
		t.Fatalf("exact actor unique index=%d action foreign key index=%d", unique, foreignKey)
	}
}

func TestPostgresCognitionTraceSchemaV2MigrationRejectsImmutableLegacySeals(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE cognition_terminal_seals (
			episode_id TEXT PRIMARY KEY,
			trace_json TEXT NOT NULL
		);
		INSERT INTO cognition_terminal_seals VALUES (
			'episode-legacy',
			'{"schema":"omnidex.cognition-trace-authority.v1","records":[]}'
		);
	`); err != nil {
		t.Fatal(err)
	}
	migration := readCognitionMigration(t, "061_cognition_trace_schema_v2.sql")
	if _, err := pool.Exec(ctx, migration); err == nil ||
		!strings.Contains(err.Error(), "cannot deterministically upgrade immutable sealed traces") {
		t.Fatalf("migration error=%v, want explicit immutable trace rejection", err)
	}
	assertMigrationRelationExists(t, pool, "cognition_trace_schema_authority", false)
}

func readCognitionMigration(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../../migrations/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

const cognitionRuntimeJournalPreflightSchema = `
CREATE FUNCTION task_ledger_text_is_exact(TEXT) RETURNS BOOLEAN
LANGUAGE SQL IMMUTABLE AS 'SELECT TRUE';
CREATE FUNCTION digest(TEXT,TEXT) RETURNS BYTEA
LANGUAGE SQL IMMUTABLE AS 'SELECT decode(repeat(''00'',32),''hex'')';
CREATE FUNCTION prevent_cognition_immutable_mutation() RETURNS TRIGGER
LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''immutable''; END';
CREATE TABLE cognition_episodes (
    episode_id TEXT PRIMARY KEY, job_id BIGINT NOT NULL, generation BIGINT NOT NULL,
    UNIQUE (episode_id,job_id,generation)
);
CREATE TABLE cognition_obligations (
    episode_id TEXT NOT NULL, node_id TEXT NOT NULL, PRIMARY KEY (episode_id,node_id)
);
CREATE TABLE job_step_attempts (
    job_id BIGINT, generation BIGINT, step_id BIGINT, attempt BIGINT, worker_id TEXT,
    PRIMARY KEY (job_id,generation,step_id,attempt,worker_id)
);
CREATE TABLE context_projections (
    projection_id TEXT, working_set_id TEXT, job_id BIGINT, generation BIGINT,
    PRIMARY KEY (projection_id,working_set_id,job_id,generation)
);
CREATE TABLE cognition_obligation_graphs (
    command_id TEXT PRIMARY KEY, episode_id TEXT, graph_version BIGINT,
    UNIQUE (episode_id,graph_version)
);
CREATE TABLE cognition_actions (legacy_id BIGSERIAL PRIMARY KEY);
`
