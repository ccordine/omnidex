package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func scrumCardNeedsTerminalJobReconcile(card ScrumCard) bool {
	switch card.PlayState {
	case scrumPlayRunning, scrumPlayQueued:
		return true
	}
	return normalizeScrumColumn(card.Column) == "in_progress" && strings.TrimSpace(card.JobID) != ""
}

func (s *Server) resolveScrumPlayOutcomeForCard(_ context.Context, job model.JobDetails, card ScrumCard) (ScrumManagerOutcome, error) {
	if strings.TrimSpace(card.JobID) != "" {
		if err := validateScrumSyncAuthority(card, job); err != nil {
			return "", fmt.Errorf("resolve Scrum play outcome: %w", err)
		}
	}
	return resolveScrumManagerOutcome(job)
}

func (s *Server) resolveScrumPlayOutcome(ctx context.Context, job model.JobDetails) (ScrumManagerOutcome, error) {
	return s.resolveScrumPlayOutcomeForCard(ctx, job, ScrumCard{})
}

func scrumSyncTerminalPlayOutput(card ScrumCard, job model.JobDetails) (ScrumCard, error) {
	updated := card
	if synced, ok, err := syncRunningJobChannelChat(updated, job); err != nil {
		return card, err
	} else if ok {
		updated = synced
	}
	return updated, nil
}
