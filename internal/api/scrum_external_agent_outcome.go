package api

import (
	"encoding/json"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

// scrumExternalAgentSessionBoilerplate matches default Codex/Cursor lifecycle lines that
// must not alone trigger a review transition before real agent work is visible.
var scrumExternalAgentSessionBoilerplate = map[string]struct{}{
	"codex external implementation session started":     {},
	"codex external implementation session completed":     {},
	"cursor external implementation session started":      {},
	"cursor external implementation session completed":    {},
	"external agent session ended":                        {},
	"external agent session completed":                    {},
	"external agent completed":                            {},
	"codex turn completed":                                {},
}

type scrumAgentStreamEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Command string `json:"command"`
}

func isScrumExternalAgentBoilerplateMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return true
	}
	if _, ok := scrumExternalAgentSessionBoilerplate[lower]; ok {
		return true
	}
	if isScrumChannelNoiseContent("assistant", message) {
		return true
	}
	return false
}

func scrumAgentOutputIndicatesRunFailure(output string) bool {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "enoent") && strings.Contains(lower, "spawn") {
		return true
	}
	if strings.Contains(lower, "strict external agent required") {
		return true
	}
	if strings.Contains(lower, `"type":"error"`) || strings.Contains(lower, `"type": "error"`) {
		return true
	}
	if strings.Contains(lower, `"status":"error"`) || strings.Contains(lower, `"status": "error"`) {
		return true
	}
	return false
}

func scrumAgentOutputHasSubstantiveContent(output string) bool {
	output = strings.TrimSpace(output)
	if output == "" {
		return false
	}
	if _, ok := parseScrumManagerOutcome(output); ok {
		return true
	}
	if scrumAgentOutputIndicatesRunFailure(output) {
		return true
	}

	substantive := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event scrumAgentStreamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			if !isScrumExternalAgentBoilerplateMessage(line) {
				substantive = true
			}
			continue
		}
		eventType := strings.ToLower(strings.TrimSpace(event.Type))
		switch eventType {
		case "started", "status":
			continue
		case "completed":
			if !isScrumExternalAgentBoilerplateMessage(event.Message) {
				substantive = true
			}
		case "error":
			if strings.TrimSpace(event.Message) != "" {
				substantive = true
			}
		case "message", "thinking", "reasoning", "tool", "command", "file_change", "turn.completed", "mcp_tool_call", "web_search":
			if strings.TrimSpace(event.Message) != "" || strings.TrimSpace(event.Command) != "" {
				substantive = true
			}
		default:
			if !isScrumExternalAgentBoilerplateMessage(event.Message) && strings.TrimSpace(event.Message) != "" {
				substantive = true
			}
		}
		if substantive {
			return true
		}
	}
	return substantive
}

func scrumStrictExternalPlayCompletedOutcome(details model.JobDetails, combinedOutput string) ScrumManagerOutcome {
	if scrumAgentOutputIndicatesRunFailure(combinedOutput) {
		return ScrumOutcomePaused
	}
	if !scrumAgentOutputHasSubstantiveContent(combinedOutput) {
		return ScrumOutcomePaused
	}
	return ScrumOutcomeSuccess
}

func scrumSyncTerminalPlayOutput(card ScrumCard, job model.JobDetails) ScrumCard {
	updated := card
	if synced, ok := syncRunningJobChannelChat(updated, job); ok {
		updated = synced
	}
	if synced, ok := syncRunningJobConsoleLog(updated, job); ok {
		updated = synced
	}
	return updated
}
