package omni

import (
	"encoding/json"
	"fmt"
	"strings"
)

func externalAgentResultError(result CursorArchitectAgentResult) error {
	if err := externalAgentPlainTextFailure(result.Output); err != nil {
		return err
	}
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
			detail := externalAgentStatusFailureDetail(payload, line)
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

func externalAgentPlainTextFailure(output string) error {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "enoent") && strings.Contains(lower, "spawn ") {
		return fmt.Errorf("external agent process failed: %s", strings.TrimSpace(output))
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
