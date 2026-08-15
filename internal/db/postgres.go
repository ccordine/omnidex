package db

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const DefaultRuntimeSchema = "omnidex_runtime"

const runtimeSchemaBootstrapLockID int64 = 0x4f4d4e4952545343

var runtimeSchemaNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func ValidateRuntimeSchemaName(name string) error {
	if !runtimeSchemaNamePattern.MatchString(name) || name == "public" ||
		name == "information_schema" || len(name) >= 3 && name[:3] == "pg_" {
		return fmt.Errorf("runtime database schema must be one dedicated lowercase identifier")
	}
	return nil
}

func ConnectRuntime(
	ctx context.Context,
	databaseURL string,
	runtimeSchema string,
) (*pgxpool.Pool, error) {
	if ctx == nil {
		return nil, fmt.Errorf("runtime database connection requires context")
	}
	if err := ValidateRuntimeSchemaName(runtimeSchema); err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	_, directSearchPath := cfg.ConnConfig.RuntimeParams["search_path"]
	options := cfg.ConnConfig.RuntimeParams["options"]
	if directSearchPath || strings.Contains(strings.ToLower(options), "search_path") {
		return nil, fmt.Errorf("DATABASE_URL search_path is forbidden; use DATABASE_SCHEMA")
	}
	configurePool(cfg)
	bootstrap, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		return nil, err
	}
	if err := bootstrapRuntimeSchema(ctx, bootstrap, runtimeSchema); err != nil {
		bootstrap.Close()
		return nil, err
	}
	bootstrap.Close()

	cfg.ConnConfig.RuntimeParams["search_path"] = runtimeSchema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var selected string
	if err := pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&selected); err != nil {
		pool.Close()
		return nil, err
	}
	if selected != runtimeSchema {
		pool.Close()
		return nil, fmt.Errorf("runtime database did not select its exact schema")
	}
	return pool, nil
}

// ConnectRuntimeReadOnly selects an existing dedicated runtime schema without
// bootstrap, migration, or schema mutation. It is for immutable evidence
// inspection such as station replay; callers must still use read-only
// transactions for every query.
func ConnectRuntimeReadOnly(
	ctx context.Context,
	databaseURL string,
	runtimeSchema string,
) (*pgxpool.Pool, error) {
	if ctx == nil {
		return nil, fmt.Errorf("read-only runtime database connection requires context")
	}
	if err := ValidateRuntimeSchemaName(runtimeSchema); err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	_, directSearchPath := cfg.ConnConfig.RuntimeParams["search_path"]
	options := cfg.ConnConfig.RuntimeParams["options"]
	if directSearchPath || strings.Contains(strings.ToLower(options), "search_path") {
		return nil, fmt.Errorf("DATABASE_URL search_path is forbidden; use DATABASE_SCHEMA")
	}
	configurePool(cfg)
	cfg.ConnConfig.RuntimeParams["search_path"] = runtimeSchema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var selected string
	if err := pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&selected); err != nil {
		pool.Close()
		return nil, err
	}
	if selected != runtimeSchema {
		pool.Close()
		return nil, fmt.Errorf("runtime database did not select its exact schema")
	}
	return pool, nil
}

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	configurePool(cfg)

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func configurePool(cfg *pgxpool.Config) {
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
}

func bootstrapRuntimeSchema(
	ctx context.Context,
	pool *pgxpool.Pool,
	runtimeSchema string,
) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin runtime schema bootstrap: %w", err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, runtimeSchemaBootstrapLockID); err != nil {
		return fmt.Errorf("lock runtime schema bootstrap: %w", err)
	}
	legacy, err := publicOmnidexSchemaExists(ctx, tx)
	if err != nil {
		return err
	}
	if err := rejectPublicOmnidexState(legacy); err != nil {
		return err
	}

	var exists, owned bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname=$1),
		       EXISTS (
			   SELECT 1 FROM pg_namespace
			   WHERE nspname=$1 AND nspowner=(SELECT oid FROM pg_roles WHERE rolname=current_user)
		       )
	`, runtimeSchema).Scan(&exists, &owned); err != nil {
		return fmt.Errorf("inspect runtime schema: %w", err)
	}
	if exists && !owned {
		return fmt.Errorf("runtime schema %q is not owned by the database user", runtimeSchema)
	}
	if !exists {
		if _, err := tx.Exec(ctx, `CREATE SCHEMA `+
			pgx.Identifier{runtimeSchema}.Sanitize()+` AUTHORIZATION CURRENT_USER`); err != nil {
			return fmt.Errorf("create dedicated runtime schema: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit runtime schema bootstrap: %w", err)
	}
	return nil
}

func rejectPublicOmnidexState(legacy bool) error {
	if legacy {
		return fmt.Errorf("legacy Omnidex state exists in public; explicit migration is required")
	}
	return nil
}

func publicOmnidexSchemaExists(ctx context.Context, tx pgx.Tx) (bool, error) {
	var legacy bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_class relations
			JOIN pg_namespace namespaces ON namespaces.oid=relations.relnamespace
			WHERE namespaces.nspname='public' AND relations.relkind IN ('r','p') AND (
				relations.relname IN (
					'schema_migrations','omni_migrations','omni_runs','task_ledgers',
					'memory_chunks','memory_chunk_tags'
				) OR
				relations.relname='jobs' AND EXISTS (
					SELECT 1 FROM pg_class steps
					JOIN pg_namespace step_namespaces ON step_namespaces.oid=steps.relnamespace
					WHERE step_namespaces.nspname='public' AND steps.relname='job_steps' AND
					      steps.relkind IN ('r','p')
				)
			)
		)
	`).Scan(&legacy); err != nil {
		return false, fmt.Errorf("inspect legacy public Omnidex state: %w", err)
	}
	return legacy, nil
}
