package semanticreview

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func TestCompletionRejectsStaleOrModelShapedAuthority(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	machine := newFixtureMachine(
		t, fixture, &scriptedSelector{choices: []string{"C17", "C99"}},
		&scriptedVerifier{structural: fixture.structural, acceptCorrection: fixture.accept},
		&scriptedCorrectionExecutor{correct: fixture.correct}, 3,
	)
	accepted, err := machine.Run(context.Background())
	if err != nil || !accepted.Complete {
		t.Fatalf("accepted result=%+v error=%v", accepted, err)
	}
	tests := []struct {
		name   string
		mutate func(*Result)
	}{
		{name: "stale receipt", mutate: func(result *Result) {
			result.VerificationReceipts[len(result.VerificationReceipts)-1].ArtifactSHA256 = result.InitialArtifact.SHA256
		}},
		{name: "none for old artifact", mutate: func(result *Result) { result.Findings[len(result.Findings)-1].ArtifactID = result.InitialArtifact.ID }},
		{name: "open correction", mutate: func(result *Result) { result.Corrections[0].Status = ObjectivePending }},
		{name: "missing dependency", mutate: func(result *Result) { result.Reviews[1].DependsOn = nil }},
		{name: "skipped round", mutate: func(result *Result) { result.Reviews[1].Round = 3 }},
		{name: "unverified output", mutate: func(result *Result) { result.Corrections[0].OutputArtifactID = "A_fabricated" }},
		{name: "receipt overclaims semantic acceptance", mutate: func(result *Result) {
			result.VerificationReceipts[0].ArtifactAcceptance = append(
				result.VerificationReceipts[0].ArtifactAcceptance,
				AcceptanceNoOpenSemanticFinding,
			)
			result.VerificationReceipts[0].ID = verificationReceiptIdentity(result.VerificationReceipts[0])
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := cloneResultForTest(accepted)
			result.Objective.Status = ObjectivePending
			result.Complete = false
			test.mutate(&result)
			if err := completeResult(&result, machine.specification, machine.rules); err == nil || result.Complete {
				t.Fatalf("invalid completion accepted: %+v", result)
			}
		})
	}
}

func TestCompletionRejectsMutuallyRewrittenIntermediateArtifactAuthority(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	selector := &scriptedSelector{choices: []string{"C17", "C17", "C99"}}
	executor := &sequenceCorrectionExecutor{outputs: []string{
		"Failures are retried after a delay, pending confirmation.",
		"Failures are retried after a delay.",
	}}
	verifier := &scriptedVerifier{
		structural: fixture.structural,
		acceptCorrection: func(value string) bool {
			return value == executor.outputs[0] || value == executor.outputs[1]
		},
	}
	machine := newFixtureMachine(t, fixture, selector, verifier, executor, 4)
	accepted, err := machine.Run(context.Background())
	if err != nil || !accepted.Complete || len(accepted.Corrections) != 2 {
		t.Fatalf("accepted=%+v error=%v", accepted, err)
	}
	result := cloneResultForTest(accepted)
	result.Objective.Status = ObjectivePending
	result.Complete = false
	forged := ArtifactID("A_forged_intermediate")
	result.Corrections[0].OutputArtifactID = forged
	result.VerificationReceipts[1].ArtifactID = forged
	result.VerificationReceipts[1].ID = verificationReceiptIdentity(result.VerificationReceipts[1])
	result.Reviews[1].ArtifactID = forged
	result.Reviews[1].ID = reviewIdentity(
		result.Objective.ID, result.Corrections[0].ID, forged,
		result.Reviews[1].ArtifactSHA256, result.Reviews[1].Round, machine.specification,
	)
	result.Reviews[1].GapID = reviewGapIdentity(
		result.Reviews[1].ID, forged, result.Reviews[1].ArtifactSHA256,
	)
	result.Findings[1].ReviewObjectiveID = result.Reviews[1].ID
	result.Findings[1].GapID = result.Reviews[1].GapID
	result.Findings[1].ArtifactID = forged
	result.Findings[1].ID = findingIdentity(result.Findings[1])
	result.Corrections[1].ParentID = result.Reviews[1].ID
	result.Corrections[1].DependsOn = []cognitionreference.ObjectiveID{result.Reviews[1].ID}
	result.Corrections[1].Finding = cloneFinding(result.Findings[1])
	result.Corrections[1].InputArtifactID = forged
	result.Corrections[1].ID = correctionIdentity(result.Corrections[1])
	result.VerificationReceipts[2].CorrectionObjectiveID = result.Corrections[1].ID
	result.VerificationReceipts[2].ID = verificationReceiptIdentity(result.VerificationReceipts[2])
	result.Reviews[2].ParentID = result.Corrections[1].ID
	result.Reviews[2].DependsOn = []cognitionreference.ObjectiveID{result.Corrections[1].ID}
	if err := completeResult(&result, machine.specification, machine.rules); err == nil || result.Complete {
		t.Fatal("mutually rewritten intermediate artifact authority was accepted")
	}
}

type sequenceCorrectionExecutor struct {
	outputs []string
	calls   int
}

func (executor *sequenceCorrectionExecutor) Execute(
	_ context.Context,
	_ CorrectionObjective,
	_ Artifact,
) (ArtifactValue, error) {
	if executor.calls >= len(executor.outputs) {
		return ArtifactValue{}, errors.New("unexpected correction call")
	}
	output := executor.outputs[executor.calls]
	executor.calls++
	return ArtifactValue{Content: []byte(output)}, nil
}

func cloneResultForTest(value Result) Result {
	value.Objective = cloneObjective(value.Objective)
	value.InitialArtifact = cloneArtifact(value.InitialArtifact)
	value.CurrentArtifact = cloneArtifact(value.CurrentArtifact)
	value.Reviews = append([]ReviewObjective{}, value.Reviews...)
	for index := range value.Reviews {
		value.Reviews[index] = cloneReviewObjective(value.Reviews[index])
	}
	value.Findings = append([]ReviewFinding{}, value.Findings...)
	for index := range value.Findings {
		value.Findings[index] = cloneFinding(value.Findings[index])
	}
	value.Corrections = append([]CorrectionObjective{}, value.Corrections...)
	for index := range value.Corrections {
		value.Corrections[index] = cloneCorrectionObjective(value.Corrections[index])
	}
	value.VerificationReceipts = append([]VerificationReceipt{}, value.VerificationReceipts...)
	for index := range value.VerificationReceipts {
		value.VerificationReceipts[index] = cloneVerificationReceipt(value.VerificationReceipts[index])
	}
	return value
}
