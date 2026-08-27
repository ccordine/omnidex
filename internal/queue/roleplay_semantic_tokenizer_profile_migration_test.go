package queue

import (
	"os"
	"strings"
	"testing"
)

const (
	roleplaySemanticTokenizerProfileMigration = "154_roleplay_semantic_tokenizer_profile_authority.sql"
	roleplaySemanticTokenizerProfile          = "ollama-0.24.0-roleplay-semantic-completion-v1"
)

func TestRoleplaySemanticTokenizerProfileMigrationKeepsClosedAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + roleplaySemanticTokenizerProfileMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"BEGIN;", "COMMIT;", "pg_get_constraintdef(oid)", "convalidated",
		"expected_definition CONSTANT TEXT", "IS DISTINCT FROM expected_definition",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("roleplay semantic profile migration lacks postcondition authority %q", required)
		}
	}
	for _, required := range append(
		registeredTokenizerProfilesThrough104(),
		roleplayRawTokenizerProfile,
		roleplaySemanticTokenizerProfile,
	) {
		if !strings.Contains(source, required) {
			t.Errorf("roleplay semantic profile migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_call_openings", "DELETE FROM station_call_openings",
		"fallback", "compatibility", "LIKE 'ollama-%'", "SIMILAR TO",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Errorf("roleplay semantic profile migration contains forbidden %q", forbidden)
		}
	}
}

func TestRoleplaySemanticTokenizerProfileMigrationChangesOnlyForwardAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "153")); err != nil {
		t.Fatal(err)
	}
	before := tokenizerProfileConstraintDefinition(t, pool)
	if strings.Contains(before, roleplaySemanticTokenizerProfile) {
		t.Fatalf("migration 153 unexpectedly accepts semantic roleplay profile")
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "154")); err != nil {
		t.Fatal(err)
	}
	after := tokenizerProfileConstraintDefinition(t, pool)
	for _, registered := range append(
		registeredTokenizerProfilesThrough104(),
		roleplayRawTokenizerProfile,
		roleplaySemanticTokenizerProfile,
	) {
		if !strings.Contains(after, registered) {
			t.Fatalf("migration 154 constraint=%q lacks %q", after, registered)
		}
	}
	for _, forbidden := range []string{"unregistered", "ollama-%", "SIMILAR TO"} {
		if strings.Contains(after, forbidden) {
			t.Fatalf("migration 154 constraint=%q contains open authority %q", after, forbidden)
		}
	}
}
