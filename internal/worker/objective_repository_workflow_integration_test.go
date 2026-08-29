package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

type workflowRepositoryGroundingStation struct {
	events      []string
	answerInput assemblyline.GroundedAnswerInput
}

type artifactTokenRepositoryGroundingStation struct{}

func (*artifactTokenRepositoryGroundingStation) Answer(
	_ context.Context,
	input assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	return assemblyline.GroundedAnswerDecision{
		Schema:        assemblyline.GroundedAnswerSchemaV1,
		RequirementID: input.RequirementID,
		Text:          "ARTIFACT_1 owns dispatch.",
		EvidenceIDs:   []string{"R01"},
	}, objectiveStationReceipt{Calls: 1}, nil
}

func (station *workflowRepositoryGroundingStation) Answer(
	_ context.Context,
	input assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	station.events = append(station.events, "answer")
	station.answerInput = cloneGroundedAnswerInput(input)
	return assemblyline.GroundedAnswerDecision{
		Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: input.RequirementID,
		Text: "DispatchOwner handles dispatch.", EvidenceIDs: []string{"R01"},
	}, objectiveStationReceipt{Calls: 1}, nil
}

func TestObjectiveTurnProductionRepositoryPathUsesCodeValidatedGroundedAnswer(t *testing.T) {
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
			return objectiveRepositoryTestAcquisition([]objectiveEvidence{evidence}, 1), nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(stations.events, ",") != "answer" {
		t.Fatalf("repository production station sequence=%v", stations.events)
	}
	if !result.Complete || result.Output != "DispatchOwner handles dispatch." || result.ModelCalls != 3 {
		t.Fatalf("repository production result=%#v", result)
	}
	if len(result.Citations) != 1 || result.Citations[0] != evidence {
		t.Fatalf("repository production evidence result=%#v", result)
	}
}

func TestObjectiveTurnRestoresArtifactIdentityOnlyAfterGroundedAcceptance(t *testing.T) {
	t.Parallel()
	evidence := mustObjectiveEvidence(
		t, "R01", "DispatchOwner owns dispatch.", "repository_symbol", "pack#symbol",
	)
	result, err := runObjectiveTurn(
		context.Background(),
		model.Job{ID: 904, Pipeline: model.PipelineChat, Instruction: "Which component owns dispatch?", Metadata: objectiveAssistantMetadata()},
		scriptedConversationCandidateProvider{}, emptyContextSieveStation(),
		&scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
			Schema: assemblyline.ConversationObjectiveKindSchemaV1,
			Kind:   assemblyline.ObjectiveKindRepositoryRead,
		}},
		&scriptedObjectiveConversationStation{},
		&artifactTokenRepositoryGroundingStation{},
		objectiveWorkflows{RepositoryRead: func(context.Context, turnAuthority) (objectiveEvidenceAcquisition, error) {
			return objectiveEvidenceAcquisition{
				Evidence:             []objectiveEvidence{evidence},
				ModelCalls:           1,
				RepositoryCallLedger: objectiveRepositoryAcquisitionCallLedger{relevanceCalls: 1},
				GroundedRequirement:  "Which component owns ARTIFACT_1?",
				KnownArtifactPaths:   []string{"internal/private/secret_owner.go"},
				ArtifactIdentities: []assemblyline.ArtifactIdentity{{
					Token: "ARTIFACT_1", Value: "internal/private/secret_owner.go",
				}},
			}, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "internal/private/secret_owner.go owns dispatch." {
		t.Fatalf("output=%q", result.Output)
	}
}

func TestObjectiveTurnRepositoryPathRejectsMissingAnswerStation(t *testing.T) {
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
		nil,
		objectiveWorkflows{RepositoryRead: func(context.Context, turnAuthority) (objectiveEvidenceAcquisition, error) {
			return objectiveRepositoryTestAcquisition([]objectiveEvidence{evidence}, 1), nil
		}},
	)
	if err == nil || !strings.Contains(err.Error(), "grounded-answer station") {
		t.Fatalf("missing repository station error=%v", err)
	}
}

func TestObjectiveTurnRelationshipQueryRetainsRelationThroughAnswerAndCitation(t *testing.T) {
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
	if len(stations.answerInput.Evidence) != 1 ||
		stations.answerInput.Evidence[0].Text != relation.Capsule.Text ||
		len(result.Citations) != 1 || result.Citations[0] != relation {
		t.Fatalf("relation lineage was lost: answer=%#v result=%#v", stations.answerInput, result)
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

func objectiveRepositoryTestAcquisition(
	evidence []objectiveEvidence,
	modelCalls int,
) objectiveEvidenceAcquisition {
	ledger := objectiveRepositoryAcquisitionCallLedger{relevanceCalls: modelCalls}
	return objectiveEvidenceAcquisition{
		Evidence: evidence, ModelCalls: modelCalls, RepositoryCallLedger: ledger,
		GroundedRequirement: "Resolve the exact repository requirement.",
	}
}
