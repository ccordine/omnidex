package queue

import (
	"fmt"
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
	if card.StepContextCursor < 0 {
		return fmt.Errorf("Scrum step-context cursor must be non-negative")
	}
	if card.SyncJobID == "" {
		if card.StepContextCursor != 0 {
			return fmt.Errorf("Scrum step-context cursor requires an exact sync job ID")
		}
		if card.PlayState == "running" || (card.Column == "in_progress" && card.JobID != "") {
			return fmt.Errorf("active Scrum job %q requires durable cursor authority", card.JobID)
		}
		return nil
	}
	if card.SyncJobID != card.JobID {
		return fmt.Errorf("Scrum sync job %q differs from card job %q", card.SyncJobID, card.JobID)
	}
	return nil
}
