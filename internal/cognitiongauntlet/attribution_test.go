package cognitiongauntlet

import (
	"math"
	"testing"
)

func TestFailureAttributionUsesDeterministicTraceConditions(t *testing.T) {
	tests := []struct {
		name  string
		trace FailureTrace
		want  FailureClass
	}{
		{name: "acquisition", trace: FailureTrace{NecessaryEvidence: true, RequiredEvidenceID: "evidence-required"}, want: FailureAcquisition},
		{name: "recording", trace: FailureTrace{NecessaryEvidence: true, Acquired: true, RequiredEvidenceID: "evidence-required", AcquisitionActionID: "action-search"}, want: FailureStateRecording},
		{name: "retention", trace: FailureTrace{NecessaryEvidence: true, Acquired: true, Recorded: true, ReleasedBeforeUse: true, EvidenceEntryID: "entry-1", ReleaseEventID: "release-1", DecisionID: "decision-1"}, want: FailureRetention},
		{name: "projection", trace: FailureTrace{NecessaryEvidence: true, Acquired: true, Recorded: true, ResidentAtDecision: true, EvidenceEntryID: "entry-1", ProjectionID: "projection-1", DecisionID: "decision-1"}, want: FailureProjection},
		{name: "policy", trace: FailureTrace{NecessaryEvidence: true, Acquired: true, Recorded: true, ResidentAtDecision: true, ProjectedAtDecision: true, ProjectionID: "projection-1", DecisionID: "decision-1"}, want: FailureModelPolicy},
		{name: "runtime", trace: FailureTrace{ActionSupported: true, ValidatorRejected: true, ActionID: "action-1", ValidatorEventID: "validator-1"}, want: FailureContractRuntime},
		{name: "stale", trace: FailureTrace{ObsoleteRevisionUsed: true, ActionID: "action-1"}, want: FailureStaleState},
		{name: "continuity", trace: FailureTrace{RestartMismatch: true, RestartEventID: "restart-1"}, want: FailureContinuity},
		{name: "completion", trace: FailureTrace{GoalPredicateTrue: true, TerminalRecorded: false, CompletionCheckID: "completion-check-1"}, want: FailureCompletion},
		{name: "policy rejection", trace: FailureTrace{PolicyRejected: true, PolicyFailureEventID: "cancellation-policy-1"}, want: FailureModelPolicy},
		{name: "budget", trace: FailureTrace{BudgetExhausted: true, BudgetEventID: "cancellation-budget-1"}, want: FailureResourceBudget},
		{name: "ambiguous", trace: FailureTrace{}, want: FailureUnattributed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AttributeFailure(test.trace)
			if err != nil {
				t.Fatal(err)
			}
			if got.Class != test.want {
				t.Fatalf("attribution=%q want=%q", got.Class, test.want)
			}
		})
	}
}

func TestFailureAttributionRejectsMissingTraceIdentity(t *testing.T) {
	if _, err := AttributeFailure(FailureTrace{NecessaryEvidence: true}); err == nil {
		t.Fatal("acquisition attribution without an exact evidence identity was accepted")
	}
}

func TestPromotionGatesRemainSeparateAndExact(t *testing.T) {
	absolute := EvaluateAbsoluteGate(AbsoluteGateInput{})
	if !absolute.Passed {
		t.Fatalf("zero-violation gate=%+v", absolute)
	}
	if EvaluateAbsoluteGate(AbsoluteGateInput{StaleWorkerWrites: 1}).Passed {
		t.Fatal("stale worker write passed architecture gate")
	}
	continuity := EvaluateContinuityGate(ContinuityGateInput{
		Episodes: 20, CorrectWorld: 20, CorrectLedger: 20, CorrectWorkingSet: 20,
		IdenticalProjection: 20,
	})
	if !continuity.Passed {
		t.Fatalf("continuity gate=%+v", continuity)
	}
	scale := EvaluateScaleGate(ScaleGateInput{WorldMultiplier: 100, ContextGrowth: 1.25, DecisionGrowth: 1.20, SuccessLossPoints: 5})
	if !scale.Passed {
		t.Fatalf("boundary scale gate=%+v", scale)
	}
	if EvaluateScaleGate(ScaleGateInput{WorldMultiplier: 100, ContextGrowth: 1.251, DecisionGrowth: 1.20, SuccessLossPoints: 5}).Passed {
		t.Fatal("over-bound context growth passed scale gate")
	}
	if EvaluateScaleGate(ScaleGateInput{WorldMultiplier: 100, ContextGrowth: math.NaN(), DecisionGrowth: 1, SuccessLossPoints: 0}).Passed {
		t.Fatal("non-finite scale metric passed")
	}
}

func TestPairedOutcomesDoNotTreatRepetitionsAsCases(t *testing.T) {
	summary, err := SummarizePaired([]PairedOutcome{
		{CaseID: "a", BaselineSuccess: true, CandidateSuccess: true},
		{CaseID: "b", BaselineSuccess: true, CandidateSuccess: false},
		{CaseID: "c", BaselineSuccess: false, CandidateSuccess: true},
		{CaseID: "d", BaselineSuccess: false, CandidateSuccess: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Cases != 4 || summary.Preserved != 1 || summary.Regressions != 1 || summary.Rescues != 1 || summary.Unresolved != 1 {
		t.Fatalf("paired summary=%+v", summary)
	}
	if _, err := SummarizePaired([]PairedOutcome{{CaseID: "a"}, {CaseID: "a"}}); err == nil {
		t.Fatal("duplicate case/repetition was treated as an independent case")
	}
}

func TestExperienceReadinessRequiresCompleteSealedAuthority(t *testing.T) {
	passed := EvaluateExperienceReadinessGate(ExperienceReadinessGateInput{
		Episodes: 2, CompleteImmutableTraces: 2, CompleteProjectionProofs: 2,
		KnownOutcomes: 2, KnownRuntimeVersions: 2, KnownModelVersions: 2,
		PostEvaluationArchetypes: 2,
	})
	if !passed.Passed {
		t.Fatalf("experience readiness=%+v", passed)
	}
	if EvaluateExperienceReadinessGate(ExperienceReadinessGateInput{
		Episodes: 1, CompleteImmutableTraces: 1, CompleteProjectionProofs: 1,
		KnownOutcomes: 1, KnownRuntimeVersions: 1, KnownModelVersions: 1,
		PostEvaluationArchetypes: 1, HiddenLabelExposures: 1,
	}).Passed {
		t.Fatal("hidden-label exposure passed experience readiness")
	}
}
