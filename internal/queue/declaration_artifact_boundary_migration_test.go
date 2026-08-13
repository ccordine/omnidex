package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDeclarationArtifactBoundaryMigrationIsExactHashGuardedSuccessor(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/083_declaration_artifact_boundary_station.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"BEGIN;",
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"c0fd460253bc36461089b326b45cf1ef2828c0a3c7063b106c231d5f0145196d",
		"b10183c38f3a39706076e2fb244605f28fd3ec117852003d707d833f8a36e8c9",
		"observed_language <> 'sql'",
		"observed_volatility <> 'i'",
		"NOT observed_strict",
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
		"WHEN 'declaration_artifact_boundary' THEN",
		"station='coding_declaration_artifact_boundary'",
		"WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(",
		"historical opening violates declaration artifact boundary station authority",
		") IS DISTINCT FROM TRUE",
		") IS DISTINCT FROM FALSE",
		"COMMIT;",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("declaration artifact boundary migration omitted %q", required)
		}
	}
	functionReplacement := strings.Index(source,
		"CREATE OR REPLACE FUNCTION station_owns_portable_work")
	historicalValidation := strings.Index(source, "WHERE station_owns_portable_work(")
	if functionReplacement < 0 || historicalValidation < functionReplacement {
		t.Fatal("historical openings were not checked against successor station authority")
	}
	for _, forbidden := range []string{
		"ALTER TABLE", "CREATE TABLE", "DROP TABLE", "DROP FUNCTION", "CASCADE",
		"fallback", "legacy",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("declaration artifact boundary migration contains out-of-scope %q", forbidden)
		}
	}
}

func TestPostgresDeclarationArtifactBoundaryStationAuthorityIsExactAndRecursive(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "083"),
	); err != nil {
		t.Fatal(err)
	}

	var direct, wrongStation, crossKind, nested, recursivelyNested, foreignNested, malformed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_declaration_artifact_boundary',
				'declaration_artifact_boundary','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_artifact_handling',
				'declaration_artifact_boundary','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_declaration_artifact_boundary',
				'artifact_handling','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_declaration_artifact_boundary','response_correction',
				'{"original":{"kind":"declaration_artifact_boundary","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_declaration_artifact_boundary','response_correction',
				'{"original":{"kind":"response_correction","payload":{"original":{"kind":"declaration_artifact_boundary","payload":{}}}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_artifact_handling','response_correction',
				'{"original":{"kind":"declaration_artifact_boundary","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_declaration_artifact_boundary','response_correction','{}'::jsonb
			)
	`).Scan(
		&direct, &wrongStation, &crossKind, &nested,
		&recursivelyNested, &foreignNested, &malformed,
	); err != nil {
		t.Fatal(err)
	}
	if !direct || wrongStation || crossKind || !nested || !recursivelyNested ||
		foreignNested || malformed {
		t.Fatalf(
			"station authority direct/wrong/cross/nested/recursive/foreign/malformed=%t/%t/%t/%t/%t/%t/%t",
			direct, wrongStation, crossKind, nested, recursivelyNested, foreignNested, malformed,
		)
	}

	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM schema_migrations
			WHERE filename='083_declaration_artifact_boundary_station.sql'
		)
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("declaration artifact boundary migration omitted its ledger entry")
	}
}

func TestPostgresDeclarationArtifactBoundaryMigrationRejectsChangedPriorFunction(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "082"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION station_owns_portable_work(
			station TEXT, work_kind TEXT, payload JSONB
		)
		RETURNS BOOLEAN AS 'SELECT FALSE' LANGUAGE SQL IMMUTABLE STRICT
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "083"))
	if err == nil || !strings.Contains(err.Error(), "prior station function hash") {
		t.Fatalf("migration error=%v", err)
	}
	assertDeclarationArtifactBoundaryMigrationNotInstalled(t, pool)
}

func TestPostgresDeclarationArtifactBoundaryMigrationRejectsInvalidHistoricalOpening(t *testing.T) {
	repository, pool, claim := semanticGapTestClaim(t, "historical-boundary-authority")
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "082"),
	); err != nil {
		t.Fatal(err)
	}

	var priorDefinition string
	if err := pool.QueryRow(t.Context(), `
		SELECT pg_get_functiondef(
			'station_owns_portable_work(text,text,jsonb)'::regprocedure
		)
	`).Scan(&priorDefinition); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION station_owns_portable_work(
			station TEXT, work_kind TEXT, payload JSONB
		)
		RETURNS BOOLEAN AS 'SELECT TRUE' LANGUAGE SQL IMMUTABLE STRICT
	`); err != nil {
		t.Fatal(err)
	}
	insertHistoricalProcedureOpening(
		t, pool, claim.Authority, "skill_procedure", []byte(`{}`),
	)
	if _, err := pool.Exec(t.Context(), priorDefinition); err != nil {
		t.Fatal(err)
	}

	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "083"))
	if err == nil || !strings.Contains(err.Error(),
		"historical opening violates declaration artifact boundary station authority") {
		t.Fatalf("migration error=%v", err)
	}
	assertDeclarationArtifactBoundaryMigrationNotInstalled(t, pool)
}

func assertDeclarationArtifactBoundaryMigrationNotInstalled(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM schema_migrations
			WHERE filename='083_declaration_artifact_boundary_station.sql'
		)
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected declaration artifact boundary migration wrote its ledger entry")
	}
}
