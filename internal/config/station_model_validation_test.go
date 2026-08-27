package config

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestCompleteRuntimeStationRoutingFailsClosed(t *testing.T) {
	models := make(map[station.ID]string)
	for _, id := range station.All() {
		if id == station.RoleplayCanonExtraction || id == station.RoleplayOngoingAction {
			continue
		}
		models[id] = "qualified-model"
	}
	cfg := Config{StationModels: models, RoleplaySemanticModel: "roleplay-model"}
	if err := validateCompleteStationModelRouting(cfg); err != nil {
		t.Fatal(err)
	}

	delete(cfg.StationModels, station.CodingServiceContinuedAvailability)
	err := validateCompleteStationModelRouting(cfg)
	if err == nil || !strings.Contains(err.Error(), string(station.CodingServiceContinuedAvailability)) {
		t.Fatalf("missing continued-availability route error=%v", err)
	}
	cfg.StationModels[station.CodingServiceContinuedAvailability] = "qualified-model"
	delete(cfg.StationModels, station.CodingServicePersistenceDestination)
	err = validateCompleteStationModelRouting(cfg)
	if err == nil || !strings.Contains(err.Error(), string(station.CodingServicePersistenceDestination)) {
		t.Fatalf("missing persistence-destination route error=%v", err)
	}
	cfg.StationModels[station.CodingServicePersistenceDestination] = "qualified-model"
	cfg.RoleplaySemanticModel = ""
	err = validateCompleteStationModelRouting(cfg)
	if err == nil || !strings.Contains(err.Error(), "roleplay semantic model") {
		t.Fatalf("missing roleplay route error=%v", err)
	}
}

func TestCompleteRuntimeStationRoutingRejectsUnregisteredAuthority(t *testing.T) {
	cfg := Config{
		StationModels:         map[station.ID]string{station.ID("unknown_station"): "model"},
		RoleplaySemanticModel: "roleplay-model",
	}
	err := validateCompleteStationModelRouting(cfg)
	if err == nil || !strings.Contains(err.Error(), "unregistered semantic station") {
		t.Fatalf("unregistered station route error=%v", err)
	}
}
