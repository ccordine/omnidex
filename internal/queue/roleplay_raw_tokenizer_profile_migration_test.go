package queue

import (
	"os"
	"strings"
	"testing"
)

const roleplayRawTokenizerProfile = "ollama-0.24.0-roleplay-raw-completion-v1"

func TestRoleplayGenerationMigrationAddsOneClosedRawProfile(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/124_roleplay_character_generation_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range append(registeredTokenizerProfilesThrough104(), roleplayRawTokenizerProfile) {
		if !strings.Contains(source, required) {
			t.Errorf("roleplay generation migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_call_openings", "DELETE FROM station_call_openings",
		"fallback", "compatibility", "LIKE 'ollama-%'", "SIMILAR TO",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Errorf("roleplay generation migration contains forbidden %q", forbidden)
		}
	}
}

func TestRoleplayGenerationMigrationChangesOnlyForwardProfileAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "123")); err != nil {
		t.Fatal(err)
	}
	before := tokenizerProfileConstraintDefinition(t, pool)
	if strings.Contains(before, roleplayRawTokenizerProfile) {
		t.Fatalf("migration 123 unexpectedly accepts raw roleplay profile")
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "124")); err != nil {
		t.Fatal(err)
	}
	after := tokenizerProfileConstraintDefinition(t, pool)
	for _, registered := range append(registeredTokenizerProfilesThrough104(), roleplayRawTokenizerProfile) {
		if !strings.Contains(after, registered) {
			t.Fatalf("migration 124 constraint=%q lacks %q", after, registered)
		}
	}
	for _, forbidden := range []string{"unregistered", "ollama-%", "SIMILAR TO"} {
		if strings.Contains(after, forbidden) {
			t.Fatalf("migration 124 constraint=%q contains open authority %q", after, forbidden)
		}
	}
}

func registeredTokenizerProfilesThrough104() []string {
	return append(registeredTokenizerProfilesThrough103(),
		codeQwenTokenizerProfile, codeGemmaFIMTokenizerProfile,
		codeGemmaChatTokenizerProfile, codeLlamaTokenizerProfile,
		deepSeekCoderTokenizerProfile, deepSeekCoderV2TokenizerProfile,
	)
}
