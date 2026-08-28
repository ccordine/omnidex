package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func resolveObjectiveDatabaseSchemaSelection(
	ctx context.Context,
	input assemblyline.DatabaseSchemaSelectionInput,
	call objectiveDatabaseRawLeafCall,
) (assemblyline.DatabaseSchemaSelectionDecision, int, error) {
	selected := make([]string, 0, input.MaxSelections)
	totalCalls := 0
	for {
		leafInput := assemblyline.DatabaseSchemaSelectionLeafInput{
			Authority: input, SelectedRelationIDs: append([]string{}, selected...),
		}
		coverageJob, err := assemblyline.NewDatabaseSchemaSelectionCoverageJob(leafInput)
		if err != nil {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
		}
		coverage, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_schema_selection_coverage", coverageJob,
			func(raw string) (string, error) {
				return assemblyline.DecodeDatabaseSchemaSelectionCoverageLeaf(leafInput, raw)
			},
		)
		totalCalls += calls
		if err != nil {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
		}
		if coverage == assemblyline.DatabaseSchemaNoUncoveredRelation {
			decision, err := assemblyline.AssembleDatabaseSchemaSelectionDecision(input, selected)
			return decision, totalCalls, err
		}
		if len(selected) == input.MaxSelections {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, fmt.Errorf(
				"database schema selection remains incomplete at its %d-relation bound",
				input.MaxSelections,
			)
		}
		relationJob, err := assemblyline.NewDatabaseSchemaRelationSelectionJob(leafInput)
		if err != nil {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
		}
		relationID, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_schema_relation_selection", relationJob,
			func(raw string) (string, error) {
				return assemblyline.DecodeDatabaseSchemaRelationSelectionLeaf(leafInput, raw)
			},
		)
		totalCalls += calls
		if err != nil {
			return assemblyline.DatabaseSchemaSelectionDecision{}, totalCalls, err
		}
		selected = append(selected, relationID)
	}
}
