package repositoryobjective

import (
	"context"
	"errors"
	"testing"
)

func TestRunHonorsCancellationBeforeRepositoryWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := Run(ctx, Objective{}, nil)
	if !errors.Is(err, context.Canceled) || result.Complete || len(result.Steps) != 0 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestRunDiscardsSelectionWhenContextCancelsDuringInference(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	selector := selectorFunc(func(_ context.Context, gap SemanticGap) (CandidateID, error) {
		cancel()
		return gap.Candidates[0].ID, nil
	})
	result, err := Run(ctx, Objective{
		ID: "objective.cancel", Root: storageFixture(t), Question: "Which declaration owns durable storage?",
		Subject:    SubjectLookup{Kind: LookupName, Value: "Resolve"},
		Acceptance: fullAcceptance(),
	}, selector)
	if !errors.Is(err, context.Canceled) || result.Complete ||
		result.SelectorCalls != 1 || result.InferenceCalls != 1 || result.Subject.Symbol.SymbolID != "" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}
