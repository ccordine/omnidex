package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type objectiveDatabaseStations interface {
	SelectSchema(context.Context, assemblyline.DatabaseSchemaSelectionInput) (
		assemblyline.DatabaseSchemaSelectionDecision, objectiveStationReceipt, error,
	)
	BuildIntent(context.Context, assemblyline.DatabaseQueryIntentInput) (
		assemblyline.DatabaseQueryIntentDecision, objectiveStationReceipt, error,
	)
	SelectJoinPath(context.Context, assemblyline.DatabaseJoinPathSelectionInput) (
		assemblyline.DatabaseJoinPathSelectionDecision, objectiveStationReceipt, error,
	)
}

func (adapter portableObjectiveDatabaseStations) SelectJoinPath(
	ctx context.Context,
	input assemblyline.DatabaseJoinPathSelectionInput,
) (assemblyline.DatabaseJoinPathSelectionDecision, objectiveStationReceipt, error) {
	job, err := assemblyline.NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		return assemblyline.DatabaseJoinPathSelectionDecision{}, objectiveStationReceipt{}, err
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
		func(value assemblyline.DatabaseJoinPathSelectionDecision) error { return value.ValidateFor(input) },
	)
}

type portableObjectiveDatabaseStations struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveDatabaseStations) SelectSchema(
	ctx context.Context,
	input assemblyline.DatabaseSchemaSelectionInput,
) (assemblyline.DatabaseSchemaSelectionDecision, objectiveStationReceipt, error) {
	resolveModel := func() (string, error) {
		return objectiveStationModel(adapter.runtime, station.DatabaseSchemaSelection)
	}
	var ledger objectiveDatabaseRawLeafCallLedger
	decision, calls, err := resolveObjectiveDatabaseSchemaSelection(
		ctx, input,
		adapter.rawLeafCall(station.DatabaseSchemaSelection, resolveModel, &ledger),
	)
	if err != nil {
		return decision, ledger.partial(), err
	}
	receipt, err := ledger.complete(calls)
	return decision, receipt, err
}

func (adapter portableObjectiveDatabaseStations) BuildIntent(
	ctx context.Context,
	input assemblyline.DatabaseQueryIntentInput,
) (assemblyline.DatabaseQueryIntentDecision, objectiveStationReceipt, error) {
	resolveModel := func() (string, error) {
		return objectiveStationModel(adapter.runtime, station.DatabaseQueryIntent)
	}
	var ledger objectiveDatabaseRawLeafCallLedger
	decision, calls, err := resolveObjectiveDatabaseQueryIntent(
		ctx, input,
		adapter.rawLeafCall(station.DatabaseQueryIntent, resolveModel, &ledger),
	)
	if err != nil {
		return decision, ledger.partial(), err
	}
	receipt, err := ledger.complete(calls)
	return decision, receipt, err
}
