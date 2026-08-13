package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func validateScrumSyncAuthority(card ScrumCard, job model.JobDetails) error {
	if job.Job.ID <= 0 {
		return fmt.Errorf("Scrum output sync requires a positive typed job ID")
	}
	jobID := strconv.FormatInt(job.Job.ID, 10)
	if strings.TrimSpace(card.JobID) != jobID {
		return fmt.Errorf("Scrum output sync job %s differs from card job %q", jobID, card.JobID)
	}
	if strings.TrimSpace(card.SyncJobID) != jobID {
		return fmt.Errorf("Scrum output sync job %s differs from durable cursor authority %q", jobID, card.SyncJobID)
	}
	if card.AgentStreamChatCursor < 0 || card.AgentStreamConsoleCursor < 0 || card.StepContextCursor < 0 {
		return fmt.Errorf("Scrum output sync cursors must be non-negative")
	}
	return nil
}

func validateScrumCardSyncState(card ScrumCard) error {
	jobID := strings.TrimSpace(card.JobID)
	syncJobID := strings.TrimSpace(card.SyncJobID)
	if card.AgentStreamChatCursor < 0 || card.AgentStreamConsoleCursor < 0 || card.StepContextCursor < 0 {
		return fmt.Errorf("Scrum output sync cursors must be non-negative")
	}
	if syncJobID == "" {
		if card.AgentStreamChatCursor != 0 || card.AgentStreamConsoleCursor != 0 || card.StepContextCursor != 0 {
			return fmt.Errorf("Scrum output cursors require an exact sync job ID")
		}
		if card.PlayState == scrumPlayRunning || (normalizeScrumColumn(card.Column) == "in_progress" && jobID != "") {
			return fmt.Errorf("active Scrum job %q requires durable cursor authority", jobID)
		}
		return nil
	}
	if syncJobID != jobID {
		return fmt.Errorf("Scrum sync job %q differs from card job %q", syncJobID, jobID)
	}
	return nil
}
