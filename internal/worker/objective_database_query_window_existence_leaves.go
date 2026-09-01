package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func resolveDatabaseQueryWindows(
	ctx context.Context,
	state assemblyline.DatabaseQueryIntentLeafState,
	call objectiveDatabaseRawLeafCall,
	total int,
) (assemblyline.DatabaseQueryIntentLeafState, int, error) {
	purposes, nextTotal, err := resolveDatabaseQueryPurposeQueue(
		ctx,
		assemblyline.DatabaseQueryPurposeAuthority{
			State: state, Collection: assemblyline.DatabaseQueryWindowPurpose,
		},
		datasource.MaxIntentFilters-len(state.TemporalWindows), false, call, total,
	)
	total = nextTotal
	if err != nil {
		return state, total, err
	}
	for _, purpose := range purposes {
		leaf := assemblyline.DatabaseQueryWindowLeafInput{State: state, Purpose: purpose}
		var calls int
		fieldID, resolved, err := assemblyline.ResolveSoleDatabaseQueryWindowFieldLeaf(leaf)
		if err != nil {
			return state, total, err
		}
		if resolved {
			leaf.FieldID = fieldID
		} else {
			fieldJob, err := assemblyline.NewDatabaseQueryWindowFieldJob(leaf)
			if err != nil {
				return state, total, err
			}
			leaf.FieldID, calls, err = callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_window_field", fieldJob,
				func(raw string) (string, error) {
					return assemblyline.DecodeDatabaseQueryWindowFieldLeaf(leaf, raw)
				},
			)
			total += calls
			if err != nil {
				return state, total, err
			}
		}
		unit, resolved, err := assemblyline.ResolveSoleDatabaseQueryWindowUnitLeaf(leaf)
		if err != nil {
			return state, total, err
		}
		if resolved {
			leaf.Unit = unit
		} else {
			unitJob, err := assemblyline.NewDatabaseQueryWindowUnitJob(leaf)
			if err != nil {
				return state, total, err
			}
			leaf.Unit, calls, err = callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_window_unit", unitJob,
				func(raw string) (datasource.WindowUnit, error) {
					return assemblyline.DecodeDatabaseQueryWindowUnitLeaf(leaf, raw)
				},
			)
			total += calls
			if err != nil {
				return state, total, err
			}
		}
		amountJob, err := assemblyline.NewDatabaseQueryWindowAmountJob(leaf)
		if err != nil {
			return state, total, err
		}
		amount, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_window_amount", amountJob,
			func(raw string) (int, error) {
				return assemblyline.DecodeDatabaseQueryWindowAmountLeaf(leaf, raw)
			},
		)
		total += calls
		if err != nil {
			return state, total, err
		}
		state.TemporalWindows = append(state.TemporalWindows, assemblyline.DatabaseTemporalWindowDecision{
			FieldID: leaf.FieldID, Unit: leaf.Unit, Amount: amount,
		})
	}
	return state, total, nil
}

func resolveDatabaseQueryExistence(
	ctx context.Context,
	state assemblyline.DatabaseQueryIntentLeafState,
	call objectiveDatabaseRawLeafCall,
	total int,
) (assemblyline.DatabaseQueryIntentLeafState, int, error) {
	purposes, nextTotal, err := resolveDatabaseQueryPurposeQueue(
		ctx,
		assemblyline.DatabaseQueryPurposeAuthority{
			State: state, Collection: assemblyline.DatabaseQueryExistencePurpose,
		},
		datasource.MaxIntentExistenceChecks-len(state.Exists), false, call, total,
	)
	total = nextTotal
	if err != nil {
		return state, total, err
	}
	for _, purpose := range purposes {
		leaf := assemblyline.DatabaseQueryExistenceLeafInput{
			State: state, Purpose: purpose, Filters: []datasource.RelationalPredicate{},
		}
		var calls int
		relationID, resolved, err := assemblyline.ResolveSoleDatabaseQueryExistenceRelationLeaf(leaf)
		if err != nil {
			return state, total, err
		}
		if resolved {
			leaf.RelationID = relationID
		} else {
			relationJob, err := assemblyline.NewDatabaseQueryExistenceRelationJob(leaf)
			if err != nil {
				return state, total, err
			}
			leaf.RelationID, calls, err = callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_existence_relation", relationJob,
				func(raw string) (string, error) {
					return assemblyline.DecodeDatabaseQueryExistenceRelationLeaf(leaf, raw)
				},
			)
			total += calls
			if err != nil {
				return state, total, err
			}
		}
		negatedJob, err := assemblyline.NewDatabaseQueryExistenceNegatedJob(leaf)
		if err != nil {
			return state, total, err
		}
		negated, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_existence_negated", negatedJob,
			func(raw string) (bool, error) {
				return assemblyline.DecodeDatabaseQueryExistenceNegatedLeaf(leaf, raw)
			},
		)
		total += calls
		if err != nil {
			return state, total, err
		}
		filters, nextTotal, err := resolveDatabaseQueryFilters(
			ctx, state, leaf.RelationID, purpose, leaf.Filters, call, total,
		)
		total = nextTotal
		if err != nil {
			return state, total, err
		}
		state.Exists = append(state.Exists, datasource.ExistencePredicate{
			RelationID: leaf.RelationID, Negated: negated, Filters: filters,
		})
	}
	return state, total, nil
}

func objectiveDatabaseTemporalFields(state assemblyline.DatabaseQueryIntentLeafState) []string {
	fields := []string{}
	for _, relation := range state.Authority.SchemaProjection.Relations {
		for _, column := range relation.Columns {
			if column.TypeCategory == datasource.TypeTemporal || column.TypeCategory == datasource.TypeDate {
				fields = append(fields, column.ID)
			}
		}
	}
	return fields
}

func objectiveDatabaseTemporalField(state assemblyline.DatabaseQueryIntentLeafState, fieldID string) bool {
	for _, candidate := range objectiveDatabaseTemporalFields(state) {
		if candidate == fieldID {
			return true
		}
	}
	return false
}
