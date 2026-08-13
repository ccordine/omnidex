package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestKnownArtifactTruthMigrationIsExactHashGuardedSuccessor(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/085_known_artifact_truth_station.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"BEGIN;", "LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"12e011154fafc79b346688262f9d4aaaa11c4ef9ff4559fa7353ee47a82a8565",
		"d0f39d5a2660eef5585b23873b6e6d644f935913d62c4feffc7c3ed99846b939",
		"observed_language <> 'sql'", "observed_volatility <> 'i'", "NOT observed_strict",
		"WHEN 'known_artifact_truth' THEN station='coding_known_artifact_truth'",
		"WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(",
		"historical opening violates known artifact truth station authority",
		") IS DISTINCT FROM TRUE", ") IS DISTINCT FROM FALSE", "COMMIT;",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("known artifact truth migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"ALTER TABLE", "CREATE TABLE", "DROP TABLE", "DROP FUNCTION", "CASCADE",
		"fallback", "legacy",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("known artifact truth migration contains out-of-scope %q", forbidden)
		}
	}
}

func TestPostgresKnownArtifactTruthStationAuthorityIsExactAndRecursive(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "085"),
	); err != nil {
		t.Fatal(err)
	}
	var direct, wrongStation, crossKind, nested, recursivelyNested, foreignNested, malformed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_known_artifact_truth','known_artifact_truth','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_artifact_handling','known_artifact_truth','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_known_artifact_truth','artifact_handling','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_known_artifact_truth','response_correction',
				'{"original":{"kind":"known_artifact_truth","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_known_artifact_truth','response_correction',
				'{"original":{"kind":"response_correction","payload":{"original":{"kind":"known_artifact_truth","payload":{}}}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_artifact_candidate_selection','response_correction',
				'{"original":{"kind":"known_artifact_truth","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_known_artifact_truth','response_correction','{}'::jsonb
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
}

func TestPostgresKnownArtifactTruthMigrationRejectsChangedPriorFunction(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "084"),
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
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "085"))
	if err == nil || !strings.Contains(err.Error(), "prior station function hash") {
		t.Fatalf("migration error=%v", err)
	}
	assertKnownArtifactTruthMigrationNotInstalled(t, pool)
}

func TestPostgresKnownArtifactTruthMigrationRejectsInvalidHistoricalOpening(t *testing.T) {
	repository, pool, claim := semanticGapTestClaim(t, "historical-known-artifact-truth-authority")
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "084"),
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
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "085"))
	if err == nil || !strings.Contains(
		err.Error(), "historical opening violates known artifact truth station authority",
	) {
		t.Fatalf("migration error=%v", err)
	}
	assertKnownArtifactTruthMigrationNotInstalled(t, pool)
}

func assertKnownArtifactTruthMigrationNotInstalled(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM schema_migrations
			WHERE filename='085_known_artifact_truth_station.sql'
		)
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected known artifact truth migration wrote its ledger entry")
	}
}
