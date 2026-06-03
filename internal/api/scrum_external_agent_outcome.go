package api

import (
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

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
