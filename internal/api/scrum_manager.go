package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
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

func syncRunningJobConsoleLog(card ScrumCard, job model.JobDetails) (ScrumCard, bool, error) {
	if err := validateScrumSyncAuthority(card, job); err != nil {
		return card, false, err
	}
	output, err := collectScrumAgentOutput(job)
	if err != nil {
		return card, false, err
	}
	if output == "" {
		return card, false, nil
	}
	syncedLen := card.AgentStreamConsoleCursor
	if syncedLen > int64(len(output)) {
		return card, false, fmt.Errorf("Scrum console cursor %d exceeds exact job output bytes %d", syncedLen, len(output))
	}
	if syncedLen >= int64(len(output)) {
		return card, false, nil
	}
	delta := output[int(syncedLen):]
	if delta == "" {
		return card, false, nil
	}

	updated := card
	if syncedLen == 0 {
		updated.ConsoleLog = appendExactScrumAgentConsole(card.ConsoleLog, "agent stream:\n"+delta)
	} else {
		updated.ConsoleLog = appendExactScrumAgentConsole(card.ConsoleLog, delta)
	}
	updated.AgentStreamConsoleCursor = int64(len(output))
	if syncedChat, ok, err := syncRunningJobChannelChat(updated, job); err != nil {
		return card, false, err
	} else if ok {
		updated = syncedChat
	}
	return updated, true, nil
}

func collectScrumAgentOutput(details model.JobDetails) (string, error) {
	const maxScrumAgentOutputBytes = 4 << 20
	output := ""
	found := false
	for _, step := range details.Steps {
		if step.Action != "external_agent_execute" {
			continue
		}
		if found {
			return "", fmt.Errorf("Scrum agent output requires exactly one external_agent_execute step")
		}
		found = true
		output = step.Output
	}
	if !utf8.ValidString(output) || strings.ContainsRune(output, '\x00') {
		return "", fmt.Errorf("Scrum agent output must be PostgreSQL-compatible UTF-8")
	}
	if len(output) > maxScrumAgentOutputBytes {
		return "", fmt.Errorf("Scrum agent output exceeds the %d-byte limit", maxScrumAgentOutputBytes)
	}
	return output, nil
}

func appendExactScrumAgentConsole(existing, delta string) string {
	if existing == "" || delta == "" || strings.HasSuffix(existing, "\n") {
		return existing + delta
	}
	return existing + "\n" + delta
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

func appendScrumCardContextLines(lines []string, card ScrumCard) []string {
	items := make([]scrum.ChecklistItem, 0, len(card.Checklist))
	for _, item := range card.Checklist {
		items = append(items, scrum.ChecklistItem{ID: item.ID, Text: item.Text, Done: item.Done})
	}
	tests := make([]scrum.ChecklistItem, 0, len(card.TestCriteria))
	for _, item := range card.TestCriteria {
		tests = append(tests, scrum.ChecklistItem{ID: item.ID, Text: item.Text, Done: item.Done})
	}
	return scrum.AppendCardContextLines(lines, scrum.CardContext{
		Description:  card.Description,
		Checklist:    items,
		TestCriteria: tests,
		RefFiles:     card.RefFiles,
		RecipeID:     card.RecipeID,
		RecipeJSON:   string(card.Recipe),
	})
}

func scrumCardContextFromMetadata(raw json.RawMessage) []string {
	return scrum.ContextLinesFromMetadata(raw)
}
