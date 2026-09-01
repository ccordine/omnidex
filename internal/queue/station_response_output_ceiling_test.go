package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestExpectedPortableStationMaxOutputTokensBoundsSourceBody(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
		Language: "typescript", Dialect: "TypeScript",
		Signature: "function value(): string", Behavior: "Return one value.",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExpectedPortableStationMaxOutputTokens(job, llm.MinInferenceContextTokens)
	if err != nil {
		t.Fatal(err)
	}
	if got != maxSourceBodyOutputTokens {
		t.Fatalf("source output budget = %d, want %d", got, maxSourceBodyOutputTokens)
	}
}

func TestExpectedPortableStationMaxOutputTokensBoundsOpaqueChoice(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewArtifactHandlingJob(assemblyline.ArtifactHandlingInput{
		UserRequest: "Keep ARTIFACT_1 unchanged.", Token: "ARTIFACT_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExpectedPortableStationMaxOutputTokens(job, llm.MinInferenceContextTokens)
	if err != nil {
		t.Fatal(err)
	}
	if got != minSingleLineCompletionTokens {
		t.Fatalf("opaque output budget = %d, want bounded completion room %d", got, minSingleLineCompletionTokens)
	}
}

func TestExpectedSourceBodyCorrectionBudgets(t *testing.T) {
	t.Parallel()
	open, err := ExpectedSourceBodyCorrectionMaxOutputTokens(0, false, llm.MinInferenceContextTokens)
	if err != nil {
		t.Fatal(err)
	}
	opaque, err := ExpectedSourceBodyCorrectionMaxOutputTokens(1, true, llm.MinInferenceContextTokens)
	if err != nil {
		t.Fatal(err)
	}
	if open != maxSourceBodyCorrectionOutputTokens || opaque != minSingleLineCompletionTokens {
		t.Fatalf("correction budgets = (open=%d opaque=%d)", open, opaque)
	}
}

func TestExpectedPortableStationMaxOutputTokensRejectsInvalidNativeContext(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
		Language: "typescript", Dialect: "TypeScript",
		Signature: "function value(): string", Behavior: "Return one value.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExpectedPortableStationMaxOutputTokens(
		job, llm.MinInferenceContextTokens-1,
	); err == nil {
		t.Fatal("invalid native context was accepted")
	}
}
