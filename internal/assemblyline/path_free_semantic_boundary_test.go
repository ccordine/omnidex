package assemblyline

import (
	"strings"
	"testing"
)

func TestSemanticStationInputsRejectQualifiedPathsBeforeInference(t *testing.T) {
	t.Parallel()
	const qualified = "/workspace/generated"
	emptyContext, err := BootstrapApplicationContext(
		qualified, ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	existingContext, err := BootstrapApplicationContext(
		qualified, ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, build := range map[string]func() error{
		"application classification": func() error {
			_, err := NewApplicationClassificationJob(ApplicationClassificationInput{
				UserRequest: qualified,
			})
			return err
		},
		"application product context": func() error {
			_, err := NewApplicationProductContextJob(ApplicationProductContextInput{
				UserRequest: qualified, Context: emptyContext,
			})
			return err
		},
		"application context need": func() error {
			_, err := NewApplicationContextNeedCoverageJob(ApplicationContextNeedLeafInput{
				UserRequest: qualified, Context: existingContext, AcceptedQuestions: []string{},
			})
			return err
		},
		"repository requirements": func() error {
			_, err := NewRepositoryRequirementCoverageJob(
				RepositoryRequirementLeafInput{
					Authority: RepositoryRequirementInterpretationInput{
						UserRequest: qualified, Context: existingContext,
					},
					AcceptedRequirements: []string{},
				},
			)
			return err
		},
		"artifact handling": func() error {
			_, err := NewArtifactHandlingJob(ArtifactHandlingInput{
				UserRequest: "Remove ARTIFACT_1 through " + qualified + ".",
				Token:       "ARTIFACT_1",
			})
			return err
		},
		"capability relation": func() error {
			_, err := NewCapabilityRelationJob(CapabilityRelationInput{
				LocalContext: "One local application.",
				LeftNeed:     "Read " + qualified,
				RightNeed:    "Return a value.",
			})
			return err
		},
		"skill selection": func() error {
			_, err := NewSkillSelectionJob(SkillSelectionInput{
				LocalContext: "One local application.",
				Need:         "Read a value.",
				Candidates: []SkillCandidateSummary{{
					Token: "SKILL_1", Purpose: "Inspect " + qualified,
				}},
			})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := build(); err == nil || !strings.Contains(err.Error(), "filesystem identity") {
				t.Fatalf("path-bearing input error=%v", err)
			}
		})
	}
}

func TestSemanticStationCandidatesRejectQualifiedPathsAtAcceptance(t *testing.T) {
	t.Parallel()
	const request = "Improve the existing service."
	existingContext, err := BootstrapApplicationContext(
		request, ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	requirementInput := RepositoryRequirementInterpretationInput{
		UserRequest: request, Context: existingContext,
	}
	for name, validate := range map[string]func() error{
		"application intent": func() error {
			return (ApplicationIntentCandidate{
				Schema: ApplicationIntentCandidateSchemaV1, ProductContext: "Read /mnt/data",
				Requirements: []ApplicationIntentCandidateRequirement{
					applicationIntentCandidateRequirementFixture(
						t,
						"Return one value", ApplicationRequirementNoDerivedResult,
					),
				},
			}).Validate()
		},
		"application context need": func() error {
			return (ApplicationContextNeedDecision{
				Schema:    ApplicationContextNeedSchemaV1,
				Questions: []string{"What owns C:\\private\\value?"},
			}).Validate()
		},
		"repository requirements": func() error {
			return (RepositoryRequirementInterpretation{
				Schema:       RepositoryRequirementInterpretationSchemaV3,
				Requirements: []string{"Update ../private"},
			}).ValidateFor(requirementInput)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := validate(); err == nil || !strings.Contains(err.Error(), "filesystem identity") {
				t.Fatalf("path-bearing candidate error=%v", err)
			}
		})
	}
}

func TestSemanticBoundariesRetainUnprovenDottedProductNames(t *testing.T) {
	t.Parallel()
	if err := (ApplicationIntentCandidate{
		Schema:         ApplicationIntentCandidateSchemaV1,
		ProductContext: "Node.js service with Vue.js interface",
		Requirements: []ApplicationIntentCandidateRequirement{
			applicationIntentCandidateRequirementFixture(
				t,
				"Expose the requested behavior", ApplicationRequirementNoDerivedResult,
			),
		},
	}).Validate(); err != nil {
		t.Fatalf("dotted product names were rejected: %v", err)
	}
}
