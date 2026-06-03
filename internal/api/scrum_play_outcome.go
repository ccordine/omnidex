package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
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
	evidence = strings.TrimSpace(evidence)
	if outcome, ok := parseScrumManagerOutcome(evidence); ok {
		return outcome
	}
	if scrum.IsScrumRawPlay(job.Job.Metadata) || scrum.IsScrumJob(job.Job.Metadata) {
		switch job.Job.Status {
		case model.JobStatusCompleted:
			if scrum.IsStrictScrumExternal(job.Job.Metadata) {
				return scrumStrictExternalPlayCompletedOutcome(job, evidence)
			}
			return ScrumOutcomeSuccess
		case model.JobStatusFailed:
			return ScrumOutcomeFailed
		case model.JobStatusCanceled:
			return ScrumOutcomePaused
		default:
			return ScrumOutcomeInProgress
		}
	}
	switch job.Job.Status {
	case model.JobStatusCompleted:
		return ScrumOutcomeSuccess
	case model.JobStatusFailed:
		return ScrumOutcomeFailed
	case model.JobStatusCanceled:
		return ScrumOutcomePaused
	default:
		return ScrumOutcomeInProgress
	}
}

func scrumBaselinePlayOutcomeFromEvidence(job model.JobDetails, evidence string) ScrumManagerOutcome {
	outcome := resolveScrumManagerOutcomeFromEvidence(job, evidence)
	if job.Job.Status == model.JobStatusCompleted && outcome == ScrumOutcomeInProgress {
		if scrum.IsStrictScrumExternal(job.Job.Metadata) && !scrumAgentOutputHasSubstantiveContent(evidence) {
			return ScrumOutcomePaused
		}
		return ScrumOutcomeSuccess
	}
	return outcome
}

// scrumFinalizeTerminalPlayOutcome maps completed jobs with real agent work to review unless explicitly failed/blocked.
func scrumFinalizeTerminalPlayOutcome(job model.JobDetails, evidence string, outcome ScrumManagerOutcome) ScrumManagerOutcome {
	if !scrumJobStatusTerminal(job.Job.Status) {
		return outcome
	}
	if outcome == ScrumOutcomeBlocked || outcome == ScrumOutcomeFailed {
		return outcome
	}
	if parsed, ok := parseScrumManagerOutcome(evidence); ok {
		switch parsed {
		case ScrumOutcomeBlocked, ScrumOutcomeFailed:
			return parsed
		case ScrumOutcomeSuccess:
			return ScrumOutcomeSuccess
		}
	}
	if job.Job.Status == model.JobStatusCompleted {
		switch outcome {
		case ScrumOutcomeInProgress, ScrumOutcomePaused:
			if scrumAgentOutputHasSubstantiveContent(evidence) {
				return ScrumOutcomeSuccess
			}
		}
	}
	return outcome
}

func (s *Server) resolveScrumPlayOutcomeForCard(ctx context.Context, job model.JobDetails, card ScrumCard) (ScrumManagerOutcome, string) {
	evidence := collectScrumPlayEvidence(job, &card)
	baseline := scrumBaselinePlayOutcomeFromEvidence(job, evidence)
	if !scrumJobStatusTerminal(job.Job.Status) {
		return baseline, ""
	}
	classified, ok := s.classifyScrumAgentOutcomeWithEvidence(ctx, job, baseline, evidence)
	if !ok {
		outcome := scrumFinalizeTerminalPlayOutcome(job, evidence, baseline)
		return outcome, ""
	}
	note := fmt.Sprintf("outcome scan (%s): %s", classified.Outcome, classified.Reason)
	if classified.RealError && classified.Outcome != ScrumOutcomeSuccess {
		note += " (real error)"
	}
	outcome := classified.Outcome
	if stabilized, ok := stabilizeCompletedScrumOutcome(job, baseline, classified); ok {
		outcome = stabilized
		note += " (kept completed job ready for review)"
	}
	outcome = scrumFinalizeTerminalPlayOutcome(job, evidence, outcome)
	if note != "" && outcome != classified.Outcome {
		note += " (completed job advanced to review)"
	}
	return outcome, note
}
