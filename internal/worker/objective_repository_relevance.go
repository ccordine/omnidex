package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const maxRepositoryGroundedCitations = 2

func resolveObjectiveRepositorySearchTerm(
	exactRequirement string,
	resolve objectiveRepositorySearchTermCall,
) ([]string, objectiveStationReceipt, error) {
	input := assemblyline.RepositorySearchTermInput{UnresolvedConcept: exactRequirement}
	decision, receipt, err := resolve(exactRequirement)
	if err != nil {
		return nil, receipt, err
	}
	if err := decision.ValidateFor(input); err != nil {
		return nil, receipt, err
	}
	return append([]string(nil), decision.Anchors...), receipt, nil
}

func objectiveRepositoryRelevanceInput(
	exactRequirement string,
	evidence []objectiveEvidence,
) (assemblyline.RepositoryEvidenceRelevanceInput, error) {
	candidates := make([]assemblyline.RepositoryEvidenceCandidate, len(evidence))
	for index, item := range evidence {
		candidates[index] = assemblyline.RepositoryEvidenceCandidate{
			EvidenceID: item.Capsule.ID, Text: item.Capsule.Text,
		}
	}
	input := assemblyline.RepositoryEvidenceRelevanceInput{
		ExactRequirement: exactRequirement, Candidates: candidates,
		MaxSelections: minInt(maxRepositoryGroundedCitations, len(candidates)),
	}
	if err := input.Validate(); err != nil {
		return assemblyline.RepositoryEvidenceRelevanceInput{}, err
	}
	return input, nil
}

func filterObjectiveRepositoryEvidence(
	evidence []objectiveEvidence,
	selectedIDs []string,
) ([]objectiveEvidence, error) {
	selected := make(map[string]struct{}, len(selectedIDs))
	for _, id := range selectedIDs {
		selected[id] = struct{}{}
	}
	out := make([]objectiveEvidence, 0, len(selectedIDs))
	for _, item := range evidence {
		if _, keep := selected[item.Capsule.ID]; keep {
			out = append(out, item)
		}
	}
	if len(out) != len(selectedIDs) {
		return nil, fmt.Errorf("repository relevance selection lost exact evidence")
	}
	return out, nil
}

func cloneObjectiveRepositoryEvidence(items []objectiveEvidence) []objectiveEvidence {
	cloned := make([]objectiveEvidence, len(items))
	copy(cloned, items)
	return cloned
}

func (r *nativeRuntimeV3) resolveObjectiveRepositoryRelevance(
	ctx context.Context,
	exactRequirement string,
	evidence []objectiveEvidence,
) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
	input, err := objectiveRepositoryRelevanceInput(exactRequirement, evidence)
	if err != nil {
		return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{}, err
	}
	modelName, err := r.svc.requiredStationModel(r.routing, station.RepositoryEvidenceRelevance)
	if err != nil {
		return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{}, err
	}
	selected := make([]string, 0, input.MaxSelections)
	totalCalls := 0
	for len(selected) < input.MaxSelections {
		leafInput := assemblyline.RepositoryEvidenceRelevanceLeafInput{
			ExactRequirement:    input.ExactRequirement,
			Candidates:          append([]assemblyline.RepositoryEvidenceCandidate(nil), input.Candidates...),
			SelectedEvidenceIDs: append([]string{}, selected...),
			MaxSelections:       input.MaxSelections,
		}
		job, err := assemblyline.NewRepositoryEvidenceRelevanceLeafJob(leafInput)
		if err != nil {
			return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
		}
		evidenceID, calls, err := runObjectivePortableRawLeafCall(
			ctx, r, modelName, "repository_evidence_relevance_leaf", job,
			func(raw string) (string, error) {
				return assemblyline.DecodeRepositoryEvidenceRelevanceLeaf(leafInput, raw)
			},
			func(string) error { return nil },
		)
		totalCalls += calls
		if err != nil {
			return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
		}
		if evidenceID == assemblyline.RepositoryEvidenceNoRelevantCandidate {
			break
		}
		selected = append(selected, evidenceID)
	}
	decision, err := assemblyline.AssembleRepositoryEvidenceRelevanceDecision(input, selected)
	return decision, objectiveStationReceipt{Calls: totalCalls}, err
}

func (r *nativeRuntimeV3) resolveObjectiveRepositorySearchTerm(
	ctx context.Context,
	exactRequirement string,
) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
	input := assemblyline.RepositorySearchTermInput{UnresolvedConcept: exactRequirement}
	modelName, err := r.svc.requiredStationModel(r.routing, station.CodingRepositorySearchTerm)
	if err != nil {
		return assemblyline.RepositorySearchTermDecision{}, objectiveStationReceipt{}, err
	}
	anchors := make([]string, 0, assemblyline.MaxRepositorySearchAnchorLeaves)
	totalCalls := 0
	for {
		leafInput := assemblyline.RepositorySearchAnchorLeafInput{
			UnresolvedConcept: exactRequirement,
			AcceptedAnchors:   append([]string{}, anchors...),
		}
		anchorJob, err := assemblyline.NewRepositorySearchAnchorJob(leafInput)
		if err != nil {
			return assemblyline.RepositorySearchTermDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
		}
		anchor, calls, err := runObjectivePortableRawLeafCall(
			ctx, r, modelName, "repository_search_anchor", anchorJob,
			func(raw string) (string, error) {
				return assemblyline.DecodeRepositorySearchAnchorLeaf(leafInput, raw)
			},
			func(value string) error {
				return assemblyline.ValidatePathFreeModelContext(
					"repository search anchor", value,
				)
			},
		)
		totalCalls += calls
		if err != nil {
			return assemblyline.RepositorySearchTermDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
		}
		anchors = append(anchors, anchor)
		leafInput.AcceptedAnchors = append([]string{}, anchors...)
		coverageJob, err := assemblyline.NewRepositorySearchAnchorCoverageJob(leafInput)
		if err != nil {
			return assemblyline.RepositorySearchTermDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
		}
		coverage, calls, err := runObjectivePortableRawLeafCall(
			ctx, r, modelName, "repository_search_anchor_coverage", coverageJob,
			func(raw string) (string, error) {
				return assemblyline.DecodeRepositorySearchAnchorCoverageLeaf(leafInput, raw)
			},
			func(string) error { return nil },
		)
		totalCalls += calls
		if err != nil {
			return assemblyline.RepositorySearchTermDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
		}
		if coverage == assemblyline.RepositoryNoUncoveredAnchor {
			decision, err := assemblyline.AssembleRepositorySearchTermDecision(input, anchors)
			return decision, objectiveStationReceipt{Calls: totalCalls}, err
		}
		if len(anchors) == assemblyline.MaxRepositorySearchAnchorLeaves {
			return assemblyline.RepositorySearchTermDecision{}, objectiveStationReceipt{Calls: totalCalls}, fmt.Errorf(
				"repository search anchor coverage remains incomplete at the code-owned %d-item bound",
				assemblyline.MaxRepositorySearchAnchorLeaves,
			)
		}
	}
}
