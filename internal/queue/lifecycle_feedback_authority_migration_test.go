package queue

import (
	"os"
	"strings"
	"testing"
)

func TestLifecycleFeedbackAuthorityMigrationIsNarrowAndFailsClosed(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/088_exact_lifecycle_feedback_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE job_generations, task_entries IN ACCESS EXCLUSIVE MODE",
		"server_encoding",
		"UTF8",
		"lifecycle_feedback_is_valid",
		"btrim",
		"octet_length(value) BETWEEN 1 AND maximum_bytes",
		"convert_from(convert_to(value, 'UTF8'), 'UTF8') = value",
		"position(decode('00','hex') in convert_to(value,'UTF8')) = 0",
		"job_generations_authoritative_shape",
		"task_entries_content_check",
		"kind='feedback'",
		"task_ledger_text_is_exact(content)",
		"historical lifecycle feedback",
		"feedback_sha256 = encode(digest(feedback, 'sha256'), 'hex')",
		"content_sha256 <> encode(digest(content, 'sha256'), 'hex')",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("exact lifecycle-feedback migration is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"CREATE OR REPLACE FUNCTION task_ledger_text_is_exact",
		"UPDATE job_generations",
		"UPDATE task_entries",
		"DELETE FROM",
		"CASCADE",
		"IF EXISTS",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("exact lifecycle-feedback migration contains forbidden broad mutation %q", forbidden)
		}
	}
}
