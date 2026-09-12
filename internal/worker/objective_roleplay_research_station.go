package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

// portableObjectiveRoleplayGroundedStation obtains one untrusted paragraph
// inventory. Code owns the source-order queue and independently binds and
// authorizes each candidate. A rejected candidate dies at the sieve boundary;
// it cannot veto already accepted paragraphs or create more work.
type portableObjectiveRoleplayGroundedStation struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveRoleplayGroundedStation) RespondGrounded(
	ctx context.Context,
	input assemblyline.RoleplayGroundedResponseInput,
) (assemblyline.RoleplayGroundedResponseDecision, int, error) {
	if err := input.Validate(); err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, 0, err
	}
	resolveModel := func() (string, error) {
		return objectiveStationModel(adapter.runtime, station.ConversationResponse)
	}
	inventoryJob, err := assemblyline.NewRoleplayGroundedParagraphInventoryJob(input)
	if err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, 0, err
	}
	inventory, dispatches, err := runObjectivePortableRawLeafStation(
		ctx,
		adapter.runtime,
		"roleplay_grounded_paragraph_inventory",
		inventoryJob,
		station.ConversationResponse,
		resolveModel,
		func(raw string) (assemblyline.RoleplayGroundedParagraphInventory, error) {
			return assemblyline.DecodeRoleplayGroundedParagraphInventory(input, raw)
		},
	)
	totalCalls := dispatches
	if err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, totalCalls, err
	}

	paragraphs := make([]assemblyline.RoleplayGroundedParagraph, 0, len(inventory.Candidates))
	processed := make(map[string]struct{}, len(inventory.Candidates))
	for _, candidate := range inventory.Candidates {
		if _, duplicate := processed[candidate]; duplicate {
			continue
		}
		processed[candidate] = struct{}{}

		relevanceInput := assemblyline.GroundedParagraphRelevanceInput{
			ExactQuestion:      input.ExactQuestion,
			Context:            assemblyline.CloneObjectiveContext(input.Context),
			ParagraphText:      candidate,
			KnownArtifactPaths: append([]string{}, input.KnownArtifactPaths...),
		}
		relevance, relevanceDispatches, err := runGroundedParagraphRelevance(
			ctx, adapter.runtime, station.ConversationResponse, resolveModel, relevanceInput,
		)
		totalCalls += relevanceDispatches
		if err != nil {
			return assemblyline.RoleplayGroundedResponseDecision{}, totalCalls, err
		}
		if relevance != assemblyline.GroundedParagraphRelevant {
			continue
		}
		supportInput := assemblyline.GroundedParagraphSupportInput{
			ParagraphText: candidate, ClaimDomain: assemblyline.GroundedRealWorldClaims,
			Evidence:           append([]assemblyline.GroundedEvidenceCapsule(nil), input.RealWorldEvidence...),
			KnownArtifactPaths: append([]string{}, input.KnownArtifactPaths...),
		}
		support, supportDispatches, err := runGroundedParagraphSupport(
			ctx, adapter.runtime, station.ConversationResponse, resolveModel, supportInput,
		)
		totalCalls += supportDispatches
		if err != nil {
			return assemblyline.RoleplayGroundedResponseDecision{}, totalCalls, err
		}
		if support != assemblyline.GroundedParagraphFullySupported {
			continue
		}

		evidenceIDs, _, leafCalls, err := adapter.bindRoleplayGroundedEvidence(
			ctx, input, candidate, resolveModel,
		)
		totalCalls += leafCalls
		if err != nil {
			return assemblyline.RoleplayGroundedResponseDecision{}, totalCalls, err
		}
		if len(evidenceIDs) == 0 {
			continue
		}
		paragraphs = append(paragraphs, assemblyline.RoleplayGroundedParagraph{
			Text: candidate, EvidenceIDs: evidenceIDs,
		})
	}
	if len(paragraphs) == 0 {
		return assemblyline.RoleplayGroundedResponseDecision{}, totalCalls, fmt.Errorf(
			"roleplay grounded paragraph inventory queue produced no responsive fully supported paragraphs",
		)
	}
	decision, err := assemblyline.AssembleRoleplayGroundedResponseDecision(input, paragraphs)
	return decision, totalCalls, err
}

func (adapter portableObjectiveRoleplayGroundedStation) bindRoleplayGroundedEvidence(
	ctx context.Context,
	input assemblyline.RoleplayGroundedResponseInput,
	paragraphText string,
	resolveModel func() (string, error),
) ([]string, []assemblyline.GroundedEvidenceCapsule, int, error) {
	return resolveRoleplayGroundedEvidenceRelations(
		ctx,
		input,
		paragraphText,
		func(
			ctx context.Context,
			relationInput assemblyline.RoleplayGroundedEvidenceRelationInput,
		) (assemblyline.RoleplayGroundedEvidenceRelation, int, error) {
			relationJob, err := assemblyline.NewRoleplayGroundedResponseEvidenceRelationJob(
				relationInput,
			)
			if err != nil {
				return "", 0, err
			}
			return runObjectivePortableRawLeafStation(
				ctx,
				adapter.runtime,
				"roleplay_grounded_response_evidence_relation",
				relationJob,
				station.ConversationResponse,
				resolveModel,
				func(raw string) (assemblyline.RoleplayGroundedEvidenceRelation, error) {
					return assemblyline.DecodeRoleplayGroundedResponseEvidenceRelationLeaf(
						relationInput, raw,
					)
				},
			)
		},
	)
}

type roleplayGroundedEvidenceRelationLeaf func(
	context.Context,
	assemblyline.RoleplayGroundedEvidenceRelationInput,
) (assemblyline.RoleplayGroundedEvidenceRelation, int, error)

func resolveRoleplayGroundedEvidenceRelations(
	ctx context.Context,
	input assemblyline.RoleplayGroundedResponseInput,
	paragraphText string,
	relate roleplayGroundedEvidenceRelationLeaf,
) ([]string, []assemblyline.GroundedEvidenceCapsule, int, error) {
	if relate == nil {
		return nil, nil, 0, fmt.Errorf(
			"roleplay grounded evidence requires one binary relation leaf",
		)
	}
	evidenceIDs := make([]string, 0, len(input.RealWorldEvidence))
	supporting := make([]assemblyline.GroundedEvidenceCapsule, 0, len(input.RealWorldEvidence))
	totalCalls := 0
	for _, evidence := range input.RealWorldEvidence {
		relationInput := assemblyline.RoleplayGroundedEvidenceRelationInput{
			ExactQuestion:      input.ExactQuestion,
			ParagraphText:      paragraphText,
			Evidence:           evidence,
			KnownArtifactPaths: append([]string{}, input.KnownArtifactPaths...),
		}
		relation, dispatches, err := relate(ctx, relationInput)
		totalCalls += dispatches
		if err != nil {
			return nil, nil, totalCalls, err
		}
		if relation == assemblyline.RoleplayGroundedEvidenceSupportsParagraph {
			evidenceIDs = append(evidenceIDs, evidence.ID)
			supporting = append(supporting, evidence)
		}
	}
	return evidenceIDs, supporting, totalCalls, nil
}
