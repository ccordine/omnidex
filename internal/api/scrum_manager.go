package api

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

type ScrumManagerOutcome string

const (
	ScrumOutcomeSuccess    ScrumManagerOutcome = "success"
	ScrumOutcomeFailed     ScrumManagerOutcome = "failed"
	ScrumOutcomeInProgress ScrumManagerOutcome = "in_progress"
	ScrumOutcomePaused     ScrumManagerOutcome = "paused"
)

type scrumColumnTransition struct {
	Column      string
	PlayState   string
	ConsoleNote string
}

func resolveScrumManagerOutcome(details model.JobDetails) (ScrumManagerOutcome, error) {
	switch details.Job.Status {
	case model.JobStatusFailed, model.JobStatusCanceled:
		return ScrumOutcomeFailed, nil
	case model.JobStatusCompleted:
		return ScrumOutcomeSuccess, nil
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
		return ScrumOutcomeInProgress, nil
	}
	return "", fmt.Errorf("Scrum job has unsupported typed lifecycle status %q", details.Job.Status)
}

func applyScrumReturnColumn(transition scrumColumnTransition, outcome ScrumManagerOutcome, metadata json.RawMessage) scrumColumnTransition {
	returnColumn := scrumReturnColumnFromMetadata(metadata)
	if returnColumn == "" || !scrumManagerAutoAdvance(outcome) {
		return transition
	}
	// Channel-from-review runs return to review after typed job completion.
	if outcome == ScrumOutcomeSuccess && returnColumn == "review" {
		transition.Column = "review"
	}
	return transition
}

func scrumColumnForOutcome(outcome ScrumManagerOutcome) scrumColumnTransition {
	switch outcome {
	case ScrumOutcomeSuccess:
		return scrumColumnTransition{Column: "review", PlayState: "", ConsoleNote: "play: moved to review"}
	case ScrumOutcomeFailed:
		return scrumColumnTransition{Column: "error", PlayState: "", ConsoleNote: "play: moved to error (failed)"}
	case ScrumOutcomePaused:
		return scrumColumnTransition{Column: "assigned", PlayState: scrumPlayPaused, ConsoleNote: "play: returned to assigned (paused)"}
	case ScrumOutcomeInProgress:
		return scrumColumnTransition{Column: "in_progress", PlayState: scrumPlayRunning, ConsoleNote: "play: still in progress"}
	default:
		return scrumColumnTransition{Column: "error", PlayState: "", ConsoleNote: "play: moved to error (invalid outcome)"}
	}
}

func scrumManagerTerminal(outcome ScrumManagerOutcome) bool {
	switch outcome {
	case ScrumOutcomeSuccess, ScrumOutcomeFailed, ScrumOutcomePaused:
		return true
	default:
		return false
	}
}
