package queue

import (
	"os"
	"strings"
	"testing"
)

const scrumChannelRelationMigration = "089_scrum_channel_message_relation.sql"

func TestScrumChannelRelationMigrationRequiresCleanStartWithoutPreservationFallbacks(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/" + scrumChannelRelationMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	compact := strings.Join(strings.Fields(source), " ")

	for _, required := range []string{
		"migration 089 reset required",
		"FROM scrum_cards",
		"FROM scrum_channel_operations",
		"FROM scrum_flow_events",
		"kind = 'scrum_channel_message'",
		"DROP TABLE scrum_channel_operations",
		"DROP TABLE scrum_flow_events",
		"DROP COLUMN chat",
		"DROP COLUMN planning_chat",
		"DROP COLUMN console_log",
		"DROP COLUMN agent_stream_chat_cursor",
		"DROP COLUMN agent_stream_console_cursor",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("clean-start migration is missing %q", required)
		}
	}
	if !strings.Contains(compact, "LOCK TABLE lifecycle_operation_registry, scrum_channel_operations, scrum_flow_events, scrum_cards") {
		t.Fatal("clean-start precondition is not protected by one fixed-order lock")
	}

	for _, forbidden := range []string{
		"archive",
		"backfill",
		"tombstone",
		"pg_trigger_depth",
		"ON CONFLICT DO NOTHING",
		"IF EXISTS scrum_",
		"IF NOT EXISTS scrum_",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Fatalf("clean-start migration contains forbidden preservation/fallback authority %q", forbidden)
		}
	}
}

func TestScrumChannelCounterEnforcementUsesOnlyTheAppendedOrdinal(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/" + scrumChannelRelationMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "CREATE FUNCTION enforce_scrum_card_message_counters()")
	if start < 0 {
		t.Fatal("bounded Scrum counter enforcement function is missing")
	}
	end := strings.Index(source[start:], "CREATE FUNCTION reject_scrum_message_mutation()")
	if end < 0 {
		t.Fatal("bounded Scrum counter enforcement function is missing")
	}
	function := source[start : start+end]
	for _, required := range []string{
		"NEW.channel_message_count <> OLD.channel_message_count + 1",
		"ordinal=NEW.channel_message_count",
		"OLD.channel_content_bytes + appended_bytes",
	} {
		if !strings.Contains(function, required) {
			t.Errorf("bounded counter enforcement is missing %q", required)
		}
	}
	for _, unbounded := range []string{"COUNT(", "SUM("} {
		if strings.Contains(function, unbounded) {
			t.Errorf("counter enforcement retains unbounded aggregate %q", unbounded)
		}
	}
}
