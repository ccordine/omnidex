package omni

import (
	"encoding/json"
	"fmt"
	"strings"
)

func externalAgentResultError(result CursorArchitectAgentResult) error {
	for _, line := range strings.Split(result.Output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event AgentEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(event.Type), "error") {
			msg := strings.TrimSpace(event.Message)
			if msg == "" {
				msg = "external agent reported an error"
			}
			return fmt.Errorf("%s", msg)
		}
	}

	for _, line := range strings.Split(result.Output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		status := strings.ToUpper(strings.TrimSpace(fmt.Sprint(payload["status"])))
		if status == "ERROR" || status == "FAILED" || status == "CANCELLED" {
			detail := strings.TrimSpace(fmt.Sprint(payload["message"]))
			if detail == "" {
				detail = line
			}
			return fmt.Errorf("cursor agent run failed (%s): %s", strings.ToLower(status), detail)
		}
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(payload["type"])), "status") &&
			status == "ERROR" {
			return fmt.Errorf("cursor agent run failed: %s", line)
		}
	}

	combined := strings.ToLower(result.Summary + "\n" + result.Output)
	if strings.Contains(combined, `"status":"error"`) ||
		strings.Contains(combined, `"status": "error"`) ||
		strings.Contains(combined, `"status":"error"`) {
		return fmt.Errorf("cursor agent run failed: %s", strings.TrimSpace(firstNonEmpty(result.Summary, result.Output)))
	}
	return nil
}

// ExternalAgentResultError reports whether an external agent run failed.
func ExternalAgentResultError(result CursorArchitectAgentResult) error {
	return externalAgentResultError(result)
}
