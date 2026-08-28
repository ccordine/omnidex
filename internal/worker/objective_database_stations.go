package worker

import (
	"context"
	"fmt"

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
	FindEvidenceGap(context.Context, assemblyline.DatabaseEvidenceGapInput) (
		assemblyline.DatabaseEvidenceGapDecision, objectiveStationReceipt, error,
	)
	SelectJoinPath(context.Context, assemblyline.DatabaseJoinPathSelectionInput) (
		assemblyline.DatabaseJoinPathSelectionDecision, objectiveStationReceipt, error,
	)
}

func (adapter portableObjectiveDatabaseStations) SelectJoinPath(
	ctx context.Context,
	input assemblyline.DatabaseJoinPathSelectionInput,
) (assemblyline.DatabaseJoinPathSelectionDecision, objectiveStationReceipt, error) {
	modelName, err := objectiveStationModel(adapter.runtime, station.DatabaseJoinPathSelection)
	if err != nil {
		return assemblyline.DatabaseJoinPathSelectionDecision{}, objectiveStationReceipt{}, err
	}
	job, err := assemblyline.NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		return assemblyline.DatabaseJoinPathSelectionDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableRawLeafCall(
		ctx, adapter.runtime, modelName, "database_join_path_selection", job,
		func(raw string) (assemblyline.DatabaseJoinPathSelectionDecision, error) {
			return assemblyline.DecodeDatabaseJoinPathSelectionDecision(input, raw)
		},
		func(value assemblyline.DatabaseJoinPathSelectionDecision) error { return value.ValidateFor(input) },
	)
	return decision, objectiveStationReceipt{Calls: calls}, err
}

type portableObjectiveDatabaseStations struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveDatabaseStations) SelectSchema(
	ctx context.Context,
	input assemblyline.DatabaseSchemaSelectionInput,
) (assemblyline.DatabaseSchemaSelectionDecision, objectiveStationReceipt, error) {
	modelName, err := objectiveStationModel(adapter.runtime, station.DatabaseSchemaSelection)
	if err != nil {
		return assemblyline.DatabaseSchemaSelectionDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := resolveObjectiveDatabaseSchemaSelection(
		ctx, input, adapter.rawLeafCall(modelName),
	)
	return decision, objectiveStationReceipt{Calls: calls}, err
}

func (adapter portableObjectiveDatabaseStations) BuildIntent(
	ctx context.Context,
	input assemblyline.DatabaseQueryIntentInput,
) (assemblyline.DatabaseQueryIntentDecision, objectiveStationReceipt, error) {
	modelName, err := objectiveStationModel(adapter.runtime, station.DatabaseQueryIntent)
	if err != nil {
		return assemblyline.DatabaseQueryIntentDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := resolveObjectiveDatabaseQueryIntent(
		ctx, input, adapter.rawLeafCall(modelName),
	)
	return decision, objectiveStationReceipt{Calls: calls}, err
}

func (adapter portableObjectiveDatabaseStations) FindEvidenceGap(
	ctx context.Context,
	input assemblyline.DatabaseEvidenceGapInput,
) (assemblyline.DatabaseEvidenceGapDecision, objectiveStationReceipt, error) {
	modelName, err := objectiveStationModel(adapter.runtime, station.DatabaseEvidenceGap)
	if err != nil {
		return assemblyline.DatabaseEvidenceGapDecision{}, objectiveStationReceipt{}, err
	}
	job, err := assemblyline.NewDatabaseEvidenceGapJob(input)
	if err != nil {
		return assemblyline.DatabaseEvidenceGapDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableRawLeafCall(
		ctx, adapter.runtime, modelName, "database_evidence_gap", job,
		func(raw string) (assemblyline.DatabaseEvidenceGapDecision, error) {
			return assemblyline.DecodeDatabaseEvidenceGapDecision(input, raw)
		},
		func(value assemblyline.DatabaseEvidenceGapDecision) error { return value.ValidateFor(input) },
	)
	return decision, objectiveStationReceipt{Calls: calls}, err
}

func validateObjectiveDatabaseStationCalls(label string, receipt objectiveStationReceipt) error {
	maximum := maxTypedWorkerAttempts
	if label == "schema selection" {
		maximum = maxDatabaseSchemaSelectionModelCalls
	}
	if label == "query intent" {
		maximum = maxObjectiveDatabaseQueryIntentCalls
	}
	if receipt.Calls < 1 || receipt.Calls > maximum {
		return fmt.Errorf("database %s station reported %d calls outside the bounded correction budget", label, receipt.Calls)
	}
	return nil
}
