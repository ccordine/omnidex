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
	if _, err := assemblyline.NewRepositoryEvidenceRelevanceJob(input); err != nil {
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
	job, err := assemblyline.NewRepositoryEvidenceRelevanceJob(input)
	if err != nil {
		return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableCall[assemblyline.RepositoryEvidenceRelevanceDecision](
		ctx, r, modelName, "repository_evidence_relevance", job,
		func(value assemblyline.RepositoryEvidenceRelevanceDecision) error { return value.ValidateFor(input) },
	)
	return decision, objectiveStationReceipt{Calls: calls}, err
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
	job, err := assemblyline.NewRepositorySearchTermJob(input)
	if err != nil {
		return assemblyline.RepositorySearchTermDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableCall[assemblyline.RepositorySearchTermDecision](
		ctx, r, modelName, "repository_search_term", job,
		func(value assemblyline.RepositorySearchTermDecision) error { return value.ValidateFor(input) },
	)
	return decision, objectiveStationReceipt{Calls: calls}, err
}
