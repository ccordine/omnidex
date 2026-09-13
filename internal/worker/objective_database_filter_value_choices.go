package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func resolveDatabaseQueryClosedFilterValues(
	ctx context.Context,
	leaf assemblyline.DatabaseQueryFilterLeafInput,
	call objectiveDatabaseRawLeafCall,
	total int,
) ([]datasource.IntentLiteral, int, error) {
	if ctx == nil {
		return nil, total, fmt.Errorf("database filter value selection requires context")
	}
	leaf.AcceptedValues = append([]datasource.IntentLiteral{}, leaf.AcceptedValues...)
	for {
		if err := ctx.Err(); err != nil {
			return nil, total, err
		}
		choice, resolved, err := assemblyline.ResolveDatabaseQueryFilterValueChoice(leaf)
		if err != nil {
			return nil, total, err
		}
		if !resolved {
			job, err := assemblyline.NewDatabaseQueryFilterValueChoiceJob(leaf)
			if err != nil {
				return nil, total, err
			}
			var calls int
			choice, calls, err = callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_filter_value_choice", job,
				func(raw string) (assemblyline.DatabaseQueryFilterValueChoice, error) {
					return assemblyline.DecodeDatabaseQueryFilterValueChoice(leaf, raw)
				},
			)
			total += calls
			if err != nil {
				return nil, total, err
			}
			if err := validateObjectiveLeafCallCount("database filter value choice", calls); err != nil {
				return nil, total, err
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, total, err
		}
		if err := choice.ValidateFor(leaf); err != nil {
			return nil, total, err
		}
		if choice.Value == nil {
			if len(leaf.AcceptedValues) == 0 {
				return nil, total, fmt.Errorf("database set-membership filter has no applicable value")
			}
			return leaf.AcceptedValues, total, nil
		}
		leaf.AcceptedValues = append(leaf.AcceptedValues, *choice.Value)
	}
}
