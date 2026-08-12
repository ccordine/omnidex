package queue

import (
	"os"
	"strings"
	"testing"
)

func TestCognitionAcceptedFactMaterializationMigrationOwnsPortableAuthority(t *testing.T) {
	sql := readAcceptedFactMaterializationMigrations(t,
		"064_cognition_accepted_fact_materialization.sql",
		"064_cognition_accepted_fact_materialization_authority.sql",
	)
	for _, required := range []string{
		"CREATE TABLE cognition_accepted_fact_materializations",
		"fact_id TEXT NOT NULL UNIQUE REFERENCES cognition_accepted_facts(fact_id)",
		"pre_fact_ledger_json TEXT NOT NULL",
		"pre_fact_ledger_sha256 TEXT NOT NULL",
		"pre_fact_ledger_json_sha256 TEXT NOT NULL",
		"command_id TEXT NOT NULL UNIQUE",
		"payload_json TEXT NOT NULL",
		"require_exact_cognition_accepted_fact_materialization",
		"require_cognition_accepted_fact_materialization_reverse",
		"DEFERRABLE INITIALLY DEFERRED",
		"cognition_accepted_fact_materializations_immutable",
		"cognition_accepted_fact_materializations_no_truncate",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("accepted-fact materialization migration lacks %q", required)
		}
	}
}

func readAcceptedFactMaterializationMigrations(t *testing.T, names ...string) string {
	t.Helper()
	var sql strings.Builder
	for _, name := range names {
		raw, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		sql.Write(raw)
	}
	return sql.String()
}

func TestCognitionAcceptedFactMaterializationTraceMigrationOwnsTerminalTotality(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/064_cognition_accepted_fact_materialization_trace.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"accepted_fact_materialization",
		"guard_cognition_accepted_fact_materialization_active_episode",
		"require_terminal_cognition_accepted_fact_materialization_trace",
		"require_cognition_accepted_fact_materialization_terminal_reverse",
		"DEFERRABLE INITIALLY DEFERRED",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("accepted-fact trace migration lacks %q", required)
		}
	}
}
