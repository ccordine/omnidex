package semanticreview

import (
	"context"
	"reflect"
	"testing"
)

func TestSemanticIssueSpawnsCorrectionAndFreshReview(t *testing.T) {
	for _, fixture := range semanticReviewFixtures() {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			selector := &scriptedSelector{choices: append([]string{}, fixture.choices...)}
			verifier := &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept}
			executor := &scriptedCorrectionExecutor{correct: fixture.correct}
			machine := newFixtureMachine(t, fixture, selector, verifier, executor, 3)

			result, err := machine.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !result.Complete || result.StationCalls != 2 || result.CorrectionCalls != 1 ||
				result.VerificationCalls != 2 {
				t.Fatalf("unexpected result: %+v", result)
			}
			if selector.calls != 2 || executor.calls != 1 || verifier.calls != 2 {
				t.Fatalf("calls selector=%d executor=%d verifier=%d", selector.calls, executor.calls, verifier.calls)
			}
			for _, input := range verifier.inputs {
				if !reflect.DeepEqual(input.ArtifactAcceptance, exactArtifactAcceptance) {
					t.Fatalf("verifier received non-artifact acceptance: %+v", input)
				}
			}
			if result.CurrentArtifact.Revision != result.InitialArtifact.Revision+1 ||
				result.CurrentArtifact.SHA256 == result.InitialArtifact.SHA256 {
				t.Fatalf("artifact authority did not advance: %+v", result)
			}
			if len(result.Reviews) != 2 || len(result.Findings) != 2 || len(result.Corrections) != 1 {
				t.Fatalf("topology reviews=%d findings=%d corrections=%d", len(result.Reviews), len(result.Findings), len(result.Corrections))
			}
			if result.Findings[0].Kind != FindingSemanticIssue || result.Findings[1].Kind != FindingNone {
				t.Fatalf("findings=%+v", result.Findings)
			}
			if result.Corrections[0].Status != ObjectiveComplete ||
				result.Objective.Status != ObjectiveComplete {
				t.Fatalf("objective status root=%q correction=%q", result.Objective.Status, result.Corrections[0].Status)
			}
			if result.Reviews[1].DependsOn[0] != result.Corrections[0].ID ||
				result.Corrections[0].DependsOn[0] != result.Reviews[0].ID {
				t.Fatalf("non-recursive topology: reviews=%+v corrections=%+v", result.Reviews, result.Corrections)
			}
			if reflect.DeepEqual(selector.gaps[0], selector.gaps[1]) ||
				selector.gaps[0].ID == selector.gaps[1].ID {
				t.Fatalf("review did not rebuild a fresh gap: %+v", selector.gaps)
			}
			assertGapCarriesOnlyCurrentArtifact(t, selector.gaps[0], result.InitialArtifact)
			assertGapCarriesOnlyCurrentArtifact(t, selector.gaps[1], result.CurrentArtifact)
			if gapContains(selector.gaps[1], string(result.InitialArtifact.Content)) {
				t.Fatal("fresh review replayed the superseded artifact")
			}
		})
	}
}

func TestTwoUnrelatedFixturesUseSameGenericRuntime(t *testing.T) {
	fixtures := semanticReviewFixtures()
	if len(fixtures) != 2 {
		t.Fatalf("fixture count=%d", len(fixtures))
	}
	for _, fixture := range fixtures {
		selector := &scriptedSelector{choices: append([]string{}, fixture.choices...)}
		machine := newFixtureMachine(
			t, fixture, selector, &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept},
			&scriptedCorrectionExecutor{correct: fixture.correct}, 3,
		)
		result, err := machine.Run(context.Background())
		if err != nil || !result.Complete {
			t.Fatalf("fixture %q result=%+v error=%v", fixture.name, result, err)
		}
	}
}
