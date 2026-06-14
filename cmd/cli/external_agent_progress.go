package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func printExternalAgentStreamUpdatesWithUI(steps []model.Step, offsets map[int64]int, ui *chatUI, maxChars int) bool {
	if offsets == nil {
		return false
	}
	printed := false
	for _, step := range steps {
		if strings.TrimSpace(step.Action) != "external_agent_execute" {
			continue
		}
		output := step.Output
		if output == "" {
			continue
		}
		start := offsets[step.ID]
		if start < 0 || start > len(output) {
			start = 0
		}
		if start == len(output) {
			continue
		}
		offsets[step.ID] = len(output)
		for _, line := range strings.Split(output[start:], "\n") {
			summary := formatExternalAgentCLIEventLine(line, maxChars)
			if summary == "" {
				continue
			}
			if ui != nil {
				emitSystem(ui, summary)
			} else {
				fmt.Printf("  %s\n", summary)
			}
			printed = true
		}
	}
	return printed
}

func formatExternalAgentCLIEventLine(line string, maxChars int) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	var event struct {
		Agent   string   `json:"agent"`
		Type    string   `json:"type"`
		Message string   `json:"message"`
		Command string   `json:"command"`
		Files   []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[") {
			return ""
		}
		return "external agent: " + compactProgressValue(line, maxChars)
	}
	agent := strings.TrimSpace(event.Agent)
	if agent == "" {
		agent = "agent"
	}
	kind := strings.ToLower(strings.TrimSpace(event.Type))
	message := strings.TrimSpace(event.Message)
	switch kind {
	case "started":
		return agent + " started"
	case "status":
		if message == "" {
			return ""
		}
		return agent + " status: " + compactProgressValue(message, maxChars)
	case "thinking", "reasoning":
		if message == "" {
			return ""
		}
		return agent + " thinking: " + compactProgressValue(message, maxChars)
	case "command":
		detail := strings.TrimSpace(event.Command)
		if detail == "" {
			detail = message
		}
		if detail == "" {
			return ""
		}
		return agent + " command: " + compactProgressValue(detail, maxChars)
	case "file_change":
		if len(event.Files) > 0 {
			return agent + " files: " + compactProgressValue(strings.Join(event.Files, ", "), maxChars)
		}
		if message == "" {
			return ""
		}
		return agent + " file change: " + compactProgressValue(message, maxChars)
	case "tool", "mcp_tool_call", "web_search":
		if message == "" {
			return ""
		}
		return agent + " tool: " + compactProgressValue(message, maxChars)
	case "message":
		if message == "" {
			return ""
		}
		return agent + ": " + compactProgressValue(message, maxChars)
	case "error":
		if message == "" {
			message = "external agent reported an error"
		}
		return agent + " error: " + compactProgressValue(message, maxChars)
	case "completed", "turn.completed":
		if message == "" {
			message = "external agent session completed"
		}
		return agent + " completed: " + compactProgressValue(message, maxChars)
	default:
		if message == "" {
			return ""
		}
		if kind == "" {
			kind = "event"
		}
		return agent + " " + kind + ": " + compactProgressValue(message, maxChars)
	}
}
