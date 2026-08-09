package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

// reconcileScrumCardJobState fixes cards stuck in running/queued when the linked job already
// finished (or never started). Without this, channel messages only save and play queue stalls.
func (s *Server) reconcileScrumCardJobState(ctx context.Context, projectID int64, card ScrumCard) (ScrumCard, bool, error) {
	jobIDText := strings.TrimSpace(card.JobID)
	if jobIDText == "" {
		if card.PlayState == scrumPlayRunning || card.PlayState == scrumPlayQueued {
			return card, false, fmt.Errorf("Scrum card %q is %q without a job id", card.ID, card.PlayState)
		}
		return card, false, nil
	}
	if s.repo == nil || projectID <= 0 {
		return card, false, fmt.Errorf("postgres repository and project are required to reconcile Scrum card %q", card.ID)
	}
	jobID, err := parseJobID(jobIDText)
	if err != nil {
		return card, false, fmt.Errorf("parse job id for Scrum card %q: %w", card.ID, err)
	}
	job, err := s.repo.CurrentJobDetails(ctx, jobID)
	if err != nil {
		return card, false, fmt.Errorf("load job %d for Scrum card %q: %w", jobID, card.ID, err)
	}

	switch job.Job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		if !scrumCardNeedsTerminalJobReconcile(card) {
			return card, false, nil
		}
		if isScrumAutoReviewJob(job.Job.Metadata) {
			finished, ok, err := s.finishScrumAutoReviewFromContext(ctx, projectID, card, job)
			return finished, ok, err
		}
		card = scrumSyncTerminalPlayOutput(card, job)
		outcome, _ := s.resolveScrumPlayOutcomeForCard(ctx, job, card)
		transition := scrumColumnForOutcome(outcome)
		transition = applyScrumReturnColumn(transition, outcome, job.Job.Metadata)
		card.Column = transition.Column
		card.PlayState = transition.PlayState
		card.QueueOrder = 0
		return card, true, nil
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
		if card.PlayState == scrumPlayQueued {
			card.PlayState = scrumPlayRunning
			card.QueueOrder = 0
			return card, true, nil
		}
		return card, false, nil
	default:
		return card, false, fmt.Errorf("job %d for Scrum card %q has unsupported status %q", jobID, card.ID, job.Job.Status)
	}
}

func (s *Server) prepareScrumCardForChannelDispatch(ctx context.Context, projectID int64, card ScrumCard) (ScrumCard, error) {
	reconciled, changed, err := s.reconcileScrumCardJobState(ctx, projectID, card)
	if err != nil {
		return card, err
	}
	if changed {
		saved, err := s.persistScrumCardFromContext(ctx, projectID, reconciled)
		if err != nil {
			return card, err
		}
		card = saved
	}
	return card, nil
}

func (s *Server) persistScrumCardFromContext(ctx context.Context, projectID int64, card ScrumCard) (ScrumCard, error) {
	if s.repo == nil || projectID <= 0 {
		return ScrumCard{}, fmt.Errorf("postgres repository and project are required for Scrum persistence")
	}
	card.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	current, err := s.repo.GetScrumCard(ctx, projectID, card.ID)
	if err != nil {
		return ScrumCard{}, fmt.Errorf("load Scrum card before persistence: %w", err)
	}
	previous, err := dbScrumCardToAPI(current)
	if err != nil {
		return ScrumCard{}, fmt.Errorf("decode current Scrum card: %w", err)
	}
	patch, err := apiScrumCardToPatch(card)
	if err != nil {
		return ScrumCard{}, err
	}
	updated, err := s.repo.UpdateScrumCard(ctx, projectID, card.ID, patch)
	if err != nil {
		return ScrumCard{}, err
	}
	result, err := dbScrumCardToAPI(updated)
	if err != nil {
		return ScrumCard{}, fmt.Errorf("decode updated Scrum card: %w", err)
	}
	result.FlowMetrics = s.trackScrumCardFlow(ctx, projectID, previous, result, "persist")
	s.notifyScrumCardColumnTransition(ctx, projectID, previous, result)
	return result, nil
}
