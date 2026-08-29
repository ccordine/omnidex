package api

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func validateChatJobPresentation(presentation queue.JobPresentation) error {
	if err := validateChatJob(presentation.Job); err != nil {
		return err
	}
	if presentation.Job.CurrentGeneration <= 0 ||
		presentation.Progress.JobID != presentation.Job.ID ||
		presentation.Progress.Generation != presentation.Job.CurrentGeneration {
		return fmt.Errorf("chat job presentation has mismatched current-generation progress authority")
	}
	if len(presentation.Steps) == 0 || len(presentation.Steps) > 128 {
		return fmt.Errorf("chat job presentation requires between 1 and 128 current steps")
	}
	var priorSortIndex = -1
	var priorID int64
	for _, step := range presentation.Steps {
		if step.ID <= 0 || step.JobID != presentation.Job.ID ||
			step.Generation != presentation.Job.CurrentGeneration || step.SupersededAtGeneration != nil ||
			step.SortIndex < 0 || (step.SortIndex == priorSortIndex && step.ID <= priorID) ||
			step.SortIndex < priorSortIndex || step.CreatedAt.IsZero() || step.UpdatedAt.IsZero() ||
			strings.TrimSpace(step.Action) == "" {
			return fmt.Errorf("chat job presentation step %d has invalid current authority", step.ID)
		}
		switch step.Status {
		case model.StepStatusPending, model.StepStatusRunning, model.StepStatusCompleted,
			model.StepStatusFailed, model.StepStatusWaiting, model.StepStatusCanceled:
		default:
			return fmt.Errorf("chat job presentation step %d has unsupported status %q", step.ID, step.Status)
		}
		priorSortIndex, priorID = step.SortIndex, step.ID
	}
	return nil
}
