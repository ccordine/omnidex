package api

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) persistScrumCardTransition(
	ctx context.Context,
	projectID int64,
	previous ScrumCard,
	next ScrumCard,
	kind queue.ScrumCardReconcileKind,
) (ScrumCard, error) {
	if s.repo == nil || projectID <= 0 {
		return ScrumCard{}, fmt.Errorf("postgres repository and project are required for Scrum persistence")
	}
	if previous.ID == "" || previous.ID != next.ID {
		return ScrumCard{}, fmt.Errorf("Scrum persistence requires one unchanged card identity")
	}
	expectedUpdatedAt, err := time.Parse(time.RFC3339Nano, previous.UpdatedAt)
	if err != nil {
		return ScrumCard{}, fmt.Errorf("parse observed Scrum card revision: %w", err)
	}
	column, err := queue.ParseScrumCardColumn(next.Column)
	if err != nil {
		return ScrumCard{}, err
	}
	messages, err := scrumChannelMessageAppends(next.PendingChannelMessages)
	if err != nil {
		return ScrumCard{}, err
	}
	updated, err := s.repo.ReconcileScrumCard(ctx, queue.ScrumCardReconcileCommand{
		ProjectID: projectID, CardID: previous.ID, ExpectedUpdatedAt: expectedUpdatedAt,
		ExpectedJobID: previous.JobID, Kind: kind, Column: column,
		PlayState: next.PlayState, QueueOrder: next.QueueOrder,
		JobID: next.JobID,
		Messages: messages,
	})
	if err != nil {
		return ScrumCard{}, err
	}
	result, err := dbScrumCardToAPI(updated)
	if err != nil {
		return ScrumCard{}, fmt.Errorf("decode updated Scrum card: %w", err)
	}
	s.notifyScrumCardColumnTransition(ctx, projectID, previous, result)
	return result, nil
}
