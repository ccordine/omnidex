package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

const responseSchemaAuthorityRetirementMigration = "181_response_schema_authority_retirement.sql"

func TestResponseSchemaAuthorityRetirementMigrationIsExactAndFreshOnly(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + responseSchemaAuthorityRetirementMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE station_gap_openings, llm_call_evidence",
		"DROP COLUMN response_schema",
		"omnidex.render-portable-job.v5",
		"station_gap_openings_projection_v5",
		"projection_envelope::jsonb-'prompt'-'renderer'='{}'::jsonb",
		"ADD CONSTRAINT llm_call_evidence_current_raw_transport CHECK",
		"response_format='text'",
		"CREATE OR REPLACE FUNCTION require_llm_call_station_gap()",
		"requires a fresh reset: immutable inference history exists",
		"43d018602e2004cb252c9fe87e0aa64052b1ea3e2494729279f33936dec524d9",
		"137f98e5c9262e6611a28b2ea2a46a96bdf1ae176b6c896b2ea0078529673c50",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("response-schema retirement migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_gap_openings", "UPDATE llm_call_evidence",
		"DELETE FROM", "TRUNCATE ", "NOT VALID", " CASCADE",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("response-schema retirement migration contains forbidden mutation %q", forbidden)
		}
	}
}

func TestPostgresResponseSchemaAuthorityIsCompletelyRetired(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, responseSchemaAuthorityRetirementMigration, 1)

	var retiredColumnCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name IN ('station_gap_openings','llm_call_evidence')
		  AND column_name='response_schema'
	`).Scan(&retiredColumnCount); err != nil {
		t.Fatal(err)
	}
	if retiredColumnCount != 0 {
		t.Fatalf("current response-schema column count=%d want 0", retiredColumnCount)
	}

	want := map[string]string{
		"station_gap_openings_renderer_version_check": "522c94915fab9d58a737582594a176fddd2c80322ecaf687edeeef34cd421605",
		"station_gap_openings_projection_v5":          "8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad",
		"station_gap_openings_current_raw_transport":  "a147cbfc147e9f009ab9a4f51c0fac68ec42a5a7e44df60a67671955ac0afc7e",
		"llm_call_evidence_current_raw_transport":     "cf2775374afaa1cd7948e9edc383d4ee4b52661e329baf5a7dcef10c59751dd7",
	}
	got := make(map[string]string, len(want))
	rows, err := pool.Query(t.Context(), `
		SELECT conname,
		       encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
		FROM pg_constraint
		WHERE conrelid IN (
			'station_gap_openings'::regclass,
			'llm_call_evidence'::regclass
		) AND conname=ANY($1)
	`, stationGapOpeningMapKeys(want))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, digest string
		if err := rows.Scan(&name, &digest); err != nil {
			t.Fatal(err)
		}
		got[name] = digest
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for name, digest := range want {
		if got[name] != digest {
			t.Errorf("current constraint %q digest=%q want %q", name, got[name], digest)
		}
	}

	var functionDigest, functionSource string
	if err := pool.QueryRow(t.Context(), `
		SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex'),prosrc
		FROM pg_proc
		WHERE oid=to_regprocedure('require_llm_call_station_gap()')
	`).Scan(&functionDigest, &functionSource); err != nil {
		t.Fatal(err)
	}
	if functionDigest != "137f98e5c9262e6611a28b2ea2a46a96bdf1ae176b6c896b2ea0078529673c50" ||
		strings.Contains(functionSource, "response_schema") {
		t.Fatalf("current station evidence trigger digest/source=%q/%q", functionDigest, functionSource)
	}
}

func TestPostgresResponseSchemaRetirementRejectsChangedPriorAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "180"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE station_gap_openings
		DROP CONSTRAINT station_gap_openings_renderer_version_check,
		ADD CONSTRAINT station_gap_openings_renderer_version_check CHECK (TRUE)
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "requires the exact prior constraints") {
		t.Fatalf("changed prior response-schema authority error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, responseSchemaAuthorityRetirementMigration, 0)
}

func TestPostgresResponseSchemaRetirementRejectsImmutableHistory(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "180"),
	); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		t.Context(), "response-schema-retirement-history", model.PipelineCoding, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "response-schema-retirement-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("history claim=%+v err=%v", claim, err)
	}
	opening, err := validateStationGapOpening(stationGapOpenFixture(t, claim.Authority))
	if err != nil {
		t.Fatal(err)
	}
	opening = freezeHistoricalRawStationGapV4(t, opening)
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := insertHistoricalStationGapOpeningTx(t.Context(), tx, &opening); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	err = repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(
		err.Error(), "requires a fresh reset: immutable inference history exists",
	) {
		t.Fatalf("immutable response-schema history error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, responseSchemaAuthorityRetirementMigration, 0)
}
