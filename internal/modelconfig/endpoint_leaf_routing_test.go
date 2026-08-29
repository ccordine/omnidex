package modelconfig

import (
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestEndpointLeafStationsReuseCodingWorkloadModelRouting(t *testing.T) {
	t.Parallel()
	routing := Apply(Routing{}, Config{"coding_workload_model": "endpoint-leaf-model"})
	for _, id := range []station.ID{
		station.CodingServiceEndpointExposure,
		station.CodingServiceEndpointMethod,
		station.CodingServiceEndpointRouteTemplate,
		station.CodingServiceEndpointRequestMedia,
		station.CodingServiceEndpointResponseMedia,
		station.CodingServiceEndpointSuccessStatus,
	} {
		if got := routing.Stations[id]; got != "endpoint-leaf-model" {
			t.Fatalf("endpoint leaf station %s model=%q", id, got)
		}
	}
}

func TestStateLeafStationsReuseCodingWorkloadModelRouting(t *testing.T) {
	t.Parallel()
	routing := Apply(Routing{}, Config{"coding_workload_model": "state-leaf-model"})
	for _, id := range []station.ID{
		station.CodingApplicationStateFieldCoverage,
		station.CodingApplicationStateFieldPurpose,
		station.CodingApplicationStateFieldKind,
		station.CodingApplicationRecordFieldCoverage,
		station.CodingApplicationRecordFieldPurpose,
		station.CodingApplicationRecordFieldKind,
	} {
		if got := routing.Stations[id]; got != "state-leaf-model" {
			t.Fatalf("state leaf station %s model=%q", id, got)
		}
	}
}
