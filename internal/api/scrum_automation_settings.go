package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type scrumAutomationSettings struct {
	AutoWork     ScrumAutoWorkConfig
	AutoReview   ScrumAutoReviewConfig
	CreateTicket ScrumCreateTicketConfig
}

func loadScrumAutomationSettings(settings json.RawMessage) (scrumAutomationSettings, error) {
	autoWork, err := loadScrumAutoWorkConfig(settings)
	if err != nil {
		return scrumAutomationSettings{}, err
	}
	autoReview, err := loadScrumAutoReviewConfig(settings)
	if err != nil {
		return scrumAutomationSettings{}, err
	}
	createTicket, err := loadScrumCreateTicketConfig(settings)
	if err != nil {
		return scrumAutomationSettings{}, err
	}
	return scrumAutomationSettings{
		AutoWork:     autoWork,
		AutoReview:   autoReview,
		CreateTicket: createTicket,
	}, nil
}

func (s *Server) scrumAutomationSettings(ctx context.Context, projectID int64) (scrumAutomationSettings, error) {
	if s == nil || s.repo == nil || projectID <= 0 {
		return scrumAutomationSettings{}, fmt.Errorf("postgres repository and project are required to load Scrum automation settings")
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return scrumAutomationSettings{}, fmt.Errorf("load project %d for Scrum automation settings: %w", projectID, err)
	}
	return loadScrumAutomationSettings(project.Settings)
}
