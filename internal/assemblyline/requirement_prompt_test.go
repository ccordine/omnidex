package assemblyline

import (
	"strings"
	"testing"
)

func TestFeatureExtractionPromptHasOneSemanticJobAndNoProductRecipe(t *testing.T) {
	t.Parallel()

	request := "Make an unfamiliar interactive thing with a movable control."
	prompt, err := BuildRequirementPartitionPrompt(RequirementPartitionInput{
		SourceText: request, Mode: RequirementExtractFeatures,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"every explicit", "shortest exact contiguous", "USER_REQUEST"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt missing %q:\n%s", required, prompt)
		}
	}
	fixed := strings.ReplaceAll(prompt, request, "")
	for _, productSpecific := range []string{
		"audio", "music", "sequencer", "drawing", "expense", "converter", "habit", "React", "Go CLI",
	} {
		if strings.Contains(fixed, productSpecific) {
			t.Fatalf("generic extractor contains product recipe %q:\n%s", productSpecific, fixed)
		}
	}
}

func TestFeatureExtractionRejectsParaphrasesOverlapAndReordering(t *testing.T) {
	t.Parallel()

	input := RequirementPartitionInput{
		SourceText: "build an inventory with grouped records and my own saved filter and summary",
		Mode:       RequirementExtractFeatures,
	}
	valid := RequirementPartitionDecision{
		Schema: RequirementPartitionSchemaV1,
		FeatureQuotes: []string{
			"grouped records", "my own saved filter", "summary",
		},
	}
	if err := valid.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	for name, quotes := range map[string][]string{
		"paraphrase": {"categorized entries"},
		"overlap":    {"my own saved filter", "saved filter"},
		"reordered":  {"summary", "grouped records"},
	} {
		t.Run(name, func(t *testing.T) {
			decision := RequirementPartitionDecision{Schema: RequirementPartitionSchemaV1, FeatureQuotes: quotes}
			if err := decision.ValidateFor(input); err == nil {
				t.Fatalf("accepted invalid feature extraction %#v", quotes)
			}
		})
	}
}

func TestFeatureExtractionMayReturnEmptyButFeatureSplitMayNot(t *testing.T) {
	t.Parallel()

	empty := RequirementPartitionDecision{Schema: RequirementPartitionSchemaV1, FeatureQuotes: []string{}}
	if err := empty.ValidateFor(RequirementPartitionInput{
		SourceText: "a compact browser catalog", Mode: RequirementExtractFeatures,
	}); err != nil {
		t.Fatal(err)
	}
	if err := empty.ValidateFor(RequirementPartitionInput{
		SourceText: "grouped records", Mode: RequirementSplitFeature,
	}); err == nil {
		t.Fatal("feature split accepted an empty result")
	}
}

func TestFeatureSplitPromptSeesOnlyItsEnvelope(t *testing.T) {
	t.Parallel()

	prompt, err := BuildRequirementPartitionPrompt(RequirementPartitionInput{
		SourceText: "saved filter and summary", Mode: RequirementSplitFeature,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"FEATURE_ENVELOPE", "already known", "saved filter and summary"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("feature split prompt missing %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{"USER_REQUEST", "workspace", "filename"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("feature split prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
}
