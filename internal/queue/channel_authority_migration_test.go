package queue

import (
	"os"
	"strings"
	"testing"
)

func TestChannelAuthorityMigrationIsTypedAndFailClosed(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("../../migrations/069_channel_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"LOCK TABLE ai_channels, ai_channel_messages, jobs IN ACCESS EXCLUSIVE MODE",
		"cannot install channel authority: message",
		"cannot install channel authority: channel",
		"rejected internal channel % still exists",
		"ADD COLUMN scope TEXT",
		"CHECK (scope = 'user')",
		"CHECK (role IN ('user','assistant'))",
		"channel_tags_are_exact",
		"idx_jobs_one_active_channel_turn",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration lacks %q", required)
		}
	}
	if strings.Contains(source, "SET scope = CASE") {
		t.Fatal("migration infers durable channel scope from legacy phrases")
	}
	for _, forbidden := range []string{"DROP TABLE IF EXISTS", "DROP FUNCTION IF EXISTS", "CASCADE", "ON CONFLICT", "fallback", "tombstone", "archive"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("migration contains forbidden %q", forbidden)
		}
	}
}

func TestChannelWorkspaceBindingMigrationIsImmutableAndRefusesInference(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("../../migrations/071_channel_workspace_binding.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"LOCK TABLE ai_channels, projects, jobs IN ACCESS EXCLUSIVE MODE",
		"cannot install channel workspace binding: channel",
		"ADD COLUMN project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT",
		"ADD COLUMN workspace_root TEXT NOT NULL",
		"channel workspace binding is immutable",
		"ai_channels_binding_immutable",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE ai_channels SET project_id", "UPDATE ai_channels SET workspace_root",
		"COALESCE(", "ON CONFLICT", "fallback", "legacy",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("migration contains inferred or fallback binding %q", forbidden)
		}
	}
}
