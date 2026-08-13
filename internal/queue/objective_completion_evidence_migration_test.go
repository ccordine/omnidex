package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObjectiveCompletionEvidenceMigrationFreezesAtomicAuthority(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "080_objective_completion_evidence_authority.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(content)
	for _, required := range []string{
		"step_completion_evidence_sets",
		"objective_completion_evidence_set_is_valid",
		"job_lifecycle_operations_require_objective_evidence",
		"evidence_objective_completion_authority",
		"DEFERRABLE INITIALLY DEFERRED",
		"objective completion evidence authority is immutable",
		"unauthenticated objective citations exist",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration lacks %q", required)
		}
	}
}
