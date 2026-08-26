package assemblyline

import (
	"strings"
	"testing"
)

func TestSemanticStationInputsRejectQualifiedPathsBeforeInference(t *testing.T) {
	t.Parallel()
	const qualified = "/workspace/generated"
	emptyContext, err := BootstrapApplicationContext(
		qualified, ApplicationWorkspaceEmpty, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	existingContext, err := BootstrapApplicationContext(
		qualified, ApplicationWorkspaceExisting, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, build := range map[string]func() error{
		"application intent": func() error {
			_, err := NewApplicationIntentJob(ApplicationIntentInput{
				UserRequest: qualified, Context: emptyContext,
			})
			return err
		},
		"application context need": func() error {
			_, err := NewApplicationContextNeedJob(ApplicationContextNeedInput{
				UserRequest: qualified, Context: existingContext,
			})
			return err
		},
		"repository search term": func() error {
			_, err := NewRepositorySearchTermJob(RepositorySearchTermInput{
				UnresolvedConcept: qualified,
			})
			return err
		},
		"repository requirements": func() error {
			_, err := NewRepositoryRequirementInterpretationJob(
				RepositoryRequirementInterpretationInput{
					UserRequest: qualified, Context: existingContext,
				},
			)
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
		request, ApplicationWorkspaceExisting, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	searchInput := RepositorySearchTermInput{UnresolvedConcept: "Find the owner"}
	requirementInput := RepositoryRequirementInterpretationInput{
		UserRequest: request, Context: existingContext,
	}
	for name, validate := range map[string]func() error{
		"application intent": func() error {
			return (ApplicationIntentCandidate{
				Schema:         ApplicationIntentCandidateSchemaV1,
				ProductContext: "Read /mnt/data", Requirements: []string{"Return one value"},
			}).Validate()
		},
		"application context need": func() error {
			return (ApplicationContextNeedDecision{
				Schema:    ApplicationContextNeedSchemaV1,
				Questions: []string{"What owns C:\\private\\value?"},
			}).Validate()
		},
		"repository search term": func() error {
			return (RepositorySearchTermDecision{
				Schema: RepositorySearchTermSchemaV2, Anchors: []string{"foo/bar"},
			}).ValidateFor(searchInput)
		},
		"repository requirements": func() error {
			return (RepositoryRequirementInterpretation{
				Schema:       RepositoryRequirementInterpretationSchemaV2,
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
		Requirements:   []string{"Expose the requested behavior"},
	}).Validate(); err != nil {
		t.Fatalf("dotted product names were rejected: %v", err)
	}
}
