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
) (assemblyline.RoleplayGroundedResponseDecision, objectiveStationReceipt, error) {
	if err := input.Validate(); err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{}, err
	}
	resolveModel := func() (string, error) {
		return objectiveStationModel(adapter.runtime, station.ConversationResponse)
	}
	inventoryJob, err := assemblyline.NewRoleplayGroundedParagraphInventoryJob(input)
	if err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{}, err
	}
	inventory, receipt, err := runObjectivePortableRawLeafStation(
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
	totalCalls, allReused := receipt.Calls, receipt.Reused
	if err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{Calls: totalCalls, Reused: allReused}, err
	}

	paragraphs := make([]assemblyline.RoleplayGroundedParagraph, 0, len(inventory.Candidates))
	processed := make(map[string]struct{}, len(inventory.Candidates))
	for _, candidate := range inventory.Candidates {
		if _, duplicate := processed[candidate]; duplicate {
			continue
		}
		processed[candidate] = struct{}{}

		authorizationInput := assemblyline.RoleplayGroundedParagraphAuthorizationInput{
			ExactQuestion:      input.ExactQuestion,
			RoleplayIdentity:   input.RoleplayIdentity,
			Context:            assemblyline.CloneObjectiveContext(input.Context),
			ParagraphText:      candidate,
			Evidence:           append([]assemblyline.GroundedEvidenceCapsule(nil), input.RealWorldEvidence...),
			KnownArtifactPaths: append([]string{}, input.KnownArtifactPaths...),
		}
		authorizationJob, err := assemblyline.NewRoleplayGroundedParagraphAuthorizationJob(
			authorizationInput,
		)
		if err != nil {
			return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{Calls: totalCalls, Reused: allReused}, err
		}
		authorization, authorizationReceipt, err := runObjectivePortableRawLeafStation(
			ctx,
			adapter.runtime,
			"roleplay_grounded_paragraph_authorization",
			authorizationJob,
			station.ConversationResponse,
			resolveModel,
			func(raw string) (assemblyline.RoleplayGroundedParagraphAuthorizationDecision, error) {
				return assemblyline.DecodeRoleplayGroundedParagraphAuthorizationDecision(
					authorizationInput, raw,
				)
			},
		)
		totalCalls += authorizationReceipt.Calls
		allReused = allReused && authorizationReceipt.Reused
		if err != nil {
			return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{Calls: totalCalls, Reused: allReused}, err
		}
		if authorization.Relation != assemblyline.RoleplayGroundedParagraphResponsiveAndSupported {
			continue
		}

		evidenceIDs, _, leafReceipt, err := adapter.bindRoleplayGroundedEvidence(
			ctx, input, candidate, resolveModel,
		)
		totalCalls += leafReceipt.Calls
		allReused = allReused && leafReceipt.Reused
		if err != nil {
			return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{Calls: totalCalls, Reused: allReused}, err
		}
		if len(evidenceIDs) == 0 {
			continue
		}
		paragraphs = append(paragraphs, assemblyline.RoleplayGroundedParagraph{
			Text: candidate, EvidenceIDs: evidenceIDs,
		})
	}
	if len(paragraphs) == 0 {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{
				Calls: totalCalls, Reused: allReused,
			}, fmt.Errorf(
				"roleplay grounded paragraph inventory queue produced no responsive fully supported paragraphs",
			)
	}
	decision, err := assemblyline.AssembleRoleplayGroundedResponseDecision(input, paragraphs)
	return decision, objectiveStationReceipt{Calls: totalCalls, Reused: allReused}, err
}

func (adapter portableObjectiveRoleplayGroundedStation) bindRoleplayGroundedEvidence(
	ctx context.Context,
	input assemblyline.RoleplayGroundedResponseInput,
	paragraphText string,
	resolveModel func() (string, error),
) ([]string, []assemblyline.GroundedEvidenceCapsule, objectiveStationReceipt, error) {
	evidenceIDs := make([]string, 0, len(input.RealWorldEvidence))
	supporting := make([]assemblyline.GroundedEvidenceCapsule, 0, len(input.RealWorldEvidence))
	totalCalls := 0
	allReused := true
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
			return nil, nil, objectiveStationReceipt{Calls: totalCalls, Reused: allReused}, err
		}
		relation, receipt, err := runObjectivePortableRawLeafStation(
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
		totalCalls += receipt.Calls
		allReused = allReused && receipt.Reused
		if err != nil {
			return nil, nil, objectiveStationReceipt{Calls: totalCalls, Reused: allReused}, err
		}
		if relation == assemblyline.RoleplayGroundedEvidenceSupportsParagraph {
			evidenceIDs = append(evidenceIDs, evidence.ID)
			supporting = append(supporting, evidence)
		}
	}
	return evidenceIDs, supporting, objectiveStationReceipt{
		Calls: totalCalls, Reused: allReused,
	}, nil
}
