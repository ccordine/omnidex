package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func TestExpectedPortableStationMaxOutputTokensLeavesSourceBodyUnlimited(t *testing.T) {
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
	if got != -1 {
		t.Fatalf("source num_predict = %d, want provider-native unlimited", got)
	}
}

func TestExpectedPortableStationMaxOutputTokensLeavesOpaqueChoiceUnlimited(t *testing.T) {
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
	if got != -1 {
		t.Fatalf("opaque num_predict = %d, want provider-native unlimited", got)
	}
}

func TestExpectedPortableStationMaxOutputTokensLeavesInventoryUnlimited(t *testing.T) {
	t.Parallel()
	request := "Build software that lets a user confirm an item."
	context, err := assemblyline.BootstrapApplicationContext(request)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationRequirementInventoryJob(
		assemblyline.ApplicationRequirementInventoryInput{
			UserRequest: request, Context: context, ScopeMode: model.CodingScopeModeNormal,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExpectedPortableStationMaxOutputTokens(job, llm.MinInferenceContextTokens)
	if err != nil {
		t.Fatal(err)
	}
	if got != -1 {
		t.Fatalf("inventory num_predict = %d, want provider-native unlimited", got)
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
	if open != -1 || opaque != minSingleLineCompletionTokens {
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
