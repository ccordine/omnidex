package queue

import (
	"os"
	"strings"
	"testing"
)

func TestLearnedSkillMigrationDefinesOneImmutableVersionedRegistry(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/023_worker_skills.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS worker_skills",
		"PRIMARY KEY (skill_id, version)",
		"worker_skill_checks",
		"worker_skill_dependencies",
		"worker_skill_embeddings",
		"prevent_worker_skill_content_update",
		"prevent_worker_skill_embedding_update",
		"WHERE status = 'active'",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("skill migration omitted %q", required)
		}
	}
}
