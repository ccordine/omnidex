package queue

import (
	"os"
	"strings"
	"testing"
)

const (
	legacyCoderTokenizerProfileMigration = "104_legacy_coder_tokenizer_profile_authority.sql"
	codeQwenTokenizerProfile             = "ollama-0.24.0-qwen2-llama-default-code-boundary-v1"
	codeGemmaFIMTokenizerProfile         = "ollama-0.24.0-gemma-llama-default-fim-boundary-v1"
	codeGemmaChatTokenizerProfile        = "ollama-0.24.0-gemma-llama-default-chat-boundary-v1"
	codeLlamaTokenizerProfile            = "ollama-0.24.0-llama-llama-default-code-boundary-v1"
	deepSeekCoderTokenizerProfile        = "ollama-0.24.0-llama-gpt2-no-pre-deepseek-code-boundary-v1"
	deepSeekCoderV2TokenizerProfile      = "ollama-0.24.0-deepseek2-gpt2-deepseek-llm-code-boundary-v1"
)

func TestLegacyCoderTokenizerProfileMigrationKeepsOneClosedProfileSet(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + legacyCoderTokenizerProfileMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range append(registeredTokenizerProfilesThrough103(),
		codeQwenTokenizerProfile, codeGemmaFIMTokenizerProfile,
		codeGemmaChatTokenizerProfile, codeLlamaTokenizerProfile,
		deepSeekCoderTokenizerProfile, deepSeekCoderV2TokenizerProfile,
	) {
		if !strings.Contains(source, required) {
			t.Errorf("legacy coder tokenizer migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_call_openings", "DELETE FROM station_call_openings",
		"fallback", "compatibility", "LIKE 'ollama-%'", "SIMILAR TO",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Errorf("legacy coder tokenizer migration contains forbidden %q", forbidden)
		}
	}
}

func TestLegacyCoderTokenizerProfileMigrationChangesOnlyForwardAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "103")); err != nil {
		t.Fatal(err)
	}
	before := tokenizerProfileConstraintDefinition(t, pool)
	for _, profile := range []string{
		codeQwenTokenizerProfile, codeGemmaFIMTokenizerProfile,
		codeGemmaChatTokenizerProfile, codeLlamaTokenizerProfile,
		deepSeekCoderTokenizerProfile, deepSeekCoderV2TokenizerProfile,
	} {
		if strings.Contains(before, profile) {
			t.Fatalf("migration 103 unexpectedly accepts coder profile %q", profile)
		}
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "104")); err != nil {
		t.Fatal(err)
	}
	after := tokenizerProfileConstraintDefinition(t, pool)
	for _, registered := range append(registeredTokenizerProfilesThrough103(),
		codeQwenTokenizerProfile, codeGemmaFIMTokenizerProfile,
		codeGemmaChatTokenizerProfile, codeLlamaTokenizerProfile,
		deepSeekCoderTokenizerProfile, deepSeekCoderV2TokenizerProfile,
	) {
		if !strings.Contains(after, registered) {
			t.Fatalf("migration 104 constraint=%q lacks %q", after, registered)
		}
	}
	for _, forbidden := range []string{"unregistered", "ollama-%", "SIMILAR TO"} {
		if strings.Contains(after, forbidden) {
			t.Fatalf("migration 104 constraint=%q contains open authority %q", after, forbidden)
		}
	}
}

func registeredTokenizerProfilesThrough103() []string {
	return []string{
		qwen35TokenizerProfile, qwen3TokenizerProfile, qwen2BOSTokenizerProfile,
		mistral3TokenizerProfile, phi3GPT4OTokenizerProfile, phi3DBRXTokenizerProfile,
		gemma3TokenizerProfile, llama32TokenizerProfile, qwen25CoderTokenizerProfile,
		qwen3NativeTokenizerProfile,
	}
}
