package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func (s *directCodingSession) resolveEndpointsForHTTPStack(
	runtime typedWorkerRuntime,
	stack directCodingProjectStack,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	identities []assemblyline.ArtifactIdentity,
) (directCodingServiceEndpointPlan, error) {
	if stack.CompileServiceSource == nil {
		return directCodingServiceEndpointPlan{}, fmt.Errorf(
			"project stack %s has no HTTP compiler", stack.ID,
		)
	}
	requirementModel, err := s.workerModel(station.CodingServiceEndpointRequirement)
	if err != nil {
		return directCodingServiceEndpointPlan{}, err
	}
	models := directCodingServiceEndpointLeafModels{}
	for _, binding := range []struct {
		destination *string
		stationID   station.ID
	}{
		{&models.Exposure, station.CodingServiceEndpointExposure},
		{&models.Method, station.CodingServiceEndpointMethod},
		{&models.Route, station.CodingServiceEndpointRouteTemplate},
		{&models.RequestMedia, station.CodingServiceEndpointRequestMedia},
		{&models.ResponseMedia, station.CodingServiceEndpointResponseMedia},
		{&models.SuccessStatus, station.CodingServiceEndpointSuccessStatus},
	} {
		*binding.destination, err = s.workerModel(binding.stationID)
		if err != nil {
			return directCodingServiceEndpointPlan{}, err
		}
	}
	return resolveDirectCodingServiceEndpointPlan(
		runtime, requirementModel, models, stack, workload, capabilities, identities,
	)
}
