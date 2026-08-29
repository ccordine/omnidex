package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func resolveDatabaseQueryWindows(
	ctx context.Context,
	state assemblyline.DatabaseQueryIntentLeafState,
	call objectiveDatabaseRawLeafCall,
	total int,
) (assemblyline.DatabaseQueryIntentLeafState, int, error) {
	for {
		job, err := assemblyline.NewDatabaseQueryWindowCoverageJob(state)
		if err != nil {
			return state, total, err
		}
		coverage, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_window_coverage", job,
			func(raw string) (string, error) {
				return assemblyline.DecodeDatabaseQueryWindowCoverageLeaf(state, raw)
			},
		)
		total += calls
		if err != nil || coverage == assemblyline.DatabaseQueryNoUncoveredItem {
			return state, total, err
		}
		leaf := assemblyline.DatabaseQueryWindowLeafInput{State: state}
		fields := objectiveDatabaseTemporalFields(state)
		if len(fields) == 0 {
			return state, total, fmt.Errorf("database query window has no temporal field")
		}
		if len(fields) == 1 {
			leaf.FieldID = fields[0]
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
}

func resolveDatabaseQueryExistence(
	ctx context.Context,
	state assemblyline.DatabaseQueryIntentLeafState,
	call objectiveDatabaseRawLeafCall,
	total int,
) (assemblyline.DatabaseQueryIntentLeafState, int, error) {
	for {
		job, err := assemblyline.NewDatabaseQueryExistenceCoverageJob(state)
		if err != nil {
			return state, total, err
		}
		coverage, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_existence_coverage", job,
			func(raw string) (string, error) {
				return assemblyline.DecodeDatabaseQueryExistenceCoverageLeaf(state, raw)
			},
		)
		total += calls
		if err != nil || coverage == assemblyline.DatabaseQueryNoUncoveredItem {
			return state, total, err
		}
		leaf := assemblyline.DatabaseQueryExistenceLeafInput{
			State: state, Filters: []datasource.RelationalPredicate{},
		}
		relations := objectiveDatabaseExistenceRelations(state)
		if len(relations) == 0 {
			return state, total, fmt.Errorf("database query existence has no unused projected relation")
		}
		if len(relations) == 1 {
			leaf.RelationID = relations[0]
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
			ctx, state, leaf.RelationID, leaf.Filters, call, total,
		)
		total = nextTotal
		if err != nil {
			return state, total, err
		}
		state.Exists = append(state.Exists, datasource.ExistencePredicate{
			RelationID: leaf.RelationID, Negated: negated, Filters: filters,
		})
	}
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

func objectiveDatabaseExistenceRelations(state assemblyline.DatabaseQueryIntentLeafState) []string {
	used := map[string]struct{}{}
	for _, predicate := range state.Exists {
		used[predicate.RelationID] = struct{}{}
	}
	relations := []string{}
	for _, relation := range state.Authority.SchemaProjection.Relations {
		if _, duplicate := used[relation.ID]; !duplicate {
			relations = append(relations, relation.ID)
		}
	}
	return relations
}
