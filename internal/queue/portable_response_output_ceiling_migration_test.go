package queue

import (
	"os"
	"strings"
	"testing"
)

const portableResponseOutputCeilingMigration = "165_portable_response_output_ceiling.sql"

func TestPortableResponseOutputCeilingMigrationRequiresFreshCurrentState(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + portableResponseOutputCeilingMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings, station_call_openings IN ACCESS EXCLUSIVE MODE",
		"IF EXISTS (SELECT 1 FROM station_gap_openings) OR EXISTS (SELECT 1 FROM station_call_openings)",
		"requires fresh station gap and call state",
		"DROP CONSTRAINT station_gap_openings_output_budget_check",
		"DROP CONSTRAINT station_call_openings_output_budget_check",
		"ADD CONSTRAINT station_gap_openings_output_budget_check",
		"ADD CONSTRAINT station_call_openings_output_budget_check",
		"output_limit_mode='natural' AND max_output_tokens BETWEEN 1 AND context_tokens",
		"max_input_tokens=context_tokens",
		"model_input_token_upper_bound=context_tokens",
		"pg_get_constraintdef(oid),convalidated",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("portable response output-ceiling migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"NOT VALID", "UPDATE ", "DELETE ", "TRUNCATE ",
		"SET max_output_tokens", "max_output_tokens=context_tokens",
	} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("portable response output-ceiling migration contains forbidden authority %q", forbidden)
		}
	}
}

func TestPostgresPortableResponseOutputCeilingConstraintsAreValidated(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	if err := New(pool).EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "165"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, portableResponseOutputCeilingMigration, 1)

	for table, name := range map[string]string{
		"station_gap_openings":  "station_gap_openings_output_budget_check",
		"station_call_openings": "station_call_openings_output_budget_check",
	} {
		var definition string
		var validated bool
		if err := pool.QueryRow(t.Context(), `
			SELECT pg_get_constraintdef(oid),convalidated
			FROM pg_constraint
			WHERE conrelid=$1::regclass AND conname=$2 AND contype='c'
		`, table, name).Scan(&definition, &validated); err != nil {
			t.Fatal(err)
		}
		if !validated || !strings.Contains(definition, "max_output_tokens >= 1") ||
			!strings.Contains(definition, "max_output_tokens <= context_tokens") ||
			strings.Contains(definition, "max_output_tokens = context_tokens") {
			t.Fatalf("%s.%s is not the validated bounded natural ceiling: %s", table, name, definition)
		}
	}
}
