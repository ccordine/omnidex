package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestArtifactCandidateSelectionMigrationIsExactHashGuardedSuccessor(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/084_artifact_candidate_selection_station.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"BEGIN;", "LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"b10183c38f3a39706076e2fb244605f28fd3ec117852003d707d833f8a36e8c9",
		"12e011154fafc79b346688262f9d4aaaa11c4ef9ff4559fa7353ee47a82a8565",
		"observed_language <> 'sql'", "observed_volatility <> 'i'", "NOT observed_strict",
		"WHEN 'artifact_candidate_selection' THEN",
		"station='coding_artifact_candidate_selection'",
		"WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(",
		"historical opening violates artifact candidate selection station authority",
		") IS DISTINCT FROM TRUE", ") IS DISTINCT FROM FALSE", "COMMIT;",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("artifact candidate selection migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"ALTER TABLE", "CREATE TABLE", "DROP TABLE", "DROP FUNCTION", "CASCADE",
		"fallback", "legacy",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("artifact candidate selection migration contains out-of-scope %q", forbidden)
		}
	}
}

func TestPostgresArtifactCandidateSelectionStationAuthorityIsExactAndRecursive(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "084"),
	); err != nil {
		t.Fatal(err)
	}
	var direct, wrongStation, crossKind, nested, recursivelyNested, foreignNested, malformed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_artifact_candidate_selection','artifact_candidate_selection','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_artifact_handling','artifact_candidate_selection','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_artifact_candidate_selection','artifact_handling','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_artifact_candidate_selection','response_correction',
				'{"original":{"kind":"artifact_candidate_selection","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_artifact_candidate_selection','response_correction',
				'{"original":{"kind":"response_correction","payload":{"original":{"kind":"artifact_candidate_selection","payload":{}}}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_declaration_artifact_boundary','response_correction',
				'{"original":{"kind":"artifact_candidate_selection","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_artifact_candidate_selection','response_correction','{}'::jsonb
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

func TestPostgresArtifactCandidateSelectionMigrationRejectsChangedPriorFunction(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "083"),
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
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "084"))
	if err == nil || !strings.Contains(err.Error(), "prior station function hash") {
		t.Fatalf("migration error=%v", err)
	}
	assertArtifactCandidateSelectionMigrationNotInstalled(t, pool)
}

func TestPostgresArtifactCandidateSelectionMigrationRejectsInvalidHistoricalOpening(t *testing.T) {
	repository, pool, claim := semanticGapTestClaim(t, "historical-candidate-selection-authority")
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "083"),
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
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "084"))
	if err == nil || !strings.Contains(
		err.Error(), "historical opening violates artifact candidate selection station authority",
	) {
		t.Fatalf("migration error=%v", err)
	}
	assertArtifactCandidateSelectionMigrationNotInstalled(t, pool)
}

func assertArtifactCandidateSelectionMigrationNotInstalled(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM schema_migrations
			WHERE filename='084_artifact_candidate_selection_station.sql'
		)
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected artifact candidate selection migration wrote its ledger entry")
	}
}
