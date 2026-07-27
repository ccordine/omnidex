package omni

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func externalAgentResultError(result CursorArchitectAgentResult) error {
	for _, line := range strings.Split(result.Output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		eventType := strings.ToLower(strings.TrimSpace(fmt.Sprint(payload["type"])))
		if eventType == string(AgentEventError) {
			msg := strings.TrimSpace(fmt.Sprint(payload["message"]))
			if msg == "<nil>" {
				msg = ""
			}
			if msg == "" {
				msg = "external agent reported an error"
			}
			return errors.New(msg)
		}
		status := strings.ToUpper(strings.TrimSpace(fmt.Sprint(payload["status"])))
		if status == "ERROR" || status == "FAILED" || status == "CANCELLED" || status == "CANCELED" {
			detail := externalAgentStatusFailureDetail(payload, line)
			return fmt.Errorf("external agent run failed (%s): %s", strings.ToLower(status), detail)
		}
	}
	return nil
}

func externalAgentStatusFailureDetail(payload map[string]any, fallback string) string {
	details := []string{}
	for _, key := range []string{"message", "error", "summary", "rawMessage"} {
		value := strings.TrimSpace(fmt.Sprint(payload[key]))
		if value != "" && value != "<nil>" {
			details = append(details, value)
		}
	}
	runID := strings.TrimSpace(fmt.Sprint(payload["run_id"]))
	if runID == "" || runID == "<nil>" {
		runID = strings.TrimSpace(fmt.Sprint(payload["id"]))
	}
	agentID := strings.TrimSpace(fmt.Sprint(payload["agent_id"]))
	if len(details) == 0 {
		if runID != "" && runID != "<nil>" {
			details = append(details, "run_id="+runID)
		}
		if agentID != "" && agentID != "<nil>" {
			details = append(details, "agent_id="+agentID)
		}
	}
	if len(details) == 0 {
		details = append(details, strings.TrimSpace(fallback))
	}
	detail := strings.Join(details, "; ")
	if detail == "" {
		return "external agent ended with an error status"
	}
	return detail
}

// ExternalAgentResultError reports whether an external agent run failed.
func ExternalAgentResultError(result CursorArchitectAgentResult) error {
	return externalAgentResultError(result)
}
