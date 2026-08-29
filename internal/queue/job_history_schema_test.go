package queue

import (
	"os"
	"strings"
	"testing"
)

func TestJobHistoryMigrationAddsOnlyMissingKeysetIndex(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/032_job_history_indexes.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "idx_artifacts_job_id_id") ||
		!strings.Contains(source, "ON artifacts (job_id, id)") {
		t.Fatalf("history migration omitted artifact keyset index:\n%s", source)
	}
	for _, duplicate := range []string{
		"idx_job_steps_job_id", "idx_evidence_job_id_id",
		"idx_llm_call_evidence_job",
	} {
		if strings.Contains(source, duplicate) {
			t.Fatalf("history migration duplicated existing index %q", duplicate)
		}
	}
}
