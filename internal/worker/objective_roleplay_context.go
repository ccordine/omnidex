package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/roleplay"
)

func (r *nativeRuntimeV3) projectObjectiveRoleplaySimulation(
	ctx context.Context,
	preparationID string,
	jobID int64,
) (roleplay.SimulationTurnAuthority, roleplay.NarrativeSimulationProjection, error) {
	if ctx == nil || r == nil || r.svc == nil || r.svc.repo == nil || r.claim == nil {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
			fmt.Errorf("roleplay simulation projection requires claimed job authority")
	}
	if jobID != r.claim.Job.ID {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
			fmt.Errorf("roleplay simulation projection job differs from claimed authority")
	}
	preparation, projection, err := r.svc.repo.ProjectRoleplaySimulationContext(
		ctx, preparationID, jobID,
	)
	if err != nil {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{}, err
	}
	return preparation, projection, nil
}
