package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRepositoryGroundingMigrationIsNarrowAndHashGuarded(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("../../migrations/074_repository_grounding_stations.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"expected_pre_sha256 CONSTANT TEXT := '3470c9b282bef241bf523a63ea665a38afaf64d7bf144934bc4eba8b0be4a2f8'",
		"expected_post_sha256 CONSTANT TEXT",
		"digest(convert_to(observed_source,'UTF8'),'sha256')",
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
		"WHEN 'repository_evidence_relevance' THEN station='repository_evidence_relevance'",
		"WHEN 'repository_grounded_review' THEN station='repository_grounded_review'",
		"WHEN 'repository_grounded_correction' THEN station='repository_grounded_correction'",
		"WHEN 'conversation_context_selection' THEN station='conversation_context_selection'",
		"prior station function hash",
		"repository grounding station postcondition failed",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"ALTER TABLE", "CREATE TABLE", "DROP TABLE", "DROP FUNCTION", "CASCADE",
		"IF EXISTS", "fallback", "legacy",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("migration contains out-of-scope %q", forbidden)
		}
	}
}
