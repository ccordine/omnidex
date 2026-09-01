package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationClosedChoicesExposeOnlyOpaqueLetters(t *testing.T) {
	t.Parallel()
	resultPresenceChoices := func(
		dimension ApplicationRequirementCandidateResultDimension,
	) func() ([]OpaqueModelChoice, error) {
		return func() ([]OpaqueModelChoice, error) {
			present, absent, err := applicationRequirementCandidateResultPresenceDescriptions(dimension)
			if err != nil {
				return nil, err
			}
			return applicationRequirementCandidateResultPresenceOpaqueChoices(present, absent)
		}
	}
	fixtures := []struct {
		name  string
		build func() ([]OpaqueModelChoice, error)
	}{
		{"application classification", applicationClassificationOpaqueChoices},
		{"artifact handling", artifactHandlingOpaqueChoices},
		{"requirement authorization", applicationRequirementCandidateAuthorizationOpaqueChoices},
		{"requirement cardinality", applicationRequirementCandidateCardinalityOpaqueChoices},
		{"runtime-content presence", func() ([]OpaqueModelChoice, error) {
			return applicationRequirementCandidateContentPresenceOpaqueChoices(
				ApplicationRequirementCandidateRuntimeContentDimension,
			)
		}},
		{"non-runtime-content presence", func() ([]OpaqueModelChoice, error) {
			return applicationRequirementCandidateContentPresenceOpaqueChoices(
				ApplicationRequirementCandidateNonRuntimeContentDimension,
			)
		}},
		{"runtime outcome relation", applicationRequirementCandidateOutcomeRelationOpaqueChoices},
		{"derived-value presence", resultPresenceChoices(ApplicationRequirementDerivedValueDimension)},
		{"determining-relation presence", resultPresenceChoices(ApplicationRequirementDeterminingRelationDimension)},
		{"project stack", func() ([]OpaqueModelChoice, error) {
			return applicationProjectStackConstraintOpaqueChoices([]ApplicationProjectStackCandidate{
				{CandidateID: "STACK_CANDIDATE_1", TechnicalFormat: "First technical format"},
				{CandidateID: "STACK_CANDIDATE_2", TechnicalFormat: "Second technical format"},
			})
		}},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			choices, err := fixture.build()
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := RenderOpaqueModelChoiceQuestion(
				"Which semantic description applies?",
				[]string{"Minimum semantic context."},
				choices,
			)
			if err != nil {
				t.Fatal(err)
			}
			for index, choice := range choices {
				if strings.Contains(prompt, choice.value) {
					t.Fatalf("prompt exposed code-owned value %q: %s", choice.value, prompt)
				}
				decoded, err := DecodeOpaqueModelChoice(opaqueModelChoiceID(index), choices)
				if err != nil {
					t.Fatal(err)
				}
				if decoded != choice.value {
					t.Fatalf("decoded value = %q, want %q", decoded, choice.value)
				}
				if _, err := DecodeOpaqueModelChoice(choice.value, choices); err == nil {
					t.Fatalf("code-owned value %q was accepted as model output", choice.value)
				}
			}
		})
	}
}
