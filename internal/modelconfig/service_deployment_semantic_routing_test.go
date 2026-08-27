package modelconfig

import (
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestRequirementsRouteCannotOverrideServiceDeploymentSemanticRoutes(t *testing.T) {
	base := Routing{Stations: map[station.ID]string{
		station.CodingServiceContinuedAvailability:  "availability-model",
		station.CodingServicePersistenceDestination: "destination-model",
	}}
	applied := Apply(base, Config{"coding_requirements_model": "requirements-model"})

	if got := applied.Stations[station.CodingRequirements]; got != "requirements-model" {
		t.Fatalf("requirements model=%q", got)
	}
	if got := applied.Stations[station.CodingProjectStackConstraint]; got != "requirements-model" {
		t.Fatalf("stack constraint model=%q", got)
	}
	if got := applied.Stations[station.CodingServiceContinuedAvailability]; got != "availability-model" {
		t.Fatalf("continued availability model=%q", got)
	}
	if got := applied.Stations[station.CodingServicePersistenceDestination]; got != "destination-model" {
		t.Fatalf("persistence destination model=%q", got)
	}
}
