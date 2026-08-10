package cognitiongauntlet

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func TestPausingExactClientSealsEntryBeforeDelegating(t *testing.T) {
	brain := mustRatGeneration(t).Fixed.Brain
	base := &witnessPolicyClient{model: brain.Model, witness: nil}
	path := filepath.Join(t.TempDir(), "live-generation.json")
	attempt := model.StepAttemptAuthority{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "old-worker",
	}
	wrapped, err := newPausingExactClient(base, attempt, path)
	if err != nil {
		t.Fatal(err)
	}
	client := wrapped.(*pausingExactClient)
	paused := false
	client.pause = func() error { paused = true; return nil }
	zero := 0.0
	prepared := llm.PreparedModel{
		BaseModel: brain.Model, ContextModel: brain.Model, Prompt: "bounded prompt",
		PromptHint: llm.MinimalGeneratePrompt, MaxOutputTokens: 1,
		ContextTokens: brain.NativeContextLimit, ResponseFormat: llm.ResponseFormatJSON,
		ResponseSchema: map[string]any{"type": "object"}, Temperature: &zero,
		ProviderIdentityExpectation: &llm.ProviderIdentityExpectation{
			Backend: brain.Backend, BackendVersion: brain.BackendVersion,
			Model: brain.Model, Digest: brain.Digest, Quantization: brain.Quantization,
			NativeContextLimit: brain.NativeContextLimit,
		}, ProviderObservationChallenge: brain.ProviderObservation.ChallengeSHA256,
	}
	_, generationErr := client.GeneratePreparedExact(context.Background(), prepared)
	if generationErr == nil || !paused {
		t.Fatalf("generation error=%v paused=%t", generationErr, paused)
	}
	checkpoint, err := LoadLiveGenerationCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	wantSHA, err := preparedModelAuthoritySHA256(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.PreparedSHA256 != wantSHA || checkpoint.Attempt != attempt {
		t.Fatalf("checkpoint=%+v", checkpoint)
	}
	if errors.Is(generationErr, context.Canceled) {
		t.Fatal("wrapper replaced the provider result")
	}
}
