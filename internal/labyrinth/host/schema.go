package host

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const schemaDDL = `
CREATE SCHEMA IF NOT EXISTS %[1]s;

CREATE TABLE IF NOT EXISTS %[1]s.schema_version (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    version INTEGER NOT NULL CHECK (version > 0)
);

INSERT INTO %[1]s.schema_version(singleton, version)
VALUES (TRUE, 1)
ON CONFLICT (singleton) DO NOTHING;

CREATE TABLE IF NOT EXISTS %[1]s.episodes (
    episode_id TEXT PRIMARY KEY CHECK (episode_id <> '' AND octet_length(episode_id) <= 256),
    scenario_id TEXT NOT NULL CHECK (scenario_id <> '' AND octet_length(scenario_id) <= 256),
    scenario_sha256 TEXT NOT NULL CHECK (scenario_sha256 ~ '^[0-9a-f]{64}$'),
    start_transition BYTEA NOT NULL CHECK (octet_length(start_transition) BETWEEN 2 AND 262144),
    start_transition_sha256 TEXT NOT NULL CHECK (start_transition_sha256 ~ '^[0-9a-f]{64}$'),
    current_number BIGINT NOT NULL CHECK (current_number >= 1),
    current_sha256 TEXT NOT NULL CHECK (current_sha256 ~ '^[0-9a-f]{64}$'),
    terminal BOOLEAN NOT NULL,
    receipt_count BIGINT NOT NULL DEFAULT 0 CHECK (receipt_count BETWEEN 0 AND 10000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS %[1]s.action_receipts (
    episode_id TEXT NOT NULL REFERENCES %[1]s.episodes(episode_id) ON DELETE RESTRICT,
    action_id TEXT NOT NULL CHECK (action_id <> '' AND octet_length(action_id) <= 256),
    receipt_number BIGINT NOT NULL CHECK (receipt_number BETWEEN 1 AND 10000),
    request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    expected_number BIGINT NOT NULL CHECK (expected_number >= 1),
    expected_sha256 TEXT NOT NULL CHECK (expected_sha256 ~ '^[0-9a-f]{64}$'),
    action_json BYTEA NOT NULL CHECK (octet_length(action_json) BETWEEN 2 AND 262144),
    action_json_sha256 TEXT NOT NULL CHECK (action_json_sha256 ~ '^[0-9a-f]{64}$'),
    actor_job_id BIGINT NOT NULL CHECK (actor_job_id > 0),
    actor_generation BIGINT NOT NULL CHECK (actor_generation > 0),
    actor_step_id BIGINT NOT NULL CHECK (actor_step_id > 0),
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt > 0),
    actor_worker_id TEXT NOT NULL CHECK (actor_worker_id <> '' AND octet_length(actor_worker_id) <= 256),
    outcome TEXT NOT NULL CHECK (outcome IN ('transition', 'failure')),
    result_number BIGINT,
    result_sha256 TEXT,
    transition_json BYTEA,
    transition_json_sha256 TEXT,
    failure_json BYTEA,
    failure_json_sha256 TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (episode_id, action_id),
    UNIQUE (episode_id, receipt_number),
    CHECK (
        (outcome = 'transition' AND result_number IS NOT NULL AND result_number >= 2
         AND result_sha256 ~ '^[0-9a-f]{64}$' AND transition_json IS NOT NULL
         AND transition_json_sha256 ~ '^[0-9a-f]{64}$'
         AND failure_json IS NULL AND failure_json_sha256 IS NULL)
        OR
        (outcome = 'failure' AND result_number IS NULL AND result_sha256 IS NULL
         AND transition_json IS NULL AND transition_json_sha256 IS NULL
         AND failure_json IS NOT NULL AND failure_json_sha256 ~ '^[0-9a-f]{64}$')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS labyrinth_host_action_result_revision_uq
ON %[1]s.action_receipts(episode_id, result_number)
WHERE outcome = 'transition';
`

// InstallSchema explicitly installs the benchmark-only durable host schema.
// Runtime methods never auto-create or fall back around a missing schema.
func (store *Store) InstallSchema(ctx context.Context) error {
	if err := store.validate(ctx); err != nil {
		return err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	schema := pgx.Identifier{store.schema}.Sanitize()
	if _, err := tx.Exec(ctx, fmt.Sprintf(schemaDDL, schema)); err != nil {
		return fmt.Errorf("install labyrinth durable host schema: %w", err)
	}
	var version int
	if err := tx.QueryRow(ctx, `SELECT version FROM `+schema+`.schema_version WHERE singleton=TRUE`).Scan(&version); err != nil {
		return fmt.Errorf("read labyrinth durable host schema version: %w", err)
	}
	if version != SchemaVersion {
		return fmt.Errorf("%w: database version %d, runtime version %d", ErrSchemaInvalid, version, SchemaVersion)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit labyrinth durable host schema: %w", err)
	}
	return nil
}
