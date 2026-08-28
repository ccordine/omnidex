package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const rawPortableResponseTransportMigration = "163_raw_portable_response_transport.sql"

func TestRawPortableResponseTransportMigrationIsCurrentOnly(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + rawPortableResponseTransportMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"portable_schema='omnidex.portable-job.v2'",
		"renderer_version='omnidex.render-portable-job.v4'",
		"response_schema='null'",
		"scope='portable_structural_worker'",
		"scope='portable_fragment_worker'",
		"scope='portable_semantic_worker'",
		"protocol='omnidex.ollama-raw-text-generate-request.v1'",
		"response_format='text' AND response_schema IS NULL",
		"portable_schema IS DISTINCT FROM 'omnidex.portable-job.v2'",
		"renderer_version IS DISTINCT FROM 'omnidex.render-portable-job.v4'",
		"response_schema IS DISTINCT FROM 'null'",
		"scope IS DISTINCT FROM CASE",
		"SELECT 1 FROM station_call_openings WHERE protocol IS DISTINCT FROM",
		"SELECT 1 FROM llm_call_evidence WHERE response_format IS DISTINCT FROM 'text' OR response_schema IS NOT NULL",
		"raw portable transport requires a fresh reset: legacy portable transport state exists",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("raw transport migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"omnidex.portable-job.v1",
		"omnidex.render-portable-job.v3",
		"NOT VALID",
		"UPDATE ",
		"DELETE ",
		"TRUNCATE ",
	} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("current-only raw migration contains forbidden authority %q", forbidden)
		}
	}
}

func TestRawPortableResponseTransportMigrationRegistersEveryCurrentKind(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + rawPortableResponseTransportMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, kind := range assemblyline.AllWorkKinds() {
		mapping := "WHEN '" + string(kind) + "' THEN"
		if !strings.Contains(source, mapping) {
			t.Fatalf("raw transport migration omitted current work kind %q", kind)
		}
	}
	for _, retired := range []string{
		"application_context_needs",
		"application_intent",
		"application_job_specification",
		"application_service_state_interface",
		"repository_requirements",
		"repository_change_surface",
		"repository_search_term",
		"repository_evidence_relevance",
		"repository_grounded_review",
		"context_search_terms",
		"context_relevance",
		"roleplay_grounded_response",
		"roleplay_canon_extraction",
		"grounded_answer",
		"database_schema_selection",
		"database_query_intent",
		"web_search_terms",
		"web_relevance",
		"web_grounded_synthesis",
		"web_claim_evidence_review",
	} {
		if strings.Contains(source, "WHEN '"+retired+"' THEN") {
			t.Fatalf("raw transport migration retained aggregate work kind %q", retired)
		}
	}
}

func TestPostgresRawPortableResponseTransportAuthorityIsValidated(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "163"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, rawPortableResponseTransportMigration, 1)

	rows, err := pool.Query(t.Context(), `
		SELECT conname,convalidated,pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE connamespace=(SELECT oid FROM pg_namespace WHERE nspname=current_schema())
		AND conname IN (
			'station_gap_openings_portable_schema_check',
			'station_gap_openings_renderer_version_check',
			'station_gap_openings_current_raw_transport',
			'station_call_openings_current_raw_transport',
			'llm_call_evidence_current_raw_transport'
		)
		ORDER BY conname
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	definitions := make(map[string]string, 5)
	for rows.Next() {
		var name, definition string
		var validated bool
		if err := rows.Scan(&name, &validated, &definition); err != nil {
			t.Fatal(err)
		}
		if !validated {
			t.Fatalf("current raw constraint %q is not validated", name)
		}
		definitions[name] = definition
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 5 {
		t.Fatalf("current raw constraints=%d want 5", len(definitions))
	}
	for name, required := range map[string]string{
		"station_gap_openings_portable_schema_check":  "omnidex.portable-job.v2",
		"station_gap_openings_renderer_version_check": "omnidex.render-portable-job.v4",
		"station_gap_openings_current_raw_transport":  "response_schema",
		"station_call_openings_current_raw_transport": "omnidex.ollama-raw-text-generate-request.v1",
		"llm_call_evidence_current_raw_transport":     "response_schema IS NULL",
	} {
		if !strings.Contains(definitions[name], required) {
			t.Fatalf("constraint %q omitted %q: %s", name, required, definitions[name])
		}
	}

	var currentDirect, currentNested, retiredDirect, retiredNested bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'database_query_intent','database_query_shape','{}'::jsonb
			),
			station_owns_portable_work(
				'database_query_intent','response_correction',
				'{"original":{"kind":"database_query_shape","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'database_query_intent','database_query_intent','{}'::jsonb
			),
			station_owns_portable_work(
				'database_query_intent','response_correction',
				'{"original":{"kind":"database_query_intent","payload":{}}}'::jsonb
			)
	`).Scan(&currentDirect, &currentNested, &retiredDirect, &retiredNested); err != nil {
		t.Fatal(err)
	}
	if !currentDirect || !currentNested || retiredDirect || retiredNested {
		t.Fatalf(
			"station ownership current=%v/%v retired=%v/%v",
			currentDirect, currentNested, retiredDirect, retiredNested,
		)
	}
}
