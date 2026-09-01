package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type portableObjectiveRoleplayOngoingActionStation struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveRoleplayOngoingActionStation) ResolveOngoingActionRelation(
	ctx context.Context,
	input assemblyline.RoleplayOngoingActionRelationInput,
) (assemblyline.RoleplayOngoingActionRelation, objectiveStationReceipt, error) {
	job, err := assemblyline.NewRoleplayOngoingActionRelationJob(input)
	if err != nil {
		return "", objectiveStationReceipt{}, err
	}
	return runObjectivePortableRawLeafStation(
		ctx, adapter.runtime, "roleplay_ongoing_action_relation", job,
		station.RoleplayOngoingActionRelation,
		func() (string, error) { return objectiveRoleplaySemanticModel(adapter.runtime) },
		func(raw string) (assemblyline.RoleplayOngoingActionRelation, error) {
			return assemblyline.DecodeRoleplayOngoingActionRelation(input, raw)
		},
	)
}

func (adapter portableObjectiveRoleplayOngoingActionStation) GenerateOngoingActionValue(
	ctx context.Context,
	input assemblyline.RoleplayOngoingActionValueInput,
) (string, objectiveStationReceipt, error) {
	job, err := assemblyline.NewRoleplayOngoingActionValueJob(input)
	if err != nil {
		return "", objectiveStationReceipt{}, err
	}
	return runObjectivePortableRawLeafStation(
		ctx, adapter.runtime, "roleplay_ongoing_action_value", job,
		station.RoleplayOngoingActionValue,
		func() (string, error) { return objectiveRoleplaySemanticModel(adapter.runtime) },
		func(raw string) (string, error) {
			return assemblyline.DecodeRoleplayOngoingActionValue(input, raw)
		},
	)
}
