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
	job, err := assemblyline.NewRoleplayOngoingActionJob(input)
	if err != nil {
		return assemblyline.RoleplayOngoingActionDecision{}, objectiveStationReceipt{}, err
	}
	decision, receipt, err := runObjectiveReusablePortableCall[assemblyline.RoleplayOngoingActionDecision](
		ctx, adapter.runtime, "roleplay_ongoing_action", job,
		station.RoleplayOngoingAction,
		func() (string, error) { return objectiveRoleplaySemanticModel(adapter.runtime) },
		func(value assemblyline.RoleplayOngoingActionDecision) error {
			return value.ValidateFor(input)
		},
	)
	return decision, receipt, err
}
