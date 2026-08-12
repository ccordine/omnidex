package semanticreview

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func TestNoneCannotCompleteWithoutMatchingVerificationReceipt(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	selector := &scriptedSelector{choices: []string{"C99"}}
	verifier := &scriptedVerifier{err: errors.New("exact verification rejected artifact")}
	executor := &scriptedCorrectionExecutor{correct: fixture.correct}
	machine := newFixtureMachine(t, fixture, selector, verifier, executor, 2)
	result, err := machine.Run(context.Background())
	if err == nil || result.Complete || selector.calls != 0 || executor.calls != 0 || verifier.calls != 1 {
		t.Fatalf("result=%+v error=%v calls=%d/%d/%d", result, err, selector.calls, executor.calls, verifier.calls)
	}
}

func TestSelectorFailuresExecuteNoCorrection(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	tests := []struct {
		name     string
		selector *scriptedSelector
	}{
		{name: "station error", selector: &scriptedSelector{err: errors.New("station stopped")}},
		{name: "unknown candidate", selector: &scriptedSelector{choices: []string{"C404"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := &scriptedCorrectionExecutor{correct: fixture.correct}
			machine := newFixtureMachine(t, fixture, test.selector, &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept}, executor, 2)
			result, err := machine.Run(context.Background())
			if err == nil || result.Complete || executor.calls != 0 || len(result.Corrections) != 0 {
				t.Fatalf("result=%+v error=%v executor calls=%d", result, err, executor.calls)
			}
		})
	}
}

func TestCancellationAtEveryExternalBoundaryPreventsFurtherWork(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	t.Run("before run", func(t *testing.T) {
		selector := &scriptedSelector{choices: []string{"C99"}}
		verifier := &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept}
		executor := &scriptedCorrectionExecutor{correct: fixture.correct}
		machine := newFixtureMachine(t, fixture, selector, verifier, executor, 2)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := machine.Run(ctx)
		if !errors.Is(err, context.Canceled) || result.Complete || selector.calls != 0 || verifier.calls != 0 || executor.calls != 0 {
			t.Fatalf("result=%+v error=%v", result, err)
		}
	})
	t.Run("after selection", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		selector := &scriptedSelector{choices: []string{"C17"}, cancel: cancel}
		executor := &scriptedCorrectionExecutor{correct: fixture.correct}
		machine := newFixtureMachine(t, fixture, selector, &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept}, executor, 2)
		result, err := machine.Run(ctx)
		if !errors.Is(err, context.Canceled) || result.Complete || executor.calls != 0 || len(result.Corrections) != 0 {
			t.Fatalf("result=%+v error=%v", result, err)
		}
	})
	t.Run("after correction", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		selector := &scriptedSelector{choices: []string{"C17"}}
		executor := &scriptedCorrectionExecutor{correct: fixture.correct, cancel: cancel}
		verifier := &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept}
		machine := newFixtureMachine(t, fixture, selector, verifier, executor, 2)
		result, err := machine.Run(ctx)
		if !errors.Is(err, context.Canceled) || result.Complete || verifier.calls != 1 || result.CorrectionCalls != 1 {
			t.Fatalf("result=%+v error=%v verifier=%d", result, err, verifier.calls)
		}
	})
	t.Run("at terminal completion boundary", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		selector := &scriptedSelector{choices: []string{"C17", "C99"}, cancel: cancel, cancelAt: 2}
		executor := &scriptedCorrectionExecutor{correct: fixture.correct}
		verifier := &scriptedVerifier{
			structural: fixture.structural, acceptCorrection: fixture.accept,
		}
		machine := newFixtureMachine(t, fixture, selector, verifier, executor, 3)
		result, err := machine.Run(ctx)
		if !errors.Is(err, context.Canceled) || result.Complete ||
			result.Objective.Status == ObjectiveComplete || selector.calls != 2 {
			t.Fatalf("result=%+v error=%v selector=%d", result, err, selector.calls)
		}
	})
}

func TestIssueAtFinalRoundRecordsBoundBlockedObjectiveWithoutExecution(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	selector := &scriptedSelector{choices: []string{"C17"}}
	executor := &scriptedCorrectionExecutor{correct: fixture.correct}
	machine := newFixtureMachine(t, fixture, selector, &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept}, executor, 1)
	result, err := machine.Run(context.Background())
	if !errors.Is(err, ErrReviewRoundBound) || result.Complete || executor.calls != 0 || result.CorrectionCalls != 0 {
		t.Fatalf("result=%+v error=%v executor=%d", result, err, executor.calls)
	}
	if len(result.Corrections) != 1 || result.Corrections[0].Status != ObjectiveBoundBlocked {
		t.Fatalf("corrections=%+v", result.Corrections)
	}
	if result.CurrentArtifact.ID != result.InitialArtifact.ID {
		t.Fatal("round-bound failure mutated the current artifact")
	}
}

func TestCorrectionFailuresDoNotOpenFreshReviewOrFallback(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	tests := []struct {
		name     string
		executor *scriptedCorrectionExecutor
		verifier *scriptedVerifier
	}{
		{name: "executor", executor: &scriptedCorrectionExecutor{err: errors.New("correction stopped")}, verifier: &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept}},
		{name: "unchanged", executor: &scriptedCorrectionExecutor{}, verifier: &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept}},
		{name: "wrong but nonempty", executor: &scriptedCorrectionExecutor{correct: func(string) string { return "This is nonempty but still wrong." }}, verifier: &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept}},
		{name: "verification", executor: &scriptedCorrectionExecutor{correct: fixture.correct}, verifier: &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept, errAt: 2, err: errors.New("corrected artifact rejected")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selector := &scriptedSelector{choices: []string{"C17", "C99"}}
			machine := newFixtureMachine(t, fixture, selector, test.verifier, test.executor, 3)
			result, err := machine.Run(context.Background())
			if err == nil || result.Complete || selector.calls != 1 || len(result.Reviews) != 1 || len(result.Corrections) != 1 {
				t.Fatalf("result=%+v error=%v selector=%d", result, err, selector.calls)
			}
			if result.Corrections[0].Status != ObjectiveFailed {
				t.Fatalf("correction status=%q", result.Corrections[0].Status)
			}
		})
	}
}

func TestRegistriesAreExactAndFrozenBeforeSelection(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	specification := fixtureSpecification(fixture)
	validRule := CorrectionRule{FindingCode: fixture.issue, ObjectiveKind: fixture.kind, Acceptance: []CorrectionAcceptancePredicate{AcceptanceCorrectionArtifactVerified}}
	for _, test := range []struct {
		name  string
		rules []CorrectionRule
	}{
		{name: "missing", rules: nil},
		{name: "duplicate", rules: []CorrectionRule{validRule, validRule}},
		{name: "none rule", rules: []CorrectionRule{validRule, {FindingCode: FindingCodeNone, ObjectiveKind: "C_invalid", Acceptance: []CorrectionAcceptancePredicate{AcceptanceCorrectionArtifactVerified}}}},
		{name: "extra", rules: []CorrectionRule{validRule, {FindingCode: "F_unknown", ObjectiveKind: "C_unknown", Acceptance: []CorrectionAcceptancePredicate{AcceptanceCorrectionArtifactVerified}}}},
	} {
		t.Run("rules "+test.name, func(t *testing.T) {
			if _, err := NewCorrectionRuleRegistry(specification, test.rules); err == nil {
				t.Fatal("invalid rule registry was accepted")
			}
		})
	}
	rules, err := NewCorrectionRuleRegistry(specification, []CorrectionRule{validRule})
	if err != nil {
		t.Fatal(err)
	}
	var nilExecutor *scriptedCorrectionExecutor
	for _, test := range []struct {
		name          string
		registrations []CorrectionExecutorRegistration
	}{
		{name: "missing", registrations: nil},
		{name: "typed nil", registrations: []CorrectionExecutorRegistration{{ObjectiveKind: fixture.kind, Executor: nilExecutor}}},
		{name: "duplicate", registrations: []CorrectionExecutorRegistration{{ObjectiveKind: fixture.kind, Executor: &scriptedCorrectionExecutor{}}, {ObjectiveKind: fixture.kind, Executor: &scriptedCorrectionExecutor{}}}},
		{name: "extra", registrations: []CorrectionExecutorRegistration{{ObjectiveKind: fixture.kind, Executor: &scriptedCorrectionExecutor{}}, {ObjectiveKind: "C_unknown", Executor: &scriptedCorrectionExecutor{}}}},
	} {
		t.Run("executors "+test.name, func(t *testing.T) {
			if _, err := NewCorrectionExecutorRegistry(rules, test.registrations); err == nil {
				t.Fatal("invalid executor registry was accepted")
			}
		})
	}
}

func TestMachineRejectsTypedNilPortsInvalidBoundsAndMismatchedAuthority(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	initial, err := NewInitialArtifact(fixture.objective.ID, []byte(fixture.artifact))
	if err != nil {
		t.Fatal(err)
	}
	specification := fixtureSpecification(fixture)
	rules, _ := NewCorrectionRuleRegistry(specification, []CorrectionRule{{FindingCode: fixture.issue, ObjectiveKind: fixture.kind, Acceptance: []CorrectionAcceptancePredicate{AcceptanceCorrectionArtifactVerified}}})
	executors, _ := NewCorrectionExecutorRegistry(rules, []CorrectionExecutorRegistration{{ObjectiveKind: fixture.kind, Executor: &scriptedCorrectionExecutor{correct: fixture.correct}}})
	var nilSelector *scriptedSelector
	var nilVerifier *scriptedVerifier
	for _, test := range []struct {
		name      string
		objective Objective
		artifact  Artifact
		selector  cognitionreference.Selector
		verifier  Verifier
		limits    Limits
	}{
		{name: "typed nil selector", objective: fixture.objective, artifact: initial, selector: nilSelector, verifier: &scriptedVerifier{}, limits: Limits{MaxReviewRounds: 2}},
		{name: "typed nil verifier", objective: fixture.objective, artifact: initial, selector: &scriptedSelector{}, verifier: nilVerifier, limits: Limits{MaxReviewRounds: 2}},
		{name: "zero bound", objective: fixture.objective, artifact: initial, selector: &scriptedSelector{}, verifier: &scriptedVerifier{}, limits: Limits{}},
		{name: "oversized bound", objective: fixture.objective, artifact: initial, selector: &scriptedSelector{}, verifier: &scriptedVerifier{}, limits: Limits{MaxReviewRounds: 9}},
		{name: "objective mismatch", objective: Objective{ID: "O_other", Acceptance: requiredRootAcceptance(), Status: ObjectivePending}, artifact: initial, selector: &scriptedSelector{}, verifier: &scriptedVerifier{}, limits: Limits{MaxReviewRounds: 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewMachine(test.objective, test.artifact, specification, rules, executors, test.selector, test.verifier, test.limits); err == nil {
				t.Fatal("invalid machine was accepted")
			}
		})
	}
}

func TestCallerAndPortsCannotMutateFrozenAuthority(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	selector := &scriptedSelector{choices: []string{"C17", "C99"}, mutate: true}
	verifier := &scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept, mutate: true}
	executor := &scriptedCorrectionExecutor{correct: fixture.correct, mutate: true}

	objective := fixture.objective
	initial, err := NewInitialArtifact(objective.ID, []byte(fixture.artifact))
	if err != nil {
		t.Fatal(err)
	}
	specification := fixtureSpecification(fixture)
	ruleSlice := []CorrectionRule{{FindingCode: fixture.issue, ObjectiveKind: fixture.kind, Acceptance: []CorrectionAcceptancePredicate{AcceptanceCorrectionArtifactVerified}}}
	rules, err := NewCorrectionRuleRegistry(specification, ruleSlice)
	if err != nil {
		t.Fatal(err)
	}
	executorSlice := []CorrectionExecutorRegistration{{ObjectiveKind: fixture.kind, Executor: executor}}
	executors, err := NewCorrectionExecutorRegistry(rules, executorSlice)
	if err != nil {
		t.Fatal(err)
	}
	machine, err := NewMachine(objective, initial, specification, rules, executors, selector, verifier, Limits{MaxReviewRounds: 3})
	if err != nil {
		t.Fatal(err)
	}

	objective.Acceptance[0] = "mutated"
	initial.Content[0] ^= 0xff
	specification.Evidence[0].Content = "mutated"
	specification.Candidates[0].Summary = "mutated"
	ruleSlice[0].Acceptance[0] = "mutated"
	executorSlice[0].ObjectiveKind = "mutated"

	result, err := machine.Run(context.Background())
	if err != nil || !result.Complete {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	if !reflect.DeepEqual(result.Objective.Acceptance, requiredRootAcceptance()) ||
		string(result.InitialArtifact.Content) != fixture.artifact ||
		result.Corrections[0].Acceptance[0] != AcceptanceCorrectionArtifactVerified {
		t.Fatalf("frozen authority mutated: %+v", result)
	}
}

func fixtureSpecification(fixture semanticReviewFixture) ReviewSpecification {
	return ReviewSpecification{
		ID: "RS_" + ReviewSpecificationID(fixture.objective.ID), ObjectiveID: fixture.objective.ID,
		Question: "Does the current bounded result contradict the exact requirement?",
		Evidence: []EvidenceDefinition{{ID: "E01", Kind: EvidenceFixed, Content: fixture.requirement}, {ID: "E02", Kind: EvidenceCurrentArtifact}},
		Candidates: []FindingDefinition{
			{CandidateID: "C17", FindingCode: fixture.issue, Kind: FindingSemanticIssue, Summary: "The current result contradicts the requirement.", EvidenceIDs: []cognitionreference.EvidenceID{"E01", "E02"}},
			{CandidateID: "C99", FindingCode: FindingCodeNone, Kind: FindingNone, Summary: "No semantic contradiction is present.", EvidenceIDs: []cognitionreference.EvidenceID{"E01", "E02"}},
		},
	}
}
