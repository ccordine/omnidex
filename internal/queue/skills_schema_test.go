package queue

import (
	"os"
	"strings"
	"testing"
)

func TestWorkerSkillSchemaRetainsOnlyRetrievalAuthority(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/079_worker_skill_retrieval_only.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"worker_skills", "worker_skill_embeddings",
		"worker_skill_embeddings_one_frozen_identity",
		"learned skill mutation is unavailable",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("retrieval-only skill schema omitted %q", required)
		}
	}
	for _, removed := range []string{
		"CREATE TABLE worker_skill_checks",
		"CREATE TABLE worker_skill_dependencies",
		"CREATE TABLE worker_skill_promotion_receipts",
	} {
		if strings.Contains(source, removed) {
			t.Fatalf("retrieval-only skill schema recreated %q", removed)
		}
	}
}
