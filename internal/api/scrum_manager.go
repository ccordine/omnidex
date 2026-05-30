package api

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
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
var agentStreamLenPattern = regexp.MustCompile(`(?m)^\[\[agent-stream-len:\d+\]\]\s*$`)
var agentStreamLenValuePattern = regexp.MustCompile(`\[\[agent-stream-len:(\d+)\]\]`)

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
	return "", false
}

// StripAgentStreamMarker removes internal sync markers from card console_log for display.
func StripAgentStreamMarker(consoleLog string) string {
	return strings.TrimSpace(agentStreamLenPattern.ReplaceAllString(consoleLog, ""))
}

func syncedAgentStreamLen(consoleLog string) int {
	match := agentStreamLenValuePattern.FindStringSubmatch(consoleLog)
	if len(match) < 2 {
		return 0
	}
	n, err := strconv.Atoi(match[1])
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func syncRunningJobConsoleLog(card ScrumCard, job model.JobDetails) (ScrumCard, bool) {
	output := collectScrumAgentOutput(job)
	if strings.TrimSpace(output) == "" {
		return card, false
	}
	syncedLen := syncedAgentStreamLen(card.ConsoleLog)
	if syncedLen >= len(output) {
		return card, false
	}
	delta := output[syncedLen:]
	if strings.TrimSpace(delta) == "" {
		return card, false
	}

	baseLog := StripAgentStreamMarker(card.ConsoleLog)
	updated := card
	if syncedLen == 0 {
		updated.ConsoleLog = appendScrumConsole(baseLog, "agent stream:\n"+delta)
	} else {
		updated.ConsoleLog = appendScrumConsole(baseLog, delta)
	}
	updated.ConsoleLog = strings.TrimRight(updated.ConsoleLog, "\n") + fmt.Sprintf("\n[[agent-stream-len:%d]]\n", len(output))
	if syncedChat, ok := syncRunningJobChannelChat(updated, job); ok {
		updated = syncedChat
	}
	return updated, true
}

func collectScrumAgentOutput(details model.JobDetails) string {
	parts := []string{}
	for _, step := range details.Steps {
		if output := strings.TrimSpace(sanitizeScrumChannelText(step.Output)); output != "" {
			parts = append(parts, output)
		}
		if errText := strings.TrimSpace(sanitizeScrumChannelText(step.Error)); errText != "" {
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
		case model.JobStatusFailed, model.JobStatusCanceled:
			// Return to assigned so the user can inspect output and retry; blocked only when SCRUM_STATUS says so.
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

func applyScrumReturnColumn(transition scrumColumnTransition, outcome ScrumManagerOutcome, metadata json.RawMessage) scrumColumnTransition {
	returnColumn := scrumReturnColumnFromMetadata(metadata)
	if returnColumn == "" || !scrumManagerAutoAdvance(outcome) {
		return transition
	}
	// Channel-from-review runs should land back in review even if an older build
	// moved the card to in_progress or the agent emitted SCRUM_STATUS: in_progress.
	if outcome == ScrumOutcomeSuccess && returnColumn == "review" {
		transition.Column = "review"
	}
	return transition
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
