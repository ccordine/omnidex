package queue

import (
	"os"
	"strings"
	"testing"
)

func TestCognitionEpistemicRolesMigrationExtendsOneWorkingSetAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/046_cognition_epistemic_roles.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"LOCK TABLE working_set_items, context_projection_selected_refs, context_projection_omitted_refs IN ACCESS EXCLUSIVE MODE",
		"DROP CONSTRAINT working_set_items_role_check",
		"DROP CONSTRAINT context_projection_selected_refs_role_check",
		"DROP CONSTRAINT context_projection_omitted_refs_role_check",
		"CONSTRAINT working_set_items_role_check",
		"CONSTRAINT context_projection_selected_refs_role_check",
		"CONSTRAINT context_projection_omitted_refs_role_check",
		"'fact'", "'hypothesis'",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("epistemic-role migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"transcript", "chat_history", "recent_message"} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("epistemic-role migration introduced fallback role %q", forbidden)
		}
	}
}
