package contextcompiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRequiredContextCompilesActualValuesWithoutInferenceOrHashes(t *testing.T) {
	for _, content := range []string{"The inspection is on Tuesday.", "The preferred display language is French."} {
		t.Run(content, func(t *testing.T) {
			candidate, err := assemblyline.NewContextCandidateAuthority("repository", "CTX_1", content)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Compile(context.Background(), pairwiseContextRequest(),
				pairwiseContextProvider{set: CandidateSet{Required: []assemblyline.ContextCandidateAuthority{candidate}}},
				Stations{})
			if err != nil {
				t.Fatal(err)
			}
			if result.ModelCalls != 0 || len(result.Context.Capsules) != 1 || result.Context.Capsules[0].Content != content {
				t.Fatalf("compiled context = %+v", result)
			}
			encoded, err := json.Marshal(result.Context)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "sha256") {
				t.Fatalf("context retained hash bookkeeping: %s", encoded)
			}
		})
	}
}

func TestContextValidationComparesActualCandidateAndFeedbackValues(t *testing.T) {
	candidate := assemblyline.ContextCandidateAuthority{
		Namespace: "objective_replan", CandidateID: "CTX_1", Content: "Use the revised deadline.",
	}
	set := CandidateSet{
		Required: []assemblyline.ContextCandidateAuthority{candidate},
		Replan:   &assemblyline.ObjectiveReplanAuthority{JobID: 9, Generation: 2, Feedback: candidate.Content},
	}
	if err := validateCandidateSet(set); err != nil {
		t.Fatal(err)
	}
	set.Replan.Feedback = "Keep the original deadline."
	if err := validateCandidateSet(set); err == nil || !strings.Contains(err.Error(), "differs from exact replan authority") {
		t.Fatalf("different feedback was not rejected: %v", err)
	}
	set.Replan = nil
	set.Required[0].Namespace = "repository"
	duplicate := set.Required[0]
	duplicate.CandidateID = "CTX_2"
	set.Optional = []assemblyline.ContextCandidateAuthority{duplicate}
	if err := validateCandidateSet(set); err == nil || !strings.Contains(err.Error(), "duplicates exact provider content") {
		t.Fatalf("duplicate actual text was not rejected: %v", err)
	}
}
