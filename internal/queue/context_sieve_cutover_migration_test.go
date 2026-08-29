package queue

import (
	"os"
	"strings"
	"testing"
)

const contextSieveCutoverMigration = "127_context_sieve_cutover.sql"

func TestContextSieveCutoverMigrationIsExactHashGuardedSuccessor(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + contextSieveCutoverMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"BEGIN;", "LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"339272763ca0bcd47f566a4bc27ee305b83d8dce925cab367ba0607d20f5b69f",
		"d941a57abf2c27bd33b7a6c04014ce5b491bbbee397ec5fa0214efdb9025b1f6",
		"d6a479f722926498992a63d043383b8c313a643fa8b310d3303571cb558a04e0",
		"observed_language <> 'sql'", "observed_volatility <> 'i'", "NOT observed_strict",
		"an active station opening violates migration 126 authority",
		"WITH RECURSIVE unresolved_chain AS",
		"unresolved opening contains retired station work",
		"LEFT JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id",
		"historical or active opening violates context sieve station authority",
		"WHEN 'context_search_terms' THEN station='context_search_terms'",
		"WHEN 'context_relevance' THEN station='context_relevance'",
		"WHEN 'context_minification' THEN station='context_minification'",
		"WHEN 'application_requirements' THEN station='coding_requirements'",
		"WHEN 'application_file_content' THEN station='coding_workload'",
		"WHEN 'application_job_specification_repair' THEN station='coding_workload'",
		"station IN ('coding_workload','coding_workload_review')",
		"retained only so immutable historical opening rows",
		"CREATE FUNCTION enforce_context_sieve_station_opening_insert()",
		"CREATE TRIGGER station_gap_openings_enforce_context_sieve_insert",
		"retired station work kind % cannot create a new opening",
		"retired station work kind % cannot create a correction opening",
		"nested response correction cannot create a new station opening",
		"response correction for % requires one non-blank retained_candidate",
		"jsonb_typeof(correction_payload->'retained_candidate') IS DISTINCT FROM 'string'",
		"original_kind IS DISTINCT FROM 'application_job_specification'",
		"original_kind IS DISTINCT FROM 'application_acceptance_grounding_review'",
		"trigger_authority.tgtype=7",
		"CREATE INDEX idx_ai_channel_messages_content_fts",
		"CREATE INDEX idx_roleplay_canon_events_content_fts",
		"CREATE INDEX idx_roleplay_character_memories_content_fts",
		"USING GIN (to_tsvector('simple',content))",
		"valid_index_count <> 3", "context sieve postcondition failed", "COMMIT;",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("context sieve cutover migration omitted %q", required)
		}
	}
	retiredGuard := strings.Index(source, "WITH RECURSIVE unresolved_chain AS")
	activeGuard := strings.Index(
		source, "an active station opening violates migration 126 authority",
	)
	if retiredGuard < 0 || activeGuard < 0 || retiredGuard >= activeGuard {
		t.Fatal("recursive retired-work guard must run before generic active-opening validation")
	}
	retiredGuardSource := source[retiredGuard:activeGuard]
	for _, workKind := range []string{
		"application_requirements",
		"application_file_content",
		"application_job_specification_repair",
		"application_job_specification_review",
		"conversation_context_selection",
		"memory_context_selection",
		"roleplay_narrative_continuity",
	} {
		if !strings.Contains(retiredGuardSource, "'"+workKind+"'") {
			t.Fatalf("recursive retired-work guard omitted %q", workKind)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_gap_openings", "DELETE FROM station_gap_openings",
		"DROP FUNCTION", "DROP TABLE", "CASCADE", "IF NOT EXISTS",
		"fallback", "compatibility",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Fatalf("context sieve cutover migration contains forbidden %q", forbidden)
		}
	}
}
