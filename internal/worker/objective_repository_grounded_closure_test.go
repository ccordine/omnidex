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

type recordingRepositoryAnswerStation struct {
	answer assemblyline.GroundedAnswerDecision
	calls  int
}

func (station *recordingRepositoryAnswerStation) Answer(
	_ context.Context, _ assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	station.calls++
	return station.answer, objectiveStationReceipt{Calls: 1}, nil
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
