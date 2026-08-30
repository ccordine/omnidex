package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRepositoryGroundedClosureReturnsCodeValidatedAnswerWithoutReviewGate(t *testing.T) {
	t.Parallel()
	station := &recordingRepositoryAnswerStation{answer: assemblyline.GroundedAnswerDecision{
		Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: "requirement-17",
		Text: "First evidence owns the operation.", EvidenceIDs: []string{"R01"},
	}}
	result, err := runObjectiveRepositoryGroundedClosure(
		context.Background(), repositoryGroundedAnswerInput(), station,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer.Text != station.answer.Text ||
		strings.Join(result.Answer.EvidenceIDs, ",") != "R01" ||
		result.ModelCalls != 1 || station.calls != 1 {
		t.Fatalf("result=%#v station=%#v", result, station)
	}
}

func TestRepositoryGroundedClosureRejectsUnavailableCitationDeterministically(t *testing.T) {
	t.Parallel()
	station := &recordingRepositoryAnswerStation{answer: assemblyline.GroundedAnswerDecision{
		Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: "requirement-17",
		Text: "Unsupported answer.", EvidenceIDs: []string{"R99"},
	}}
	_, err := runObjectiveRepositoryGroundedClosure(
		context.Background(), repositoryGroundedAnswerInput(), station,
	)
	if err == nil || !strings.Contains(err.Error(), "was not projected") {
		t.Fatalf("error=%v", err)
	}
}

func TestRepositoryGroundedCallBudgetAllowsFullyRestoredQueue(t *testing.T) {
	t.Parallel()
	restored := objectiveStationReceipt{Reused: true}
	station := &recordingRepositoryAnswerStation{
		answer: assemblyline.GroundedAnswerDecision{
			Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: "requirement-17",
			Text: "First evidence owns the operation.", EvidenceIDs: []string{"R01"},
		},
		receipt: &restored,
	}
	result, err := runObjectiveRepositoryGroundedClosure(
		t.Context(), repositoryGroundedAnswerInput(), station,
	)
	if err != nil {
		t.Fatalf("fully restored grounded-answer queue was rejected: %v", err)
	}
	if result.ModelCalls != 0 || result.Answer.Text != station.answer.Text || station.calls != 1 {
		t.Fatalf("result=%+v station=%+v", result, station)
	}
}

func TestRepositoryGroundedCallBudgetRequiresReuseProofForZeroCalls(t *testing.T) {
	t.Parallel()
	for _, receipt := range []objectiveStationReceipt{
		{},
		{Calls: 1, Reused: true},
	} {
		if err := validateObjectiveGroundedAnswerReceipt(
			receipt, repositoryGroundedAnswerInput(),
		); err == nil {
			t.Fatalf("invalid aggregate receipt was accepted: %+v", receipt)
		}
	}
}

type recordingRepositoryAnswerStation struct {
	answer  assemblyline.GroundedAnswerDecision
	receipt *objectiveStationReceipt
	calls   int
}

func (station *recordingRepositoryAnswerStation) Answer(
	_ context.Context, _ assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	station.calls++
	receipt := objectiveStationReceipt{Calls: 1}
	if station.receipt != nil {
		receipt = *station.receipt
	}
	return station.answer, receipt, nil
}

func repositoryGroundedAnswerInput() assemblyline.GroundedAnswerInput {
	return assemblyline.GroundedAnswerInput{
		RequirementID: "requirement-17", ExactRequirement: "Which component owns the operation?",
		Evidence: []assemblyline.GroundedEvidenceCapsule{
			{ID: "R01", Text: "First evidence owns the operation."},
			{ID: "R02", Text: "Second evidence participates in the operation."},
		},
	}
}
