package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRepositoryMutationJournalSchemaIsDurableAndImmutable(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/035_repository_mutation_journal.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE TABLE repository_mutation_operations",
		"CREATE TABLE repository_mutation_files",
		"status IN ('prepared', 'applying', 'applied', 'indeterminate')",
		"FOREIGN KEY (job_id, generation)",
		"FOREIGN KEY (job_id, generation, step_id)",
		"REFERENCES repository_snapshots(id) ON DELETE RESTRICT",
		"FOREIGN KEY (job_id, evidence_id)",
		"CREATE UNIQUE INDEX idx_repository_mutations_unresolved_generation",
		"ON repository_mutation_operations (job_id, generation)",
		"prevent_repository_mutation_identity_change",
		"prevent_repository_mutation_file_change",
		"prevent_repository_mutation_evidence_change",
		"sealed_at TIMESTAMPTZ",
		"repository mutation file authority is sealed",
		"repository mutation must be sealed with 1-8 exact files",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("repository mutation journal migration is missing %q", required)
		}
	}
}
