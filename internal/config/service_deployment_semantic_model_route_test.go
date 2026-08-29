package config

import (
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestServiceDeploymentSemanticModelsHaveSeparateAuthority(t *testing.T) {
	t.Setenv("OMNI_CODING_REQUIREMENTS_MODEL", "requirements-model")
	t.Setenv("OMNI_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL", "deployment-model")

	models := loadStationModels(Config{})
	if got := models[station.CodingRequirements]; got != "requirements-model" {
		t.Fatalf("requirements model=%q", got)
	}
	if got := models[station.CodingServiceContinuedAvailability]; got != "deployment-model" {
		t.Fatalf("continued availability model=%q", got)
	}
	if got := models[station.CodingServicePersistenceDestination]; got != "deployment-model" {
		t.Fatalf("persistence destination model=%q", got)
	}
}

func TestServiceDeploymentSemanticModelsDoNotFallBack(t *testing.T) {
	t.Setenv("OMNI_CODING_REQUIREMENTS_MODEL", "requirements-model")
	t.Setenv("OMNI_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL", "")

	models := loadStationModels(Config{})
	for _, id := range []station.ID{
		station.CodingServiceContinuedAvailability,
		station.CodingServicePersistenceDestination,
	} {
		if _, exists := models[id]; exists {
			t.Fatalf("service deployment station %s inherited another route: %+v", id, models)
		}
	}
}
