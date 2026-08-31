package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

// reconcileScrumCardJobState fixes cards stuck in running/queued when the linked job already
// finished (or never started). Without this, channel messages only save and play queue stalls.
func (s *Server) reconcileScrumCardJobState(ctx context.Context, projectID int64, card ScrumCard) (ScrumCard, bool, ScrumManagerOutcome, error) {
	jobIDText := card.JobID
	if jobIDText != strings.TrimSpace(jobIDText) {
		return card, false, "", fmt.Errorf("Scrum card %q has noncanonical job id %q", card.ID, card.JobID)
	}
	if jobIDText == "" {
		if card.PlayState == scrumPlayRunning || card.PlayState == scrumPlayQueued {
			return card, false, "", fmt.Errorf("Scrum card %q is %q without a job id", card.ID, card.PlayState)
		}
		return card, false, "", nil
	}
	if s.repo == nil || projectID <= 0 {
		return card, false, "", fmt.Errorf("postgres repository and project are required to reconcile Scrum card %q", card.ID)
	}
	jobID, err := parseJobID(jobIDText)
	if err != nil {
		return card, false, "", fmt.Errorf("parse job id for Scrum card %q: %w", card.ID, err)
	}
	job, err := s.repo.CurrentJobDetails(ctx, jobID)
	if err != nil {
		return card, false, "", fmt.Errorf("load job %d for Scrum card %q: %w", jobID, card.ID, err)
	}

	switch job.Job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		if !scrumCardNeedsTerminalJobReconcile(card) {
			return card, false, "", nil
		}
		outcome, err := s.resolveScrumPlayOutcomeForCard(ctx, job, card)
		if err != nil {
			return card, false, "", err
		}
		transition := scrumColumnForOutcome(outcome)
		transition = applyScrumReturnColumn(transition, outcome, job.Job.Metadata)
		card.Column = transition.Column
		card.PlayState = transition.PlayState
		card.QueueOrder = 0
		return card, true, outcome, nil
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
		if card.PlayState == scrumPlayQueued {
			return card, false, "", fmt.Errorf("Scrum queued card %q unexpectedly owns active job %d", card.ID, jobID)
		}
		return card, false, "", nil
	default:
		return card, false, "", fmt.Errorf("job %d for Scrum card %q has unsupported status %q", jobID, card.ID, job.Job.Status)
	}
}

func (s *Server) persistScrumCardTransition(
	ctx context.Context,
	projectID int64,
	previous ScrumCard,
	next ScrumCard,
	kind queue.ScrumCardReconcileKind,
	outcome ScrumManagerOutcome,
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
		Messages: messages, Outcome: string(outcome),
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
