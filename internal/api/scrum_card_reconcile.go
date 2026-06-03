package api

import (
	"context"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

// reconcileScrumCardJobState fixes cards stuck in running/queued when the linked job already
// finished (or never started). Without this, channel messages only save and play queue stalls.
func (s *Server) reconcileScrumCardJobState(ctx context.Context, projectID int64, card ScrumCard) (ScrumCard, bool) {
	jobIDText := strings.TrimSpace(card.JobID)
	if jobIDText == "" {
		if card.PlayState == scrumPlayRunning || card.PlayState == scrumPlayQueued {
			card.PlayState = ""
			card.QueueOrder = 0
			return card, true
		}
		return card, false
	}
	if s.repo == nil {
		return card, false
	}
	jobID, err := parseJobID(jobIDText)
	if err != nil {
		return card, false
	}
	job, err := s.repo.GetJobDetails(ctx, jobID)
	if err != nil {
		if card.PlayState == scrumPlayRunning || card.PlayState == scrumPlayQueued {
			card.PlayState = ""
			card.QueueOrder = 0
			return card, true
		}
		return card, false
	}

	switch job.Job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		if !scrumCardNeedsTerminalJobReconcile(card) {
			return card, false
		}
		if isScrumAutoReviewJob(job.Job.Metadata) {
			finished, ok := s.finishScrumAutoReviewFromContext(ctx, projectID, card, job)
			return finished, ok
		}
		card = scrumSyncTerminalPlayOutput(card, job)
		outcome, _ := s.resolveScrumPlayOutcomeForCard(ctx, job, card)
		transition := scrumColumnForOutcome(outcome)
		transition = applyScrumReturnColumn(transition, outcome, job.Job.Metadata)
		card.Column = transition.Column
		card.PlayState = transition.PlayState
		card.QueueOrder = 0
		return card, true
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
		if card.PlayState == scrumPlayQueued {
			card.PlayState = scrumPlayRunning
			card.QueueOrder = 0
			return card, true
		}
	default:
		return card, false
	}
	return card, false
}

func (s *Server) prepareScrumCardForChannelDispatch(ctx context.Context, projectID int64, card ScrumCard) (ScrumCard, error) {
	if reconciled, ok := s.reconcileScrumCardJobState(ctx, projectID, card); ok {
		saved, err := s.persistScrumCardFromContext(ctx, projectID, reconciled)
		if err != nil {
			return card, err
		}
		card = saved
	}
	card = moveScrumCardToInProgress(card)
	if card.PlayState == scrumPlayQueued {
		card.PlayState = ""
		card.QueueOrder = 0
	}
	saved, err := s.persistScrumCardFromContext(ctx, projectID, card)
	if err != nil {
		return card, err
	}
	return saved, nil
}

func (s *Server) persistScrumCardFromContext(ctx context.Context, projectID int64, card ScrumCard) (ScrumCard, error) {
	if s.repo == nil || projectID <= 0 {
		return card, nil
	}
	var previous ScrumCard
	if current, err := s.repo.GetScrumCard(ctx, projectID, card.ID); err == nil {
		previous = dbScrumCardToAPI(current)
	}
	updated, err := s.repo.UpdateScrumCard(ctx, projectID, card.ID, apiScrumCardToPatch(card))
	if err != nil {
		return ScrumCard{}, err
	}
	result := dbScrumCardToAPI(updated)
	s.notifyScrumCardColumnTransition(ctx, projectID, previous, result)
	return result, nil
}
