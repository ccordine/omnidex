package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/modelconfig"
)

func scrumCardTicketModelFromProject(settings []byte, metaTicketModel, fallback string) (string, error) {
	if modelName := strings.TrimSpace(metaTicketModel); modelName != "" {
		return modelName, nil
	}
	cfg, err := modelconfig.FromSettingsJSON(settings)
	if err != nil {
		return "", fmt.Errorf("parse project model config for Scrum ticket: %w", err)
	}
	return modelconfig.PlannerTicketModel(cfg, fallback)
}
