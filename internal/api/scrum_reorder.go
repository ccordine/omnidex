package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) scrumMoveCard(
	ctx context.Context,
	projectID int64,
	cardID string,
	column queue.ScrumCardColumn,
	beforeCardID string,
	expectedUpdatedAt time.Time,
) (scrumCardMoveResult, error) {
	typedColumn, err := queue.ParseScrumCardColumn(string(column))
	if err != nil {
		return scrumCardMoveResult{}, err
	}
	if beforeCardID != strings.TrimSpace(beforeCardID) {
		return scrumCardMoveResult{}, fmt.Errorf("before card ID must be canonical")
	}

	if s.repo == nil || ctx == nil || projectID <= 0 {
		return scrumCardMoveResult{}, fmt.Errorf("postgres repository is required for Scrum")
	}
	_, updatedStored, err := s.repo.MoveScrumCard(ctx, queue.ScrumCardMove{
		ProjectID: projectID, CardID: cardID, Column: typedColumn, BeforeCardID: beforeCardID,
		ExpectedUpdatedAt: expectedUpdatedAt,
	})
	if err != nil {
		return scrumCardMoveResult{}, err
	}
	updated, err := dbScrumCardToAPI(updatedStored)
	if err != nil {
		return scrumCardMoveResult{}, fmt.Errorf("decode moved Scrum card: %w", err)
	}
	reconcileErr := s.ReconcileScrumPlayQueueForProjectAsync(projectID)
	return scrumCardMoveResult{Card: updated, PostCommitError: reconcileErr}, nil
}

type scrumCardMoveResult struct {
	Card            ScrumCard
	PostCommitError error
}
