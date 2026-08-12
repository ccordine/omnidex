package semanticreview

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestOversizedArtifactsFailBeforeReviewOrPromotion(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	t.Run("initial", func(t *testing.T) {
		content := bytes.Repeat([]byte{'x'}, 8<<20)
		if _, err := NewInitialArtifact(fixture.objective.ID, content); !errors.Is(err, ErrInvalidArtifact) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("correction", func(t *testing.T) {
		selector := &scriptedSelector{choices: []string{"C17"}}
		verifier := &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept}
		executor := &scriptedCorrectionExecutor{correct: func(string) string {
			return string(bytes.Repeat([]byte{'x'}, 8<<20))
		}}
		machine := newFixtureMachine(t, fixture, selector, verifier, executor, 2)
		result, err := machine.Run(context.Background())
		if !errors.Is(err, ErrInvalidArtifact) || result.Complete ||
			selector.calls != 1 || len(result.Reviews) != 1 ||
			result.CorrectionCalls != 1 || result.VerificationCalls != 1 || verifier.calls != 1 ||
			result.CurrentArtifact.ID != result.InitialArtifact.ID {
			t.Fatalf("result=%+v error=%v", result, err)
		}
	})
}
