package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/station"
)

func validateCompleteStationModelRouting(cfg Config) error {
	configuredIDs := make([]string, 0, len(cfg.StationModels))
	for id := range cfg.StationModels {
		configuredIDs = append(configuredIDs, string(id))
	}
	sort.Strings(configuredIDs)
	for _, value := range configuredIDs {
		id := station.ID(value)
		if err := id.Validate(); err != nil {
			return err
		}
		if isRoleplaySemanticStation(id) {
			return fmt.Errorf(
				"semantic station %q must use the separate roleplay semantic model authority",
				id,
			)
		}
	}

	for _, id := range station.All() {
		if isRoleplaySemanticStation(id) {
			continue
		}
		if strings.TrimSpace(cfg.StationModels[id]) == "" {
			return fmt.Errorf("semantic station %q has no configured model", id)
		}
	}
	if strings.TrimSpace(cfg.RoleplaySemanticModel) == "" {
		return fmt.Errorf("roleplay semantic model is not configured")
	}
	return nil
}

func isRoleplaySemanticStation(id station.ID) bool {
	return id == station.RoleplayCanonExtraction || id == station.RoleplayOngoingAction
}
