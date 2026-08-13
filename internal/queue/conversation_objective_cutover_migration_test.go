package queue

import (
	"os"
	"strings"
	"testing"
)

func TestConversationObjectiveCutoverMigrationIsExplicitAndFailLoud(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/066_conversation_objective_cutover.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE ai_channels, job_generations, jobs, job_steps, artifacts",
		"legacy accepted-intent projection rows",
		"legacy channel persona configuration",
		"nonterminal legacy conversation generation",
		"88a885d8fa8374bc4f771ff5f2960243997a13f73fe484d0dbeaf04fa06cd379",
		"DROP COLUMN persona",
		"DROP COLUMN system",
		"DROP COLUMN provider",
		"DROP COLUMN model",
		"DROP COLUMN context",
		"DROP TABLE task_artifact_projection_items",
		"DROP TABLE task_artifact_projections",
		"boundary_action IN ('v3_coding', 'objective_resolve', 'v3_planning')",
		"require_current_job_generation_boundary",
		"NEW.boundary_action NOT IN ('v3_coding', 'objective_resolve')",
		"6d35378110ee10f551a3db1f9384099ddcae7bbf2e15763262bafcb437e493b3",
		"b8eecfca02b64a0a72f493c64e93608a67adadaaaff3fc90a6dcf55ea3e02ed3",
		"trigger.tgtype=7",
		"conversation objective cutover postcondition failed",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("066 migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"CASCADE", "IF EXISTS", "DELETE FROM", "UPDATE task_artifact_projection",
		"UPDATE ai_channels", "fallback", "archive", "tombstone",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("066 migration contains forbidden cutover path %q", forbidden)
		}
	}
}

func TestConversationObjectiveCutoverIsTheSingle066Migration(t *testing.T) {
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "066_") && strings.HasSuffix(entry.Name(), ".sql") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("066 migration count=%d want 1", count)
	}
}
