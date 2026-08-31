package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const maxRepositoryGroundedCitations = 2

type objectiveRepositoryEvidenceRelationCall func(
	context.Context,
	assemblyline.RepositoryEvidenceRelevanceRelationInput,
) (assemblyline.RepositoryEvidenceRelevanceRelationResult, objectiveStationReceipt, error)

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
	resolveModel := func() (string, error) {
		return objectiveStationModel(r, station.RepositoryEvidenceRelevance)
	}
	return resolveObjectiveRepositoryEvidenceRelations(
		ctx, input,
		func(
			ctx context.Context,
			relationInput assemblyline.RepositoryEvidenceRelevanceRelationInput,
		) (assemblyline.RepositoryEvidenceRelevanceRelationResult, objectiveStationReceipt, error) {
			job, err := assemblyline.NewRepositoryEvidenceRelevanceRelationJob(relationInput)
			if err != nil {
				return assemblyline.RepositoryEvidenceRelevanceRelationResult{}, objectiveStationReceipt{}, err
			}
			return runObjectivePortableRawLeafStation(
				ctx, r, "repository_evidence_relevance_relation", job,
				station.RepositoryEvidenceRelevance, resolveModel,
				func(raw string) (assemblyline.RepositoryEvidenceRelevanceRelationResult, error) {
					return assemblyline.DecodeRepositoryEvidenceRelevanceRelationResult(relationInput, raw)
				},
				func(value assemblyline.RepositoryEvidenceRelevanceRelationResult) error {
					return value.ValidateFor(relationInput)
				},
			)
		},
	)
}

func resolveObjectiveRepositoryEvidenceRelations(
	ctx context.Context,
	input assemblyline.RepositoryEvidenceRelevanceInput,
	call objectiveRepositoryEvidenceRelationCall,
) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
	if err := input.Validate(); err != nil {
		return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{}, err
	}
	if call == nil {
		return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{}, fmt.Errorf(
			"repository evidence relevance relation call is unavailable",
		)
	}
	selected := make([]string, 0, input.MaxSelections)
	totalCalls := 0
	allReused := true
	for _, candidate := range input.Candidates {
		if len(selected) == input.MaxSelections {
			break
		}
		relationInput := assemblyline.RepositoryEvidenceRelevanceRelationInput{
			ExactRequirement: input.ExactRequirement,
			Candidate:        candidate,
		}
		relation, receipt, err := call(ctx, relationInput)
		totalCalls += receipt.Calls
		allReused = allReused && receipt.Reused
		if err != nil {
			return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{
				Calls: totalCalls, Reused: allReused,
			}, err
		}
		if err := validateObjectiveStationReceipt(
			"repository evidence relevance relation", receipt,
		); err != nil {
			return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{
				Calls: totalCalls, Reused: allReused,
			}, err
		}
		if err := relation.ValidateFor(relationInput); err != nil {
			return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{
				Calls: totalCalls, Reused: allReused,
			}, err
		}
		if relation.Relation == assemblyline.RepositoryEvidenceDirectlyRelevant {
			selected = append(selected, candidate.EvidenceID)
		}
	}
	decision, err := assemblyline.AssembleRepositoryEvidenceRelevanceDecision(input, selected)
	return decision, objectiveStationReceipt{Calls: totalCalls, Reused: allReused}, err
}
