package worker

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRepositoryGroundedClosureRequiresIndependentReviewBeforeReturningAnswer(t *testing.T) {
	t.Parallel()
	stations := &recordingRepositoryGroundingStation{
		answer: assemblyline.GroundedAnswerDecision{
			Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: "requirement-17",
			Text: "First evidence owns the operation.", EvidenceIDs: []string{"R01"},
		},
		reviews: []assemblyline.RepositoryGroundedReviewDecision{{
			Schema:  assemblyline.RepositoryGroundedReviewSchemaV1,
			Outcome: assemblyline.RepositoryGroundedReviewNone,
		}},
	}
	result, err := runObjectiveRepositoryGroundedClosure(
		context.Background(), repositoryGroundedAnswerInput(), stations,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer.Text != stations.answer.Text || result.ModelCalls != 2 ||
		result.ReviewCalls != 1 || result.CorrectionCalls != 0 {
		t.Fatalf("result=%#v", result)
	}
	if strings.Join(stations.events, ",") != "answer,review" || len(stations.reviewInputs) != 1 {
		t.Fatalf("events=%v reviews=%#v", stations.events, stations.reviewInputs)
	}
	review := stations.reviewInputs[0]
	if len(review.Evidence) != 1 || review.Evidence[0].ID != "R01" || len(review.EvidenceIDs) != 1 {
		t.Fatalf("independent review received evidence beyond exact citations: %#v", review)
	}
}

func TestRepositoryGroundedClosureCorrectsOneTextLeafThenReReviews(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedAnswerInput()
	input.ExactRequirement = "  Which component owns the operation?  \n"
	contextText := "The earlier question and result identified the operation as First."
	input.Context.Capsules = []assemblyline.ObjectiveContextCapsule{{
		Sources: []assemblyline.ObjectiveContextSource{{
			Namespace: "conversation_exchange", CandidateID: "CTX_1",
			ContentSHA256: assemblyline.ExactObjectiveContextSHA("exact prior exchange"),
		}},
		Content: contextText, ContentSHA256: assemblyline.ExactObjectiveContextSHA(contextText),
	}}
	stations := &recordingRepositoryGroundingStation{
		answer: assemblyline.GroundedAnswerDecision{
			Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: "requirement-17",
			Text: "A guessed owner runs it.", EvidenceIDs: []string{"R02", "R01"},
		},
		reviews: []assemblyline.RepositoryGroundedReviewDecision{
			{Schema: assemblyline.RepositoryGroundedReviewSchemaV1, Outcome: assemblyline.RepositoryGroundedReviewIssue,
				IssueKind: assemblyline.RepositoryGroundedUnsupportedClaim, Detail: "The guessed owner is unsupported."},
			{Schema: assemblyline.RepositoryGroundedReviewSchemaV1, Outcome: assemblyline.RepositoryGroundedReviewNone},
		},
		correction: assemblyline.RepositoryGroundedCorrectionDecision{Text: "The evidence identifies First and Second."},
	}
	result, err := runObjectiveRepositoryGroundedClosure(
		context.Background(), input, stations,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer.Text != stations.correction.Text ||
		strings.Join(result.Answer.EvidenceIDs, ",") != "R02,R01" ||
		result.ModelCalls != 4 || result.ReviewCalls != 2 || result.CorrectionCalls != 1 {
		t.Fatalf("result=%#v", result)
	}
	if strings.Join(stations.events, ",") != "answer,review,correct,review" {
		t.Fatalf("events=%v", stations.events)
	}
	if len(stations.correctionInputs) != 1 ||
		strings.Join(stations.correctionInputs[0].EvidenceIDs, ",") != "R02,R01" {
		t.Fatalf("correction lost code-owned evidence IDs: %#v", stations.correctionInputs)
	}
	if len(stations.reviewInputs) != 2 || stations.reviewInputs[1].AnswerText != stations.correction.Text {
		t.Fatalf("corrected text was not independently re-reviewed: %#v", stations.reviewInputs)
	}
	if stations.correctionInputs[0].ExactRequirement != input.ExactRequirement ||
		stations.reviewInputs[0].ExactRequirement != input.ExactRequirement ||
		stations.reviewInputs[1].ExactRequirement != input.ExactRequirement {
		t.Fatalf("exact requirement was rewritten across closure: correction=%q reviews=%#v",
			stations.correctionInputs[0].ExactRequirement, stations.reviewInputs)
	}
	if !reflect.DeepEqual(stations.correctionInputs[0].Context.Capsules, input.Context.Capsules) ||
		!reflect.DeepEqual(stations.reviewInputs[0].Context.Capsules, input.Context.Capsules) ||
		!reflect.DeepEqual(stations.reviewInputs[1].Context.Capsules, input.Context.Capsules) {
		t.Fatalf("minified context capsule was lost or rewritten: correction=%#v reviews=%#v",
			stations.correctionInputs[0], stations.reviewInputs)
	}
}

func TestRepositoryGroundedClosureFailsAfterSecondIssueWithoutAnotherCorrection(t *testing.T) {
	t.Parallel()
	issue := assemblyline.RepositoryGroundedReviewDecision{
		Schema:    assemblyline.RepositoryGroundedReviewSchemaV1,
		Outcome:   assemblyline.RepositoryGroundedReviewIssue,
		IssueKind: assemblyline.RepositoryGroundedUnsupportedClaim, Detail: "Still unsupported.",
	}
	stations := &recordingRepositoryGroundingStation{
		answer: assemblyline.GroundedAnswerDecision{
			Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: "requirement-17",
			Text: "Unproven answer.", EvidenceIDs: []string{"R01"},
		},
		reviews:    []assemblyline.RepositoryGroundedReviewDecision{issue, issue},
		correction: assemblyline.RepositoryGroundedCorrectionDecision{Text: "Changed but unsupported answer."},
	}
	result, err := runObjectiveRepositoryGroundedClosure(
		context.Background(), repositoryGroundedAnswerInput(), stations,
	)
	if err == nil || !strings.Contains(err.Error(), "remained unsupported") ||
		result.CorrectionCalls != 1 || len(stations.correctionInputs) != 1 || len(stations.reviewInputs) != 2 {
		t.Fatalf("error=%v result=%#v station=%#v", err, result, stations)
	}
}

func TestRepositoryGroundedClosureRejectsNoopCorrection(t *testing.T) {
	t.Parallel()
	stations := &recordingRepositoryGroundingStation{
		answer: assemblyline.GroundedAnswerDecision{
			Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: "requirement-17",
			Text: "Unproven answer.", EvidenceIDs: []string{"R01"},
		},
		reviews: []assemblyline.RepositoryGroundedReviewDecision{{
			Schema:    assemblyline.RepositoryGroundedReviewSchemaV1,
			Outcome:   assemblyline.RepositoryGroundedReviewIssue,
			IssueKind: assemblyline.RepositoryGroundedUnsupportedClaim, Detail: "Unsupported.",
		}},
		correction: assemblyline.RepositoryGroundedCorrectionDecision{Text: "Unproven answer."},
	}
	_, err := runObjectiveRepositoryGroundedClosure(
		context.Background(), repositoryGroundedAnswerInput(), stations,
	)
	if err == nil || !strings.Contains(err.Error(), "must change exactly the text leaf") {
		t.Fatalf("error=%v", err)
	}
}

type recordingRepositoryGroundingStation struct {
	answer           assemblyline.GroundedAnswerDecision
	reviews          []assemblyline.RepositoryGroundedReviewDecision
	correction       assemblyline.RepositoryGroundedCorrectionDecision
	events           []string
	reviewInputs     []assemblyline.RepositoryGroundedReviewInput
	correctionInputs []assemblyline.RepositoryGroundedCorrectionInput
}

func (station *recordingRepositoryGroundingStation) Answer(
	_ context.Context, _ assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	station.events = append(station.events, "answer")
	return station.answer, objectiveStationReceipt{Calls: 1}, nil
}

func (station *recordingRepositoryGroundingStation) Review(
	_ context.Context, input assemblyline.RepositoryGroundedReviewInput,
) (assemblyline.RepositoryGroundedReviewDecision, objectiveStationReceipt, error) {
	station.events = append(station.events, "review")
	station.reviewInputs = append(station.reviewInputs, cloneRepositoryReviewInput(input))
	decision := station.reviews[len(station.reviewInputs)-1]
	return decision, objectiveStationReceipt{Calls: 1}, nil
}

func (station *recordingRepositoryGroundingStation) Correct(
	_ context.Context, input assemblyline.RepositoryGroundedCorrectionInput,
) (assemblyline.RepositoryGroundedCorrectionDecision, objectiveStationReceipt, error) {
	station.events = append(station.events, "correct")
	station.correctionInputs = append(station.correctionInputs, cloneRepositoryCorrectionInput(input))
	return station.correction, objectiveStationReceipt{Calls: 1}, nil
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
