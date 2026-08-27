package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRoleplaySemanticModelRouteMigrationRetiresSplitAuthority(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/156_roleplay_semantic_model_route.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"roleplay_semantic_model",
		"roleplay_canon_extraction_model",
		"roleplay_ongoing_action_model",
		"conflicting split roleplay model routes",
		"affected job is active",
		"UPDATE projects",
		"UPDATE jobs",
		"DROP TRIGGER jobs_chat_turn_binding_immutable",
		"CREATE TRIGGER jobs_chat_turn_binding_immutable",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("roleplay semantic route migration lacks %q", required)
		}
	}
}
