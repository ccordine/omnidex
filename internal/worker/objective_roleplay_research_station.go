package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

// portableObjectiveRoleplayGroundedStation resolves one narrative text leaf,
// then code binds each paragraph to evidence through independent pairwise
// semantic relations. Models never author paragraph arrays or evidence IDs.
type portableObjectiveRoleplayGroundedStation struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveRoleplayGroundedStation) RespondGrounded(
	ctx context.Context,
	input assemblyline.RoleplayGroundedResponseInput,
) (assemblyline.RoleplayGroundedResponseDecision, objectiveStationReceipt, error) {
	if err := input.Validate(); err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{}, err
	}
	modelName, err := objectiveStationModel(adapter.runtime, station.ConversationResponse)
	if err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{}, err
	}
	job, err := assemblyline.NewRoleplayGroundedResponseTextJob(input)
	if err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{}, err
	}
	text, calls, err := runObjectivePortableRawLeafCall(
		ctx, adapter.runtime, modelName, "roleplay_grounded_response_text", job,
		func(raw string) (string, error) {
			return assemblyline.DecodeRoleplayGroundedResponseTextLeaf(input, raw)
		},
		func(string) error { return nil },
	)
	if err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{Calls: calls}, err
	}
	paragraphTexts, err := assemblyline.SplitRoleplayGroundedResponseParagraphs(text)
	if err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{Calls: calls}, err
	}
	paragraphs := make([]assemblyline.RoleplayGroundedParagraph, 0, len(paragraphTexts))
	for _, paragraphText := range paragraphTexts {
		evidenceIDs := make([]string, 0, len(input.RealWorldEvidence))
		for _, evidence := range input.RealWorldEvidence {
			relationInput := assemblyline.RoleplayGroundedEvidenceRelationInput{
				ExactQuestion:      input.ExactQuestion,
				ParagraphText:      paragraphText,
				Evidence:           evidence,
				KnownArtifactPaths: append([]string{}, input.KnownArtifactPaths...),
			}
			relationJob, err := assemblyline.NewRoleplayGroundedResponseEvidenceRelationJob(
				relationInput,
			)
			if err != nil {
				return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{Calls: calls}, err
			}
			relation, relationCalls, err := runObjectivePortableRawLeafCall(
				ctx, adapter.runtime, modelName,
				"roleplay_grounded_response_evidence_relation", relationJob,
				func(raw string) (assemblyline.RoleplayGroundedEvidenceRelation, error) {
					return assemblyline.DecodeRoleplayGroundedResponseEvidenceRelationLeaf(
						relationInput, raw,
					)
				},
				func(assemblyline.RoleplayGroundedEvidenceRelation) error { return nil },
			)
			calls += relationCalls
			if err != nil {
				return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{Calls: calls}, err
			}
			if relation == assemblyline.RoleplayGroundedEvidenceSupportsParagraph {
				evidenceIDs = append(evidenceIDs, evidence.ID)
			}
		}
		if len(evidenceIDs) == 0 {
			return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{Calls: calls}, fmt.Errorf(
				"roleplay grounded response paragraph has no semantically bound evidence",
			)
		}
		paragraphs = append(paragraphs, assemblyline.RoleplayGroundedParagraph{
			Text: paragraphText, EvidenceIDs: evidenceIDs,
		})
	}
	decision, err := assemblyline.AssembleRoleplayGroundedResponseDecision(input, paragraphs)
	return decision, objectiveStationReceipt{Calls: calls}, err
}
