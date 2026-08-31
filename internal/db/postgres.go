package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const DefaultRuntimeSchema = "omnidex_runtime"

const databaseSetupRuntimeSchemaSlot = "__OMNIDEX_RUNTIME_SCHEMA__"

var runtimeSchemaNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func ValidateRuntimeSchemaName(name string) error {
	if !runtimeSchemaNamePattern.MatchString(name) || name == "public" ||
		name == "information_schema" || len(name) >= 3 && name[:3] == "pg_" {
		return fmt.Errorf("runtime database schema must be one dedicated lowercase identifier")
	}
	return nil
}

// RuntimeSearchPath returns the sole search-path projection used by runtime
// connections. PostgreSQL searches pg_catalog implicitly before this path;
// pg_temp remains an explicit, lower-priority session-local facility.
func RuntimeSearchPath(runtimeSchema string) (string, error) {
	if err := ValidateRuntimeSchemaName(runtimeSchema); err != nil {
		return "", err
	}
	return runtimeSchema + ",pg_temp", nil
}

func ConnectRuntime(
	ctx context.Context,
	databaseURL string,
	runtimeSchema string,
	setupSQL []byte,
) (*pgxpool.Pool, error) {
	if ctx == nil {
		return nil, fmt.Errorf("runtime database connection requires context")
	}
	runtimeSearchPath, err := RuntimeSearchPath(runtimeSchema)
	if err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	_, directSearchPath := cfg.ConnConfig.RuntimeParams["search_path"]
	options := cfg.ConnConfig.RuntimeParams["options"]
	if directSearchPath || databaseOptionsSetSearchPath(options) {
		return nil, fmt.Errorf("DATABASE_URL search_path is forbidden; use DATABASE_SCHEMA")
	}
	configurePool(cfg)
	bootstrap, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		return nil, err
	}
	if err := installRuntimeSchema(
		ctx, bootstrap, runtimeSchema, runtimeSearchPath, setupSQL,
	); err != nil {
		bootstrap.Close()
		return nil, err
	}
	bootstrap.Close()

	cfg.ConnConfig.RuntimeParams["search_path"] = runtimeSearchPath
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

func installRuntimeSchema(
	ctx context.Context,
	pool *pgxpool.Pool,
	runtimeSchema string,
	runtimeSearchPath string,
	setupSQL []byte,
) error {
	body, err := renderRuntimeDatabaseSetup(setupSQL, runtimeSchema)
	if err != nil {
		return err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin runtime schema bootstrap: %w", err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(
		ctx, `SELECT pg_advisory_xact_lock($1)`, runtimeSchemaBootstrapLockKey(runtimeSchema),
	); err != nil {
		return fmt.Errorf("lock runtime schema bootstrap: %w", err)
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
	if exists {
		if _, err := tx.Exec(ctx, `DROP SCHEMA `+
			pgx.Identifier{runtimeSchema}.Sanitize()+` CASCADE`); err != nil {
			return fmt.Errorf("reset dedicated runtime schema: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `CREATE SCHEMA `+
		pgx.Identifier{runtimeSchema}.Sanitize()+` AUTHORIZATION CURRENT_USER`); err != nil {
		return fmt.Errorf("create dedicated runtime schema: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('search_path', $1, true)`, runtimeSearchPath); err != nil {
		return fmt.Errorf("select runtime schema for database setup: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		SET LOCAL standard_conforming_strings TO on;
		SET LOCAL check_function_bodies TO off;
	`); err != nil {
		return fmt.Errorf("set database setup SQL mode: %w", err)
	}
	if _, err := tx.Exec(ctx, string(body)); err != nil {
		return fmt.Errorf("execute authoritative database setup: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit authoritative runtime schema: %w", err)
	}
	return nil
}

func renderRuntimeDatabaseSetup(body []byte, runtimeSchema string) ([]byte, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("database setup bytes are unavailable")
	}
	if !bytes.Contains(body, []byte(databaseSetupRuntimeSchemaSlot)) {
		return nil, fmt.Errorf("database setup lacks its runtime schema slot")
	}
	return bytes.ReplaceAll(
		body,
		[]byte(databaseSetupRuntimeSchemaSlot),
		[]byte(runtimeSchema),
	), nil
}

func runtimeSchemaBootstrapLockKey(runtimeSchema string) int64 {
	digest := sha256.Sum256([]byte("omnidex.runtime-schema-bootstrap.v1\x00" + runtimeSchema))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}

func databaseOptionsSetSearchPath(options string) bool {
	fields := strings.Fields(options)
	for index := 0; index < len(fields); index++ {
		setting := ""
		switch {
		case fields[index] == "-c" && index+1 < len(fields):
			index++
			setting = fields[index]
		case strings.HasPrefix(fields[index], "-c"):
			setting = strings.TrimPrefix(fields[index], "-c")
		}
		name, _, _ := strings.Cut(setting, "=")
		if strings.EqualFold(strings.TrimSpace(name), "search_path") {
			return true
		}
	}
	return false
}
