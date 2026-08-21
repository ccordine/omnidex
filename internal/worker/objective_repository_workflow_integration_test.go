package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

type workflowRepositoryGroundingStation struct {
	events       []string
	reviewCalls  int
	answerInput  assemblyline.GroundedAnswerInput
	reviewInputs []assemblyline.RepositoryGroundedReviewInput
}

func (station *workflowRepositoryGroundingStation) Answer(
	_ context.Context,
	input assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	station.events = append(station.events, "answer")
	station.answerInput = cloneGroundedAnswerInput(input)
	return assemblyline.GroundedAnswerDecision{
		Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: input.RequirementID,
		Text: "A guessed owner handles dispatch.", EvidenceIDs: []string{"R01"},
	}, objectiveStationReceipt{Calls: 1}, nil
}

func (station *workflowRepositoryGroundingStation) Review(
	_ context.Context,
	input assemblyline.RepositoryGroundedReviewInput,
) (assemblyline.RepositoryGroundedReviewDecision, objectiveStationReceipt, error) {
	station.events = append(station.events, "review")
	station.reviewInputs = append(station.reviewInputs, cloneRepositoryReviewInput(input))
	station.reviewCalls++
	if station.reviewCalls == 1 {
		return assemblyline.RepositoryGroundedReviewDecision{
			Schema:    assemblyline.RepositoryGroundedReviewSchemaV1,
			Outcome:   assemblyline.RepositoryGroundedReviewIssue,
			IssueKind: assemblyline.RepositoryGroundedUnsupportedClaim,
			Detail:    "The guessed ownership claim is unsupported.",
		}, objectiveStationReceipt{Calls: 1}, nil
	}
	return assemblyline.RepositoryGroundedReviewDecision{
		Schema:  assemblyline.RepositoryGroundedReviewSchemaV1,
		Outcome: assemblyline.RepositoryGroundedReviewNone,
	}, objectiveStationReceipt{Calls: 1}, nil
}

func (station *workflowRepositoryGroundingStation) Correct(
	_ context.Context,
	_ assemblyline.RepositoryGroundedCorrectionInput,
) (assemblyline.RepositoryGroundedCorrectionDecision, objectiveStationReceipt, error) {
	station.events = append(station.events, "correct")
	return assemblyline.RepositoryGroundedCorrectionDecision{
		Text: "DispatchOwner handles dispatch.",
	}, objectiveStationReceipt{Calls: 1}, nil
}

func TestObjectiveTurnProductionRepositoryPathConsumesReviewCorrectionAndReReview(t *testing.T) {
	t.Parallel()
	stations := &workflowRepositoryGroundingStation{}
	evidence := mustObjectiveEvidence(
		t, "R01", "type DispatchOwner struct{}", "repository_symbol", "pack#symbol",
	)
	result, err := runObjectiveTurn(
		context.Background(),
		model.Job{ID: 901, Pipeline: model.PipelineChat, Instruction: "Which type owns dispatch?", Metadata: objectiveAssistantMetadata()},
		scriptedConversationCandidateProvider{},
		emptyContextSieveStation(),
		&scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
			Schema: assemblyline.ConversationObjectiveKindSchemaV1,
			Kind:   assemblyline.ObjectiveKindRepositoryRead,
		}},
		&scriptedObjectiveConversationStation{},
		stations,
		objectiveWorkflows{RepositoryRead: func(context.Context, turnAuthority) (objectiveEvidenceAcquisition, error) {
			return objectiveRepositoryTestAcquisition([]objectiveEvidence{evidence}, 5), nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(stations.events, ",") != "answer,review,correct,review" {
		t.Fatalf("repository production station sequence=%v", stations.events)
	}
	if !result.Complete || result.Output != "DispatchOwner handles dispatch." || result.ModelCalls != 11 {
		t.Fatalf("repository production result=%#v", result)
	}
	if len(result.Citations) != 1 || result.Citations[0] != evidence || len(stations.reviewInputs) != 2 {
		t.Fatalf("repository production evidence/reviews result=%#v reviews=%#v", result, stations.reviewInputs)
	}
}

func TestObjectiveTurnRepositoryPathRejectsAnswerOnlyStation(t *testing.T) {
	t.Parallel()
	evidence := mustObjectiveEvidence(t, "R01", "bounded", "repository_symbol", "pack#symbol")
	_, err := runObjectiveTurn(
		context.Background(),
		model.Job{ID: 902, Pipeline: model.PipelineChat, Instruction: "Explain the owner.", Metadata: objectiveAssistantMetadata()},
		scriptedConversationCandidateProvider{}, emptyContextSieveStation(),
		&scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
			Schema: assemblyline.ConversationObjectiveKindSchemaV1,
			Kind:   assemblyline.ObjectiveKindRepositoryRead,
		}},
		&scriptedObjectiveConversationStation{},
		&answerOnlyRepositoryStation{},
		objectiveWorkflows{RepositoryRead: func(context.Context, turnAuthority) (objectiveEvidenceAcquisition, error) {
			return objectiveRepositoryTestAcquisition([]objectiveEvidence{evidence}, 1), nil
		}},
	)
	if err == nil || !strings.Contains(err.Error(), "independent review") {
		t.Fatalf("answer-only repository station error=%v", err)
	}
}

func TestObjectiveTurnRelationshipQueryRetainsRelationThroughReviewAndCitation(t *testing.T) {
	t.Parallel()
	relation := mustObjectiveEvidence(
		t,
		"R01",
		"Caller calls Callee\nfrom_id=caller\nto_id=callee\norigin=go_types\nconfidence=1",
		"repository_relation",
		"pack#relation",
	)
	stations := &workflowRepositoryGroundingStation{}
	result, err := runObjectiveTurn(
		context.Background(),
		model.Job{ID: 903, Pipeline: model.PipelineChat, Instruction: "How does Caller reach Callee?", Metadata: objectiveAssistantMetadata()},
		scriptedConversationCandidateProvider{}, emptyContextSieveStation(),
		&scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
			Schema: assemblyline.ConversationObjectiveKindSchemaV1,
			Kind:   assemblyline.ObjectiveKindRepositoryRead,
		}},
		&scriptedObjectiveConversationStation{}, stations,
		objectiveWorkflows{RepositoryRead: func(context.Context, turnAuthority) (objectiveEvidenceAcquisition, error) {
			return objectiveRepositoryTestAcquisition([]objectiveEvidence{relation}, 1), nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(stations.reviewInputs) != 2 || len(stations.reviewInputs[0].Evidence) != 1 ||
		stations.reviewInputs[0].Evidence[0].Text != relation.Capsule.Text ||
		len(result.Citations) != 1 || result.Citations[0] != relation {
		t.Fatalf("relation lineage was lost: reviews=%#v result=%#v", stations.reviewInputs, result)
	}
}

func TestRelationshipCitationPersistsAndRendersExactRelationProvenance(t *testing.T) {
	t.Parallel()
	relation := mustObjectiveEvidence(
		t,
		"R01",
		"Caller calls Callee\nfrom_id=caller\nto_id=callee\norigin=go_types\nconfidence=1",
		"repository_relation",
		"pack#relation",
	)
	result := objectiveTurnResult{
		ObjectiveID: "objective-relation", RequirementID: "requirement-relation",
		InstructionSHA256: strings.Repeat("a", 64), Kind: assemblyline.ObjectiveKindRepositoryRead,
		Output:    "Caller reaches Callee through a calls relation.",
		Citations: []objectiveEvidence{relation}, Complete: true,
	}
	rendered, records, err := prepareObjectiveTurnCompletion(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].SourceType != "repository_relation" ||
		records[0].SourceRef != "pack#relation" || records[0].Hash != relation.SourceSHA256 ||
		records[0].Excerpt != relation.Capsule.Text {
		t.Fatalf("persisted relation record=%#v", records)
	}
	wantSource := "repository_relation:pack#relation (source_sha256:" + relation.SourceSHA256 + ")"
	if !strings.Contains(rendered, wantSource) {
		t.Fatalf("rendered relation citation=%q want %q", rendered, wantSource)
	}
}

type answerOnlyRepositoryStation struct{}

func objectiveRepositoryTestAcquisition(
	evidence []objectiveEvidence,
	modelCalls int,
) objectiveEvidenceAcquisition {
	ledger := objectiveRepositoryAcquisitionCallLedger{relevanceCalls: []int{modelCalls}}
	if modelCalls > maxTypedWorkerAttempts {
		ledger = objectiveRepositoryAcquisitionCallLedger{
			searchTermCalls: modelCalls - maxTypedWorkerAttempts,
			relevanceCalls:  []int{maxTypedWorkerAttempts},
		}
	}
	return objectiveEvidenceAcquisition{
		Evidence: evidence, ModelCalls: modelCalls, RepositoryCallLedger: ledger,
	}
}

func (*answerOnlyRepositoryStation) Answer(
	context.Context,
	assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	return assemblyline.GroundedAnswerDecision{}, objectiveStationReceipt{Calls: 1}, nil
}
