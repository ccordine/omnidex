package queue

import (
	"os"
	"strings"
	"testing"
)

const stationOutputLimitModeMigration = "096_station_output_limit_mode.sql"

func TestStationOutputLimitModeMigrationOwnsExplicitAndNaturalAuthorities(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + stationOutputLimitModeMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE station_gap_openings, station_call_openings IN ACCESS EXCLUSIVE MODE",
		"ADD COLUMN output_limit_mode TEXT NOT NULL DEFAULT 'explicit'",
		"ALTER COLUMN output_limit_mode DROP DEFAULT",
		"station_gap_openings_output_limit_mode_check",
		"station_gap_openings_output_budget_check",
		"station_call_openings_output_limit_mode_check",
		"station_call_openings_output_budget_check",
		"output_limit_mode IN ('explicit','natural')",
		"output_limit_mode='natural' AND max_output_tokens=context_tokens",
		"max_input_tokens=context_tokens AND max_output_tokens=context_tokens",
		"model_input_token_upper_bound=context_tokens",
		"NEW.output_limit_mode",
		"gap.output_limit_mode",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("station output-limit migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"SET output_limit_mode='natural'",
		"max_output_tokens=4096",
		"num_predict",
		"fallback",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Fatalf("station output-limit migration contains forbidden %q", forbidden)
		}
	}
}

func TestStationOutputLimitModeDatabaseConstraintsAreModeSpecific(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	if err := New(pool).EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "096")); err != nil {
		t.Fatal(err)
	}
	for table, names := range map[string][]string{
		"station_gap_openings": {
			"station_gap_openings_output_limit_mode_check",
			"station_gap_openings_output_budget_check",
		},
		"station_call_openings": {
			"station_call_openings_output_limit_mode_check",
			"station_call_openings_output_budget_check",
		},
	} {
		for _, name := range names {
			var definition string
			if err := pool.QueryRow(t.Context(), `
				SELECT pg_get_constraintdef(oid, true)
				FROM pg_constraint
				WHERE conrelid=$1::regclass AND conname=$2
			`, table, name).Scan(&definition); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(definition, "output_limit_mode") ||
				!strings.Contains(definition, "natural") ||
				!strings.Contains(definition, "explicit") {
				t.Fatalf("%s.%s definition=%q lacks both exact modes", table, name, definition)
			}
		}
	}
	var triggerDefinition string
	if err := pool.QueryRow(t.Context(), `
		SELECT pg_get_functiondef('validate_station_call_opening_insert()'::regprocedure)
	`).Scan(&triggerDefinition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(triggerDefinition, "NEW.output_limit_mode") ||
		!strings.Contains(triggerDefinition, "gap.output_limit_mode") {
		t.Fatalf("station call opening trigger does not bind output mode: %s", triggerDefinition)
	}
}
