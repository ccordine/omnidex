package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const maxDatabaseSchemaSelectionLeafCalls = 1 + 2*assemblyline.MaxDatabaseSchemaRelationInventoryCandidates

func resolveObjectiveDatabaseSchemaSelection(
	ctx context.Context,
	input assemblyline.DatabaseSchemaSelectionInput,
	call objectiveDatabaseRawLeafCall,
) (assemblyline.DatabaseSchemaSelectionDecision, int, error) {
	inventoryInput, err := assemblyline.ProjectDatabaseSchemaRelationInventoryInput(input)
	if err != nil {
		return assemblyline.DatabaseSchemaSelectionDecision{}, 0, err
	}
	inventoryJob, err := assemblyline.NewDatabaseSchemaRelationInventoryJob(inventoryInput)
	if err != nil {
		return assemblyline.DatabaseSchemaSelectionDecision{}, 0, err
	}
	inventory, totalCalls, err := callObjectiveDatabaseRawLeaf(
		ctx, call, "database_schema_relation_inventory", inventoryJob,
		func(raw string) (assemblyline.DatabaseSchemaRelationInventory, error) {
			return assemblyline.DecodeDatabaseSchemaRelationInventory(inventoryInput, raw)
		},
	)
	if err != nil {
		return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
	}
	selected := make([]string, 0, input.MaxSelections)
	selectedSet := make(map[string]struct{}, input.MaxSelections)
	seenCandidates := make(map[string]struct{}, len(inventory.Candidates))
	for candidateIndex, candidate := range inventory.Candidates {
		if _, duplicate := seenCandidates[candidate]; duplicate {
			continue
		}
		seenCandidates[candidate] = struct{}{}
		necessityInput := assemblyline.DatabaseSchemaRelationNecessityInput{
			ExactNeed: input.ExactNeed,
			Context:   assemblyline.CloneObjectiveContext(input.Context),
			Candidate: candidate,
		}
		necessityJob, err := assemblyline.NewDatabaseSchemaRelationNecessityJob(necessityInput)
		if err != nil {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
		}
		necessity, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call,
			fmt.Sprintf("database_schema_relation_necessity_%03d", candidateIndex+1),
			necessityJob,
			func(raw string) (assemblyline.DatabaseSchemaRelationNecessityResult, error) {
				return assemblyline.DecodeDatabaseSchemaRelationNecessityResult(necessityInput, raw)
			},
		)
		totalCalls += calls
		if err != nil {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
		}
		if necessity.Relation == assemblyline.DatabaseSchemaRelationNotNecessary {
			continue
		}
		resolutionInput := assemblyline.DatabaseSchemaRelationResolutionInput{
			Candidate: candidate,
			Candidates: append(
				[]assemblyline.DatabaseSchemaCandidate(nil), input.Candidates...,
			),
		}
		resolutionJob, err := assemblyline.NewDatabaseSchemaRelationResolutionJob(
			resolutionInput,
		)
		if err != nil {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
		}
		resolution, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call,
			fmt.Sprintf("database_schema_relation_resolution_%03d", candidateIndex+1),
			resolutionJob,
			func(raw string) (assemblyline.DatabaseSchemaRelationResolutionResult, error) {
				return assemblyline.DecodeDatabaseSchemaRelationResolutionResult(
					resolutionInput, raw,
				)
			},
		)
		totalCalls += calls
		if err != nil {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
		}
		if _, duplicate := selectedSet[resolution.RelationID]; duplicate {
			continue
		}
		if len(selected) == input.MaxSelections {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, fmt.Errorf(
				"database schema relation inventory contains more than %d necessary distinct registered relations",
				input.MaxSelections,
			)
		}
		selected = append(selected, resolution.RelationID)
		selectedSet[resolution.RelationID] = struct{}{}
	}
	decision, err := assemblyline.AssembleDatabaseSchemaSelectionDecision(input, selected)
	return decision, totalCalls, err
}
