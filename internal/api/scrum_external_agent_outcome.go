package api

import (
	"encoding/json"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func scrumAgentOutputIndicatesRunFailure(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event struct {
			Type   string          `json:"type"`
			Status string          `json:"status"`
			Raw    json.RawMessage `json:"raw"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(event.Type), "error") || externalAgentTerminalFailureStatus(event.Status) {
			return true
		}
		if len(event.Raw) > 0 {
			var rawStatus struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(event.Raw, &rawStatus) == nil && externalAgentTerminalFailureStatus(rawStatus.Status) {
				return true
			}
		}
	}
	return false
}

func externalAgentTerminalFailureStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "error", "failed", "cancelled", "canceled":
		return true
	default:
		return false
	}
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
