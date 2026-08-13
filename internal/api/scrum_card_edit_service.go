package api

import (
	"fmt"
	"net/http"
)

func (s *Server) scrumEditCard(
	r *http.Request,
	cardID string,
	edit scrumCardEditRequest,
) (ScrumCard, error) {
	if s.repo == nil {
		return ScrumCard{}, fmt.Errorf("postgres repository is required for Scrum")
	}
	if !edit.hasEditableField() {
		return ScrumCard{}, fmt.Errorf("Scrum card edit requires at least one editable field")
	}
	if err := edit.validate(); err != nil {
		return ScrumCard{}, err
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		return ScrumCard{}, err
	}
	patch, err := edit.repositoryPatch()
	if err != nil {
		return ScrumCard{}, err
	}
	current, err := s.repo.GetScrumCard(r.Context(), projectID, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	previous, err := dbScrumCardToAPI(current)
	if err != nil {
		return ScrumCard{}, fmt.Errorf("decode current Scrum card: %w", err)
	}
	updated, err := s.repo.UpdateScrumCard(r.Context(), projectID, cardID, patch)
	if err != nil {
		return ScrumCard{}, err
	}
	result, err := dbScrumCardToAPI(updated)
	if err != nil {
		return ScrumCard{}, fmt.Errorf("decode updated Scrum card: %w", err)
	}
	result.FlowMetrics = s.trackScrumCardFlow(r.Context(), projectID, previous, result, "edit")
	return result, nil
}
