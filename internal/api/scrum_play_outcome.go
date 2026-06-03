package api

import (
	"context"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func collectScrumPlayEvidence(job model.JobDetails, card *ScrumCard) string {
	parts := []string{collectScrumAgentOutput(job)}
	if card != nil {
		hydrated := hydrateCardChannelChat(*card)
		for _, msg := range hydrated.Chat {
			role := strings.ToLower(strings.TrimSpace(msg.Role))
			if role != "assistant" && role != "system" && role != "error" {
				continue
			}
			content := strings.TrimSpace(msg.Content)
			if content == "" || isScrumChannelNoiseContent(role, content) {
				continue
			}
			parts = append(parts, content)
		}
		if displayLog := strings.TrimSpace(StripAgentStreamMarker(hydrated.ConsoleLog)); displayLog != "" {
			parts = append(parts, displayLog)
		}
	}
	return strings.Join(parts, "\n")
}

func scrumCardNeedsTerminalJobReconcile(card ScrumCard) bool {
	switch card.PlayState {
	case scrumPlayRunning, scrumPlayQueued, scrumPlayReviewing:
		return true
	}
	return normalizeScrumColumn(card.Column) == "in_progress" && strings.TrimSpace(card.JobID) != ""
}

func resolveScrumManagerOutcomeFromEvidence(job model.JobDetails, evidence string) ScrumManagerOutcome {
	return resolveProgrammaticScrumOutcome(job, evidence)
}

func scrumBaselinePlayOutcomeFromEvidence(job model.JobDetails, evidence string) ScrumManagerOutcome {
	return resolveProgrammaticScrumOutcome(job, evidence)
}

func (s *Server) resolveScrumPlayOutcomeForCard(_ context.Context, job model.JobDetails, card ScrumCard) (ScrumManagerOutcome, string) {
	evidence := collectScrumPlayEvidence(job, &card)
	return resolveProgrammaticScrumOutcome(job, evidence), ""
}

func (s *Server) resolveScrumPlayOutcome(ctx context.Context, job model.JobDetails) (ScrumManagerOutcome, string) {
	return s.resolveScrumPlayOutcomeForCard(ctx, job, ScrumCard{})
}
