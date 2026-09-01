package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type repositoryGroundedInventoryLeaf func(
	context.Context,
	assemblyline.GroundedAnswerParagraphInventoryInput,
) (assemblyline.GroundedAnswerParagraphInventory, objectiveStationReceipt, error)

type repositoryGroundedSupportLeaf func(
	context.Context,
	assemblyline.GroundedAnswerParagraphEvidenceRelationInput,
) (assemblyline.GroundedAnswerParagraphEvidenceRelationDecision, objectiveStationReceipt, error)

type repositoryGroundedAuthorizationLeaf func(
	context.Context,
	assemblyline.GroundedAnswerParagraphAuthorizationInput,
) (assemblyline.GroundedAnswerParagraphAuthorizationDecision, objectiveStationReceipt, error)

// resolveRepositoryGroundedParagraphQueue owns the exact-deduplicated queue.
// Inventory output has no answer authority; accepted paragraphs are appended
// once and are never presented to a later model call.
func resolveRepositoryGroundedParagraphQueue(
	ctx context.Context,
	input assemblyline.GroundedAnswerInput,
	inventoryLeaf repositoryGroundedInventoryLeaf,
	supportLeaf repositoryGroundedSupportLeaf,
	authorizationLeaf repositoryGroundedAuthorizationLeaf,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	var zero assemblyline.GroundedAnswerDecision
	if ctx == nil || inventoryLeaf == nil || supportLeaf == nil || authorizationLeaf == nil {
		return zero, objectiveStationReceipt{}, fmt.Errorf(
			"repository grounded paragraph queue requires context and all semantic leaves",
		)
	}
	if err := input.Validate(); err != nil {
		return zero, objectiveStationReceipt{}, err
	}
	inventoryInput := assemblyline.GroundedAnswerParagraphInventoryInput{
		ExactRequirement:   input.ExactRequirement,
		Context:            assemblyline.CloneObjectiveContext(input.Context),
		Evidence:           append([]assemblyline.GroundedEvidenceCapsule(nil), input.Evidence...),
		KnownArtifactPaths: append([]string(nil), input.KnownArtifactPaths...),
	}
	inventory, inventoryReceipt, err := inventoryLeaf(ctx, inventoryInput)
	total := inventoryReceipt.Calls
	allReused := inventoryReceipt.Reused
	if err != nil {
		return zero, objectiveStationReceipt{Calls: total, Reused: allReused}, err
	}
	if err := validateObjectiveStationReceipt(
		"repository grounded paragraph inventory", inventoryReceipt,
	); err != nil {
		return zero, objectiveStationReceipt{Calls: total, Reused: allReused}, err
	}
	if err := inventory.ValidateFor(inventoryInput); err != nil {
		return zero, objectiveStationReceipt{Calls: total, Reused: allReused}, err
	}

	accepted := make([]assemblyline.GroundedAnswerParagraph, 0, len(inventory.Candidates))
	seenCandidates := make(map[string]struct{}, len(inventory.Candidates))
	for _, candidate := range inventory.Candidates {
		if _, duplicate := seenCandidates[candidate]; duplicate {
			continue
		}
		seenCandidates[candidate] = struct{}{}

		authorizationInput := assemblyline.GroundedAnswerParagraphAuthorizationInput{
			ExactRequirement:   input.ExactRequirement,
			Context:            assemblyline.CloneObjectiveContext(input.Context),
			ParagraphText:      candidate,
			Evidence:           append([]assemblyline.GroundedEvidenceCapsule(nil), input.Evidence...),
			KnownArtifactPaths: append([]string(nil), input.KnownArtifactPaths...),
		}
		authorization, leafReceipt, err := authorizationLeaf(ctx, authorizationInput)
		total += leafReceipt.Calls
		allReused = allReused && leafReceipt.Reused
		if err != nil {
			return zero, objectiveStationReceipt{Calls: total, Reused: allReused}, err
		}
		if err := validateObjectiveStationReceipt(
			"repository grounded paragraph authorization", leafReceipt,
		); err != nil {
			return zero, objectiveStationReceipt{Calls: total, Reused: allReused}, err
		}
		if err := authorization.ValidateFor(authorizationInput); err != nil {
			return zero, objectiveStationReceipt{Calls: total, Reused: allReused}, err
		}
		if authorization.Relation != assemblyline.GroundedParagraphResponsiveAndFullySupported {
			continue
		}

		supporting := make([]assemblyline.GroundedEvidenceCapsule, 0, len(input.Evidence))
		evidenceIDs := make([]string, 0, len(input.Evidence))
		for _, evidence := range input.Evidence {
			relationInput := assemblyline.GroundedAnswerParagraphEvidenceRelationInput{
				ParagraphText:      candidate,
				Evidence:           evidence,
				KnownArtifactPaths: append([]string(nil), input.KnownArtifactPaths...),
			}
			relation, leafReceipt, err := supportLeaf(ctx, relationInput)
			total += leafReceipt.Calls
			allReused = allReused && leafReceipt.Reused
			if err != nil {
				return zero, objectiveStationReceipt{Calls: total, Reused: allReused}, err
			}
			if err := validateObjectiveStationReceipt(
				"repository grounded paragraph evidence relation", leafReceipt,
			); err != nil {
				return zero, objectiveStationReceipt{Calls: total, Reused: allReused}, err
			}
			if err := relation.ValidateFor(relationInput); err != nil {
				return zero, objectiveStationReceipt{Calls: total, Reused: allReused}, err
			}
			if relation.Relation == assemblyline.GroundedEvidenceSupportsParagraph {
				supporting = append(supporting, evidence)
				evidenceIDs = append(evidenceIDs, evidence.ID)
			}
		}
		if len(supporting) == 0 {
			continue
		}
		accepted = append(accepted, assemblyline.GroundedAnswerParagraph{
			Text: candidate, EvidenceIDs: evidenceIDs,
		})
	}
	if len(accepted) == 0 {
		return zero, objectiveStationReceipt{Calls: total, Reused: allReused}, fmt.Errorf(
			"repository grounded paragraph inventory queue produced no responsive fully supported paragraphs",
		)
	}
	decision, err := assemblyline.AssembleGroundedAnswerDecision(input, accepted)
	return decision, objectiveStationReceipt{Calls: total, Reused: allReused}, err
}
