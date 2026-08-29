package queue

import (
	"os"
	"strings"
	"testing"
)

func TestMemoryObjectiveContextMigrationIsFailClosedAndHashGuarded(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/081_memory_objective_context_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"40544be99da5f06f232982d49697763eb9ba4dcb5f7c4bffb38f4a18efd46eb4",
		"c0fd460253bc36461089b326b45cf1ef2828c0a3c7063b106c231d5f0145196d",
		"cannot install exact memory scope while unscoped memory authority exists",
		"FOREIGN KEY (channel_id,project_id)",
		"REFERENCES ai_channels(id,project_id) ON DELETE RESTRICT",
		"memory_candidates_source_memory_scope_fkey",
		"memory_candidates_promoted_memory_scope_fkey",
		"durable memory capsules are immutable",
		"memory candidate scope is immutable",
		"job_generations_objective_feedback_bounded",
		"octet_length(feedback)<=2048",
		"WHEN 'memory_context_selection' THEN station='memory_context_selection'",
		") IS DISTINCT FROM TRUE",
		"COALESCE(station_owns_portable_work(",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("memory objective context migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"IF NOT EXISTS", "ON DELETE CASCADE", "source LIKE", "tag LIKE", "fallback", "legacy",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("memory objective context migration contains forbidden %q", forbidden)
		}
	}
}
