package queue

import (
	"fmt"
	"strings"
)

func exactScrumCursor(value any, field string) (int64, error) {
	var cursor int64
	switch typed := value.(type) {
	case int64:
		cursor = typed
	case int:
		cursor = int64(typed)
	default:
		return 0, fmt.Errorf("Scrum card %s must be an integer", field)
	}
	if cursor < 0 {
		return 0, fmt.Errorf("Scrum card %s must be non-negative", field)
	}
	return cursor, nil
}

func validateDBScrumCursorAuthority(card DBScrumCard) error {
	jobID := strings.TrimSpace(card.JobID)
	syncJobID := strings.TrimSpace(card.SyncJobID)
	if card.AgentStreamChatCursor < 0 || card.AgentStreamConsoleCursor < 0 || card.StepContextCursor < 0 {
		return fmt.Errorf("Scrum output sync cursors must be non-negative")
	}
	if syncJobID == "" {
		if card.AgentStreamChatCursor != 0 || card.AgentStreamConsoleCursor != 0 || card.StepContextCursor != 0 {
			return fmt.Errorf("Scrum output cursors require an exact sync job ID")
		}
		if strings.TrimSpace(card.PlayState) == "running" ||
			(strings.TrimSpace(card.Column) == "in_progress" && jobID != "") {
			return fmt.Errorf("active Scrum job %q requires durable cursor authority", jobID)
		}
		return nil
	}
	if syncJobID != jobID {
		return fmt.Errorf("Scrum sync job %q differs from card job %q", syncJobID, jobID)
	}
	return nil
}
