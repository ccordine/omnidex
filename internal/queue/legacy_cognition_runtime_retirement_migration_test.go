package queue

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const legacyCognitionRetirementMigration = "../../migrations/065_legacy_cognition_runtime_retirement.sql"

func TestLegacyCognitionRuntimeRetirementMigrationFreezesExactCutover(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(legacyCognitionRetirementMigration)
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)

	dropOrder := readLegacyRetirementFixture(t, "legacy_cognition_drop_order.txt")
	wantTables := append([]string(nil), dropOrder...)
	sort.Strings(wantTables)
	if got := extractLegacyRetirementArray(t, schema, "expected_tables"); !equalStrings(got, wantTables) {
		t.Fatalf("legacy table inventory differs:\n got %v\nwant %v", got, wantTables)
	}
	if got := extractLegacyRetirementArray(t, schema, "drop_order"); !equalStrings(got, dropOrder) {
		t.Fatalf("legacy table drop order differs:\n got %v\nwant %v", got, dropOrder)
	}
	wantRoutines := readLegacyRetirementFixture(t, "legacy_cognition_routines.txt")
	if got := extractLegacyRetirementArray(t, schema, "expected_routines"); !equalStrings(got, wantRoutines) {
		t.Fatalf("legacy routine inventory differs: got %d names, want %d", len(got), len(wantRoutines))
	}

	for _, required := range []string{
		"LOCK TABLE", "context_projections", "job_lifecycle_operations", "job_step_attempts",
		"task_entries", "task_events", "IN ACCESS EXCLUSIVE MODE",
		"legacy cognition retirement blocked: active cognition episodes remain",
		"legacy cognition retirement blocked: authoritative rows remain",
		"legacy cognition retirement blocked: trace schema authority differs from migration 061",
		"legacy cognition retirement blocked: shadow context projections remain",
		"DROP CONSTRAINT context_projections_usage_mode_check",
		"CHECK (usage_mode='live')", "DROP TRIGGER %I ON %I.%I",
		"DROP TABLE %I.%I", "DROP FUNCTION %I.%I(%s)",
		"ALTER TABLE job_step_attempts DROP CONSTRAINT job_step_attempts_exact_actor_unique",
		"require_task_node_supersession_event", "task_node_supersessions_require_event",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("retirement migration omitted %q", required)
		}
	}
	for _, trigger := range legacyCognitionCoreTriggerTriples() {
		if !strings.Contains(schema, trigger) {
			t.Errorf("retirement migration omitted core trigger triple %q", trigger)
		}
	}

	for _, forbidden := range []string{
		"DROP TABLE IF EXISTS", "DROP FUNCTION IF EXISTS", "DROP TRIGGER IF EXISTS",
		" CASCADE", "DELETE FROM cognition_", "UPDATE cognition_", "RENAME TO",
		"archive", "tombstone", "compatibility",
	} {
		if strings.Contains(schema, forbidden) {
			t.Errorf("retirement migration contains forbidden fallback/destructive form %q", forbidden)
		}
	}
}

func TestLegacyCognitionRoutineFixtureUsesPostgresIdentifierLimit(t *testing.T) {
	t.Parallel()
	want := "require_cognition_accepted_fact_materialization_terminal_revers"
	if len(want) != 63 {
		t.Fatalf("catalog identifier bytes=%d want 63", len(want))
	}
	routines := readLegacyRetirementFixture(t, "legacy_cognition_routines.txt")
	if !slicesContain(routines, want) {
		t.Fatalf("routine fixture omitted PostgreSQL-truncated catalog name %q", want)
	}
	if slicesContain(routines, want+"e") {
		t.Fatal("routine fixture used the 64-byte source spelling instead of pg_proc authority")
	}
}

func legacyCognitionCoreTriggerTriples() []string {
	return []string{
		"job_lifecycle_operations.job_lifecycle_operations_require_cognition_seals=>require_cognition_lifecycle_operation_seal_set",
		"task_entries.task_entries_require_cognition_accepted_fact=>require_cognition_accepted_fact_reverse",
		"task_entries.task_entries_require_cognition_proposal_disposition=>require_cognition_proposal_candidate_disposition",
		"task_entries.task_entries_require_cognition_proposal_materialization=>require_cognition_model_proposal_entry_materialization",
		"task_entries.task_entries_require_cognition_selected_decision=>require_cognition_selected_decision_reverse",
		"task_events.task_events_require_cognition_belief_revision=>require_cognition_hypothesis_rejection_materialization",
	}
}

func readLegacyRetirementFixture(t testing.TB, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for index := range lines {
		lines[index] = strings.TrimSpace(lines[index])
		if lines[index] == "" {
			t.Fatalf("%s contains an empty line", name)
		}
	}
	return lines
}

func extractLegacyRetirementArray(t testing.TB, schema, name string) []string {
	t.Helper()
	marker := name + " TEXT[] := ARRAY["
	start := strings.Index(schema, marker)
	if start < 0 {
		t.Fatalf("migration omitted %s array", name)
	}
	rest := schema[start+len(marker):]
	end := strings.Index(rest, "]::TEXT[];")
	if end < 0 {
		t.Fatalf("migration has unterminated %s array", name)
	}
	matches := regexp.MustCompile(`'([a-z0-9_]+)'`).FindAllStringSubmatch(rest[:end], -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, match[1])
	}
	return out
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
