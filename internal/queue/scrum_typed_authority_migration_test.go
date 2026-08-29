package queue

import (
	"os"
	"strings"
	"testing"
)

func TestScrumTypedAuthorityMigrationFailsClosed(t *testing.T) {
	source, err := os.ReadFile("../../migrations/076_scrum_typed_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"agent_stream_chat_cursor BIGINT NOT NULL",
		"agent_stream_console_cursor BIGINT NOT NULL",
		"step_context_cursor BIGINT NOT NULL",
		"NOT (column_name = 'in_progress' AND job_id <> '')",
		"sync_job_id <> '' AND sync_job_id = job_id",
		"migration 076 cannot establish exact Scrum cursor authority",
		"NOT (settings ? 'scrum_auto_review')",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("migration lacks %q", required)
		}
	}
}
