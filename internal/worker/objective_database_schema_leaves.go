package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const maxDatabaseSchemaSelectionLeafCalls = assemblyline.MaxDatabaseSchemaCandidates

func resolveObjectiveDatabaseSchemaSelection(
	ctx context.Context,
	input assemblyline.DatabaseSchemaSelectionInput,
	call objectiveDatabaseRawLeafCall,
) (assemblyline.DatabaseSchemaSelectionDecision, int, error) {
	remaining := append([]assemblyline.DatabaseSchemaCandidate(nil), input.Candidates...)
	if _, err := assemblyline.ProjectDatabaseSchemaRelationChoiceInput(input, remaining); err != nil {
		return assemblyline.DatabaseSchemaSelectionDecision{}, 0, err
	}
	if len(remaining) == 1 && !input.HasAcceptedRelations {
		decision, err := assemblyline.AssembleDatabaseSchemaSelectionDecision(
			input, []string{remaining[0].RelationID},
		)
		return decision, 0, err
	}

	selected := make([]string, 0, input.MaxSelections)
	totalCalls := 0
	for len(remaining) > 0 {
		if len(selected) == input.MaxSelections {
			break
		}
		if totalCalls >= maxDatabaseSchemaSelectionLeafCalls {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, fmt.Errorf(
				"database schema selection exceeded its %d-call semantic reduction bound",
				maxDatabaseSchemaSelectionLeafCalls,
			)
		}
		choiceInput, err := assemblyline.ProjectDatabaseSchemaRelationChoiceInput(input, remaining)
		if err != nil {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
		}
		choiceJob, err := assemblyline.NewDatabaseSchemaRelationChoiceJob(choiceInput)
		if err != nil {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
		}
		choice, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call,
			fmt.Sprintf("database_schema_relation_choice_%03d", len(selected)+1),
			choiceJob,
			func(raw string) (assemblyline.DatabaseSchemaRelationChoiceResult, error) {
				return assemblyline.DecodeDatabaseSchemaRelationChoiceResult(choiceInput, raw)
			},
		)
		totalCalls += calls
		if err != nil {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
		}
		if totalCalls > maxDatabaseSchemaSelectionLeafCalls {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, fmt.Errorf(
				"database schema selection exceeded its %d-call semantic reduction bound",
				maxDatabaseSchemaSelectionLeafCalls,
			)
		}
		if choice.NoAdditional {
			break
		}
		selected = append(selected, choice.RelationID)
		var removed bool
		remaining, removed = removeDatabaseSchemaCandidate(remaining, choice.RelationID)
		if !removed {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, fmt.Errorf(
				"database schema relation choice %q was not in the remaining code-owned set",
				choice.RelationID,
			)
		}
	}
	decision, err := assemblyline.AssembleDatabaseSchemaSelectionDecision(input, selected)
	return decision, totalCalls, err
}

func removeDatabaseSchemaCandidate(
	candidates []assemblyline.DatabaseSchemaCandidate,
	relationID string,
) ([]assemblyline.DatabaseSchemaCandidate, bool) {
	for index, candidate := range candidates {
		if candidate.RelationID != relationID {
			continue
		}
		remaining := make([]assemblyline.DatabaseSchemaCandidate, 0, len(candidates)-1)
		remaining = append(remaining, candidates[:index]...)
		remaining = append(remaining, candidates[index+1:]...)
		return remaining, true
	}
	return candidates, false
}
