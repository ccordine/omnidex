package api

import (
	"context"
	"fmt"
)

func (s *Server) scrumEditCard(
	ctx context.Context,
	projectID int64,
	cardID string,
	edit scrumCardEditRequest,
) (ScrumCard, error) {
	if s.repo == nil || ctx == nil || projectID <= 0 {
		return ScrumCard{}, fmt.Errorf("postgres repository is required for Scrum")
	}
	if !edit.hasEditableField() {
		return ScrumCard{}, fmt.Errorf("Scrum card edit requires at least one editable field")
	}
	if err := edit.validate(); err != nil {
		return ScrumCard{}, err
	}
	patch := edit.repositoryPatch()
	updated, err := s.repo.UpdateScrumCardAtRevision(
		ctx, projectID, cardID, edit.ExpectedUpdatedAt.Value, patch,
	)
	if err != nil {
		return ScrumCard{}, err
	}
	result, err := dbScrumCardToAPI(updated)
	if err != nil {
		return ScrumCard{}, fmt.Errorf("decode updated Scrum card: %w", err)
	}
	return result, nil
}
