package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const maxRepositoryGroundedCitations = 2

func objectiveRepositoryRelevanceInput(
	exactRequirement string,
	evidence []objectiveEvidence,
) (assemblyline.RepositoryEvidenceRelevanceInput, error) {
	candidates := make([]assemblyline.RepositoryEvidenceCandidate, len(evidence))
	for index, item := range evidence {
		if item.SourceType != "repository_symbol" && item.SourceType != "repository_relation" {
			return assemblyline.RepositoryEvidenceRelevanceInput{}, fmt.Errorf(
				"repository relevance candidate %q has source type %q",
				item.Capsule.ID, item.SourceType,
			)
		}
		if err := validateObjectiveEvidence(item); err != nil {
			return assemblyline.RepositoryEvidenceRelevanceInput{}, err
		}
		candidates[index] = assemblyline.RepositoryEvidenceCandidate{
			EvidenceID: item.Capsule.ID, Text: item.SelectionText,
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
