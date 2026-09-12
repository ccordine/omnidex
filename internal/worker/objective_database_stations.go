package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type objectiveDatabaseStations interface {
	SelectSchema(context.Context, assemblyline.DatabaseSchemaSelectionInput) (
		assemblyline.DatabaseSchemaSelectionDecision, int, error,
	)
	BuildIntent(context.Context, assemblyline.DatabaseQueryIntentInput) (
		assemblyline.DatabaseQueryIntentDecision, int, error,
	)
	SelectJoinPath(context.Context, assemblyline.DatabaseJoinPathSelectionInput) (
		assemblyline.DatabaseJoinPathSelectionDecision, int, error,
	)
}

func (adapter portableObjectiveDatabaseStations) SelectJoinPath(
	ctx context.Context,
	input assemblyline.DatabaseJoinPathSelectionInput,
) (assemblyline.DatabaseJoinPathSelectionDecision, int, error) {
	decision, resolved, err := assemblyline.ResolveSoleDatabaseJoinPathSelectionDecision(input)
	if err != nil {
		return assemblyline.DatabaseJoinPathSelectionDecision{}, 0, err
	}
	if resolved {
		return decision, 0, nil
	}
	job, err := assemblyline.NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		return assemblyline.DatabaseJoinPathSelectionDecision{}, 0, err
	}
	return runObjectivePortableRawLeafStation(
		ctx, adapter.runtime, "database_join_path_selection", job,
		station.DatabaseJoinPathSelection,
		func() (string, error) {
			return objectiveStationModel(adapter.runtime, station.DatabaseJoinPathSelection)
		},
		func(raw string) (assemblyline.DatabaseJoinPathSelectionDecision, error) {
			return assemblyline.DecodeDatabaseJoinPathSelectionDecision(input, raw)
		},
	)
}

type portableObjectiveDatabaseStations struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveDatabaseStations) SelectSchema(
	ctx context.Context,
	input assemblyline.DatabaseSchemaSelectionInput,
) (assemblyline.DatabaseSchemaSelectionDecision, int, error) {
	resolveModel := func() (string, error) {
		return objectiveStationModel(adapter.runtime, station.DatabaseSchemaSelection)
	}
	decision, calls, err := resolveObjectiveDatabaseSchemaSelection(
		ctx, input,
		adapter.rawLeafCall(station.DatabaseSchemaSelection, resolveModel),
	)
	return decision, calls, err
}

func (adapter portableObjectiveDatabaseStations) BuildIntent(
	ctx context.Context,
	input assemblyline.DatabaseQueryIntentInput,
) (assemblyline.DatabaseQueryIntentDecision, int, error) {
	resolveModel := func() (string, error) {
		return objectiveStationModel(adapter.runtime, station.DatabaseQueryIntent)
	}
	decision, calls, err := resolveObjectiveDatabaseQueryIntent(
		ctx, input,
		adapter.rawLeafCall(station.DatabaseQueryIntent, resolveModel),
	)
	return decision, calls, err
}
