package queue

import (
	"os"
	"strings"
	"testing"
)

const (
	qwen3NativeTokenizerProfileMigration = "103_qwen3_native_tokenizer_profile_authority.sql"
	qwen3NativeTokenizerProfile          = "ollama-0.24.0-qwen3-gpt2-qwen2-no-bos-boundary-v1"
)

func TestQwen3NativeTokenizerProfileMigrationKeepsOneClosedProfileSet(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + qwen3NativeTokenizerProfileMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE station_call_openings IN ACCESS EXCLUSIVE MODE",
		"DROP CONSTRAINT station_call_openings_tokenizer_profile_check",
		"ADD CONSTRAINT station_call_openings_tokenizer_profile_check",
		qwen35TokenizerProfile,
		qwen3TokenizerProfile,
		qwen2BOSTokenizerProfile,
		mistral3TokenizerProfile,
		phi3GPT4OTokenizerProfile,
		phi3DBRXTokenizerProfile,
		gemma3TokenizerProfile,
		llama32TokenizerProfile,
		qwen25CoderTokenizerProfile,
		qwen3NativeTokenizerProfile,
	} {
		if !strings.Contains(source, required) {
			t.Errorf("Qwen3 native tokenizer migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_call_openings",
		"DELETE FROM station_call_openings",
		"fallback",
		"compatibility",
		"LIKE 'ollama-%'",
		"SIMILAR TO",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Errorf("Qwen3 native tokenizer migration contains forbidden %q", forbidden)
		}
	}
}

func TestQwen3NativeTokenizerProfileMigrationChangesOnlyForwardAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "102")); err != nil {
		t.Fatal(err)
	}
	before := tokenizerProfileConstraintDefinition(t, pool)
	if strings.Contains(before, qwen3NativeTokenizerProfile) {
		t.Fatalf("migration 102 unexpectedly accepts Qwen3 native profile: %s", before)
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "103")); err != nil {
		t.Fatal(err)
	}
	after := tokenizerProfileConstraintDefinition(t, pool)
	for _, registered := range []string{
		qwen35TokenizerProfile,
		qwen3TokenizerProfile,
		qwen2BOSTokenizerProfile,
		mistral3TokenizerProfile,
		phi3GPT4OTokenizerProfile,
		phi3DBRXTokenizerProfile,
		gemma3TokenizerProfile,
		llama32TokenizerProfile,
		qwen25CoderTokenizerProfile,
		qwen3NativeTokenizerProfile,
	} {
		if !strings.Contains(after, registered) {
			t.Fatalf("migration 103 constraint=%q lacks %q", after, registered)
		}
	}
	for _, forbidden := range []string{"unregistered", "ollama-%", "SIMILAR TO"} {
		if strings.Contains(after, forbidden) {
			t.Fatalf("migration 103 constraint=%q contains open authority %q", after, forbidden)
		}
	}
}
