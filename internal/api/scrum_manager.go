package api

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
)

type ScrumManagerOutcome string

const (
	ScrumOutcomeSuccess    ScrumManagerOutcome = "success"
	ScrumOutcomeFailed     ScrumManagerOutcome = "failed"
	ScrumOutcomeBlocked    ScrumManagerOutcome = "blocked"
	ScrumOutcomeInProgress ScrumManagerOutcome = "in_progress"
	ScrumOutcomePaused     ScrumManagerOutcome = "paused"
)

var scrumStatusLinePattern = regexp.MustCompile(`(?im)^SCRUM_STATUS:\s*(success|failed|blocked|in_progress)\s*$`)
var scrumStatusJSONPattern = regexp.MustCompile(`(?i)"scrum_status"\s*:\s*"(success|failed|blocked|in_progress)"`)

type scrumColumnTransition struct {
	Column      string
	PlayState   string
	ConsoleNote string
}

func parseScrumManagerOutcome(text string) (ScrumManagerOutcome, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	if match := scrumStatusLinePattern.FindStringSubmatch(text); len(match) > 1 {
		return ScrumManagerOutcome(strings.ToLower(match[1])), true
	}
	if match := scrumStatusJSONPattern.FindStringSubmatch(text); len(match) > 1 {
		return ScrumManagerOutcome(strings.ToLower(match[1])), true
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "scrum status: blocked"):
		return ScrumOutcomeBlocked, true
	}
	return "", false
}

func collectScrumAgentOutput(details model.JobDetails) string {
	parts := []string{}
	for _, step := range details.Steps {
		if output := strings.TrimSpace(step.Output); output != "" {
			parts = append(parts, output)
		}
		if errText := strings.TrimSpace(step.Error); errText != "" {
			parts = append(parts, errText)
		}
	}
	return strings.Join(parts, "\n")
}

func resolveScrumManagerOutcome(details model.JobDetails) ScrumManagerOutcome {
	combined := collectScrumAgentOutput(details)
	if outcome, ok := parseScrumManagerOutcome(combined); ok {
		return outcome
	}
	if scrum.IsScrumRawPlay(details.Job.Metadata) || scrum.IsScrumJob(details.Job.Metadata) {
		switch details.Job.Status {
		case model.JobStatusCompleted:
			return ScrumOutcomeSuccess
		case model.JobStatusFailed:
			return ScrumOutcomeBlocked
		case model.JobStatusCanceled:
			return ScrumOutcomePaused
		default:
			return ScrumOutcomeInProgress
		}
	}
	switch details.Job.Status {
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

func scrumColumnForOutcome(outcome ScrumManagerOutcome) scrumColumnTransition {
	switch outcome {
	case ScrumOutcomeSuccess:
		return scrumColumnTransition{Column: "review", PlayState: "", ConsoleNote: "play: moved to review"}
	case ScrumOutcomeBlocked:
		return scrumColumnTransition{Column: "blocked", PlayState: "", ConsoleNote: "play: moved to blocked"}
	case ScrumOutcomeFailed:
		return scrumColumnTransition{Column: "blocked", PlayState: "", ConsoleNote: "play: moved to blocked (failed)"}
	case ScrumOutcomePaused:
		return scrumColumnTransition{Column: "assigned", PlayState: scrumPlayPaused, ConsoleNote: "play: returned to assigned (paused)"}
	case ScrumOutcomeInProgress:
		return scrumColumnTransition{Column: "in_progress", PlayState: scrumPlayRunning, ConsoleNote: "play: still in progress"}
	default:
		return scrumColumnTransition{Column: "review", PlayState: "", ConsoleNote: "play: moved to review"}
	}
}

func scrumManagerTerminal(outcome ScrumManagerOutcome) bool {
	switch outcome {
	case ScrumOutcomeSuccess, ScrumOutcomeBlocked, ScrumOutcomeFailed, ScrumOutcomePaused:
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
		JiraTicket:   card.JiraTicket,
		Checklist:    items,
		TestCriteria: tests,
		Tags:         card.Tags,
		RefFiles:     card.RefFiles,
		RecipeID:     card.RecipeID,
		RecipeJSON:   string(card.Recipe),
	})
}

func scrumCardContextFromMetadata(raw json.RawMessage) []string {
	return scrum.ContextLinesFromMetadata(raw)
}
