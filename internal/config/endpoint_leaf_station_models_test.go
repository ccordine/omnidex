package config

import (
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestEndpointLeafStationsReuseCodingWorkloadEnvironmentModel(t *testing.T) {
	t.Setenv("OMNI_CODING_WORKLOAD_MODEL", "endpoint-leaf-model")
	models := loadStationModels(Config{})
	for _, id := range []station.ID{
		station.CodingServiceEndpointExposure,
		station.CodingServiceEndpointMethod,
		station.CodingServiceEndpointRouteTemplate,
		station.CodingServiceEndpointRequestMedia,
		station.CodingServiceEndpointResponseMedia,
		station.CodingServiceEndpointSuccessStatus,
	} {
		if got := models[id]; got != "endpoint-leaf-model" {
			t.Fatalf("endpoint leaf station %s model=%q", id, got)
		}
	}
}

func TestStateLeafStationsReuseCodingWorkloadEnvironmentModel(t *testing.T) {
	t.Setenv("OMNI_CODING_WORKLOAD_MODEL", "state-leaf-model")
	models := loadStationModels(Config{})
	for _, id := range []station.ID{
		station.CodingApplicationStateFieldPurposeInventory,
		station.CodingApplicationStateFieldKind,
		station.CodingApplicationRecordFieldPurposeInventory,
		station.CodingApplicationRecordFieldKind,
		station.CodingApplicationServiceStatePurposeNecessity,
		station.CodingApplicationServiceStatePurposeRelation,
	} {
		if got := models[id]; got != "state-leaf-model" {
			t.Fatalf("state leaf station %s model=%q", id, got)
		}
	}
}
