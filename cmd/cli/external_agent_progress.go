package main

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/agentstream"
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
	if strings.TrimSpace(line) == "" {
		return ""
	}
	event, err := agentstream.DecodeLine(line)
	if err != nil {
		return "external agent stream rejected: " + compactProgressValue(err.Error(), maxChars)
	}
	agent := event.Agent
	message := strings.TrimSpace(event.Message)
	switch event.Type {
	case agentstream.EventStarted:
		return agent + " started"
	case agentstream.EventStatus:
		if message == "" {
			return ""
		}
		return agent + " status: " + compactProgressValue(message, maxChars)
	case agentstream.EventThinking:
		if message == "" {
			return ""
		}
		return agent + " thinking: " + compactProgressValue(message, maxChars)
	case agentstream.EventCommand:
		detail := strings.TrimSpace(event.Command)
		if detail == "" {
			detail = message
		}
		if detail == "" {
			return ""
		}
		return agent + " command: " + compactProgressValue(detail, maxChars)
	case agentstream.EventFileChange:
		if len(event.Files) > 0 {
			return agent + " files: " + compactProgressValue(strings.Join(event.Files, ", "), maxChars)
		}
		if message == "" {
			return ""
		}
		return agent + " file change: " + compactProgressValue(message, maxChars)
	case agentstream.EventTool:
		if message == "" {
			return ""
		}
		return agent + " tool: " + compactProgressValue(message, maxChars)
	case agentstream.EventMessage:
		if message == "" {
			return ""
		}
		return agent + ": " + compactProgressValue(message, maxChars)
	case agentstream.EventError, agentstream.EventInterrupted:
		if message == "" {
			message = "external agent reported an error"
		}
		return agent + " error: " + compactProgressValue(message, maxChars)
	case agentstream.EventCompleted:
		if message == "" {
			message = "external agent session completed"
		}
		return agent + " completed: " + compactProgressValue(message, maxChars)
	default:
		return "external agent stream rejected: unsupported event type"
	}
}
