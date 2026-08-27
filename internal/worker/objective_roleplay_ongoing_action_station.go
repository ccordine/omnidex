package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type portableObjectiveRoleplayOngoingActionStation struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveRoleplayOngoingActionStation) ResolveOngoingAction(
	ctx context.Context,
	input assemblyline.RoleplayOngoingActionInput,
) (assemblyline.RoleplayOngoingActionDecision, objectiveStationReceipt, error) {
	model, err := objectiveStationModel(adapter.runtime, station.RoleplayOngoingAction)
	if err != nil {
		return assemblyline.RoleplayOngoingActionDecision{}, objectiveStationReceipt{}, err
	}
	job, err := assemblyline.NewRoleplayOngoingActionJob(input)
	if err != nil {
		return assemblyline.RoleplayOngoingActionDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableCall[assemblyline.RoleplayOngoingActionDecision](
		ctx, adapter.runtime, model, "roleplay_ongoing_action", job,
		func(value assemblyline.RoleplayOngoingActionDecision) error {
			return value.ValidateFor(input)
		},
	)
	return decision, objectiveStationReceipt{Calls: calls}, err
}
