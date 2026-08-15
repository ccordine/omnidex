package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	qwen25CoderTokenizerProfileMigration = "102_qwen25_coder_tokenizer_profile_authority.sql"
	qwen25CoderTokenizerProfile          = "ollama-0.24.0-qwen2-gpt2-qwen2-no-bos-boundary-v1"
)

func TestQwen25CoderTokenizerProfileMigrationKeepsOneClosedProfileSet(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + qwen25CoderTokenizerProfileMigration)
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
	} {
		if !strings.Contains(source, required) {
			t.Errorf("Qwen 2.5 Coder tokenizer migration lacks %q", required)
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
			t.Errorf("Qwen 2.5 Coder tokenizer migration contains forbidden %q", forbidden)
		}
	}
}

func TestQwen25CoderTokenizerProfileMigrationChangesOnlyForwardAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "101")); err != nil {
		t.Fatal(err)
	}
	before := tokenizerProfileConstraintDefinition(t, pool)
	if strings.Contains(before, qwen25CoderTokenizerProfile) {
		t.Fatalf("migration 101 unexpectedly accepts Qwen 2.5 Coder profile: %s", before)
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "102")); err != nil {
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
	} {
		if !strings.Contains(after, registered) {
			t.Fatalf("migration 102 constraint=%q lacks %q", after, registered)
		}
	}
	for _, forbidden := range []string{"unregistered", "ollama-%", "SIMILAR TO"} {
		if strings.Contains(after, forbidden) {
			t.Fatalf("migration 102 constraint=%q contains open authority %q", after, forbidden)
		}
	}
}

func tokenizerProfileConstraintDefinition(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var definition string
	if err := pool.QueryRow(t.Context(), `
		SELECT pg_get_constraintdef(oid, true)
		FROM pg_constraint
		WHERE conrelid='station_call_openings'::regclass
		  AND conname='station_call_openings_tokenizer_profile_check'
	`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	return definition
}
