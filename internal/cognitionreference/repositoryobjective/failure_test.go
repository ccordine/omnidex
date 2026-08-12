package repositoryobjective

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func TestRunRejectsInvalidObjectiveBeforeRepositoryWork(t *testing.T) {
	root := deliveryFixture(t)
	validAcceptance := fullAcceptance()
	for _, test := range []struct {
		name      string
		objective Objective
	}{
		{name: "unclean root", objective: Objective{
			ID: "objective.invalid", Root: root + "/.",
			Subject:    SubjectLookup{Kind: LookupQualifiedName, Value: "example.test/delivery.Dispatch"},
			Acceptance: validAcceptance,
		}},
		{name: "duplicate acceptance", objective: Objective{
			ID: "objective.invalid", Root: root,
			Subject:    SubjectLookup{Kind: LookupQualifiedName, Value: "example.test/delivery.Dispatch"},
			Acceptance: []AcceptancePredicate{AcceptanceSubjectResolved, AcceptanceSubjectResolved},
		}},
		{name: "unsupported lookup", objective: Objective{
			ID: "objective.invalid", Root: root,
			Subject: SubjectLookup{Kind: "semantic", Value: "Dispatch"}, Acceptance: validAcceptance,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := Run(t.Context(), test.objective, nil)
			if !errors.Is(err, ErrInvalidObjective) || result.Complete || len(result.Steps) != 0 ||
				result.ObjectiveID != "" || result.Acceptance != nil {
				t.Fatalf("result=%#v error=%v", result, err)
			}
		})
	}
}

func TestRunRejectsSemanticMetadataWhenCodeResolvesSubject(t *testing.T) {
	result, err := Run(t.Context(), Objective{
		ID: "objective.exact", Root: deliveryFixture(t), Question: "Which one?",
		Subject:    SubjectLookup{Kind: LookupQualifiedName, Value: "example.test/delivery.Dispatch"},
		Acceptance: fullAcceptance(),
	}, nil)
	if !errors.Is(err, ErrInvalidObjective) || !strings.Contains(err.Error(), "forbids") ||
		result.SelectorCalls != 0 || result.Complete {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestRunFailsLoudlyForAbsentOrUnresolvedAmbiguousSubject(t *testing.T) {
	root := storageFixture(t)
	base := Objective{
		ID: "objective.lookup", Root: root,
		Acceptance: fullAcceptance(),
	}
	missing := base
	missing.Subject = SubjectLookup{Kind: LookupName, Value: "Absent"}
	if result, err := Run(t.Context(), missing, nil); !errors.Is(err, ErrSubjectAbsent) || result.Complete {
		t.Fatalf("absent result=%#v error=%v", result, err)
	}
	noQuestion := base
	noQuestion.Subject = SubjectLookup{Kind: LookupName, Value: "Resolve"}
	if result, err := Run(t.Context(), noQuestion, nil); !errors.Is(err, ErrSemanticResolution) || result.Complete {
		t.Fatalf("questionless result=%#v error=%v", result, err)
	}
	noSelector := noQuestion
	noSelector.Question = "Which declaration owns durable storage?"
	if result, err := Run(t.Context(), noSelector, nil); !errors.Is(err, ErrSemanticResolution) || result.Complete {
		t.Fatalf("selectorless result=%#v error=%v", result, err)
	}
}

func TestRunDoesNotFallbackFromSelectorFailureOrInvalidChoice(t *testing.T) {
	objective := Objective{
		ID: "objective.failure", Root: storageFixture(t), Question: "Which declaration owns durable storage?",
		Subject:    SubjectLookup{Kind: LookupName, Value: "Resolve"},
		Acceptance: fullAcceptance(),
	}
	providerErr := errors.New("provider unavailable")
	for _, test := range []struct {
		name     string
		selector cognitionreference.Selector
	}{
		{name: "provider error", selector: selectorFunc(func(context.Context, SemanticGap) (CandidateID, error) {
			return "", providerErr
		})},
		{name: "outside gap", selector: selectorFunc(func(context.Context, SemanticGap) (CandidateID, error) {
			return "C99", nil
		})},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := Run(t.Context(), objective, test.selector)
			if !errors.Is(err, ErrSemanticResolution) || result.Complete ||
				result.SelectorCalls != 1 || result.Subject.Symbol.SymbolID != "" {
				t.Fatalf("result=%#v error=%v", result, err)
			}
		})
	}
}
