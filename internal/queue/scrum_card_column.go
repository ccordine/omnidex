package queue

import "fmt"

// ScrumCardColumn is one code-owned physical board state. Transport adapters
// must parse it exactly; spelling aliases and whitespace normalization would
// give clients an unregistered transition vocabulary.
type ScrumCardColumn string

const (
	MaxScrumCardIDBytes = 256

	ScrumCardBacklog    ScrumCardColumn = "backlog"
	ScrumCardReady      ScrumCardColumn = "ready"
	ScrumCardAssigned   ScrumCardColumn = "assigned"
	ScrumCardInProgress ScrumCardColumn = "in_progress"
	ScrumCardReview     ScrumCardColumn = "review"
	ScrumCardBlocked    ScrumCardColumn = "blocked"
	ScrumCardError      ScrumCardColumn = "error"
	ScrumCardDone       ScrumCardColumn = "done"
)

func ParseScrumCardColumn(value string) (ScrumCardColumn, error) {
	column := ScrumCardColumn(value)
	switch column {
	case ScrumCardBacklog, ScrumCardReady, ScrumCardAssigned, ScrumCardInProgress,
		ScrumCardReview, ScrumCardBlocked, ScrumCardError, ScrumCardDone:
		return column, nil
	default:
		return "", fmt.Errorf("Scrum card column %q is not registered", value)
	}
}
