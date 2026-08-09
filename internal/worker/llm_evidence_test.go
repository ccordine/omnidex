package worker

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestPortableLLMEvidenceIdentityComesFromValidatedImmutableWork(t *testing.T) {
	t.Parallel()

	job, err := assemblyline.NewApplicationClassificationJob(assemblyline.ApplicationClassificationInput{
		UserRequest: "Build a small inventory application.",
	})
	if err != nil {
		t.Fatal(err)
	}
	work, err := llmEvidenceWorkForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if work.ID != job.ID || work.Kind != string(job.Kind) {
		t.Fatalf("evidence work=%#v want id=%q kind=%q", work, job.ID, job.Kind)
	}
	job.ID = "forged"
	if _, err := llmEvidenceWorkForPortableJob(job); err == nil {
		t.Fatal("evidence accepted a forged portable work identity")
	}
}

func TestLLMEvidenceCapturesEffectivePreparedRequest(t *testing.T) {
	t.Parallel()

	contract := llmResponseContract{
		Format: llm.ResponseFormatJSON, MaxTokens: 512, PromptHint: "initial user prompt",
	}
	record := newLLMCallEvidenceRecord(
		7, "portable_semantic_worker", "requested-model", "initial system prompt",
		map[string]any{"type": "object"}, contract, 4096, 2,
		llmEvidenceWork{ID: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Kind: "application_classification"},
	)
	prepared := llm.PreparedModel{
		BaseModel: "base-model", ContextModel: "effective-model",
		Prompt: "exact prepared system", PromptHint: "exact prepared user",
		ResponseFormat: llm.ResponseFormatJSON, ResponseSchema: map[string]any{"type": "array"},
		ContextTokens: 8192, MaxOutputTokens: 768, ThinkingEnabled: true,
	}
	applyPreparedRequestToEvidence(&record, prepared)
	if record.RequestedModel != "requested-model" || record.Model != "effective-model" {
		t.Fatalf("models requested=%q effective=%q", record.RequestedModel, record.Model)
	}
	if record.SystemPrompt != prepared.Prompt || record.UserPrompt != prepared.PromptHint {
		t.Fatalf("prompts system=%q user=%q", record.SystemPrompt, record.UserPrompt)
	}
	if record.ContextTokens != 8192 || record.MaxOutputTokens != 768 {
		t.Fatalf("limits context=%d output=%d", record.ContextTokens, record.MaxOutputTokens)
	}
	if record.ResponseSchema["type"] != "array" {
		t.Fatalf("schema=%#v", record.ResponseSchema)
	}
	if !record.ThinkingEnabled {
		t.Fatal("native thinking state was omitted from evidence")
	}
}

func TestEvidencePersistenceFailureCannotTriggerAnUnrecordedProviderRetry(t *testing.T) {
	t.Parallel()

	err := llmEvidencePersistenceError{
		PersistenceError: errors.New("database unavailable"),
		CallError:        errors.New("ollama create failed: EOF"),
	}
	if shouldRetrySameModelAfterCreateEOF(err) {
		t.Fatal("evidence persistence failure allowed another provider attempt")
	}
}
