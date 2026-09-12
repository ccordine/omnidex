package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type repositoryGroundedInventoryLeaf func(
	context.Context,
	assemblyline.GroundedAnswerParagraphInventoryInput,
) (assemblyline.GroundedAnswerParagraphInventory, int, error)

type repositoryGroundedSupportLeaf func(
	context.Context,
	assemblyline.GroundedAnswerParagraphEvidenceRelationInput,
) (assemblyline.GroundedAnswerParagraphEvidenceRelationDecision, int, error)

type repositoryGroundedRelevanceLeaf func(
	context.Context,
	assemblyline.GroundedParagraphRelevanceInput,
) (assemblyline.GroundedParagraphRelevance, int, error)

type repositoryGroundedFullSupportLeaf func(
	context.Context,
	assemblyline.GroundedParagraphSupportInput,
) (assemblyline.GroundedParagraphSupport, int, error)

// resolveRepositoryGroundedParagraphQueue owns the exact-deduplicated queue.
// Inventory output has no answer authority; accepted paragraphs are appended
// once and are never presented to a later model call.
func resolveRepositoryGroundedParagraphQueue(
	ctx context.Context,
	input assemblyline.GroundedAnswerInput,
	inventoryLeaf repositoryGroundedInventoryLeaf,
	supportLeaf repositoryGroundedSupportLeaf,
	relevanceLeaf repositoryGroundedRelevanceLeaf,
	fullSupportLeaf repositoryGroundedFullSupportLeaf,
) (assemblyline.GroundedAnswerDecision, int, error) {
	var zero assemblyline.GroundedAnswerDecision
	if ctx == nil || inventoryLeaf == nil || supportLeaf == nil || relevanceLeaf == nil || fullSupportLeaf == nil {
		return zero, 0, fmt.Errorf(
			"repository grounded paragraph queue requires context and all semantic leaves",
		)
	}
	if err := input.Validate(); err != nil {
		return zero, 0, err
	}
	inventoryInput := assemblyline.GroundedAnswerParagraphInventoryInput{
		ExactRequirement:   input.ExactRequirement,
		Context:            assemblyline.CloneObjectiveContext(input.Context),
		Evidence:           append([]assemblyline.GroundedEvidenceCapsule(nil), input.Evidence...),
		KnownArtifactPaths: append([]string(nil), input.KnownArtifactPaths...),
	}
	inventory, inventoryCalls, err := inventoryLeaf(ctx, inventoryInput)
	total := inventoryCalls
	if err != nil {
		return zero, total, err
	}
	if err := validateObjectiveLeafCallCount(
		"repository grounded paragraph inventory", inventoryCalls,
	); err != nil {
		return zero, total, err
	}
	if err := inventory.ValidateFor(inventoryInput); err != nil {
		return zero, total, err
	}

	accepted := make([]assemblyline.GroundedAnswerParagraph, 0, len(inventory.Candidates))
	seenCandidates := make(map[string]struct{}, len(inventory.Candidates))
	for _, candidate := range inventory.Candidates {
		if _, duplicate := seenCandidates[candidate]; duplicate {
			continue
		}
		seenCandidates[candidate] = struct{}{}

		relevanceInput := assemblyline.GroundedParagraphRelevanceInput{
			ExactQuestion:      input.ExactRequirement,
			Context:            assemblyline.CloneObjectiveContext(input.Context),
			ParagraphText:      candidate,
			KnownArtifactPaths: append([]string(nil), input.KnownArtifactPaths...),
		}
		relevance, leafCalls, err := relevanceLeaf(ctx, relevanceInput)
		total += leafCalls
		if err != nil {
			return zero, total, err
		}
		if err := validateObjectiveLeafCallCount(
			"repository grounded paragraph relevance", leafCalls,
		); err != nil {
			return zero, total, err
		}
		if err := relevance.ValidateFor(relevanceInput); err != nil {
			return zero, total, err
		}
		if relevance != assemblyline.GroundedParagraphRelevant {
			continue
		}

		fullSupportInput := assemblyline.GroundedParagraphSupportInput{
			ParagraphText: candidate, ClaimDomain: assemblyline.GroundedAllFactualClaims,
			Evidence:           append([]assemblyline.GroundedEvidenceCapsule(nil), input.Evidence...),
			KnownArtifactPaths: append([]string(nil), input.KnownArtifactPaths...),
		}
		fullSupport, leafCalls, err := fullSupportLeaf(ctx, fullSupportInput)
		total += leafCalls
		if err != nil {
			return zero, total, err
		}
		if err := validateObjectiveLeafCallCount("repository grounded paragraph support", leafCalls); err != nil {
			return zero, total, err
		}
		if err := fullSupport.ValidateFor(fullSupportInput); err != nil {
			return zero, total, err
		}
		if fullSupport != assemblyline.GroundedParagraphFullySupported {
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
			relation, leafCalls, err := supportLeaf(ctx, relationInput)
			total += leafCalls
			if err != nil {
				return zero, total, err
			}
			if err := validateObjectiveLeafCallCount(
				"repository grounded paragraph evidence relation", leafCalls,
			); err != nil {
				return zero, total, err
			}
			if err := relation.ValidateFor(relationInput); err != nil {
				return zero, total, err
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
		return zero, total, fmt.Errorf(
			"repository grounded paragraph inventory queue produced no responsive fully supported paragraphs",
		)
	}
	decision, err := assemblyline.AssembleGroundedAnswerDecision(input, accepted)
	return decision, total, err
}
