package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentstream"
)

func agentNDJSONLineToChatMessages(line string) ([]ScrumChatMessage, error) {
	event, err := agentstream.DecodeLine(line)
	if err != nil {
		return nil, err
	}
	role, err := scrumRoleForAgentEvent(event.Type)
	if err != nil {
		return nil, err
	}
	return []ScrumChatMessage{{Role: role, Content: event.Message}}, nil
}

func scrumRoleForAgentEvent(eventType agentstream.EventType) (string, error) {
	switch eventType {
	case agentstream.EventStarted, agentstream.EventStatus:
		return "system", nil
	case agentstream.EventMessage, agentstream.EventCompleted:
		return "assistant", nil
	case agentstream.EventThinking:
		return "thinking", nil
	case agentstream.EventCommand, agentstream.EventTool, agentstream.EventFileChange:
		return "tool", nil
	case agentstream.EventError, agentstream.EventInterrupted:
		return "error", nil
	default:
		return "", fmt.Errorf("external agent event has unsupported type %q", eventType)
	}
}

func appendParsedAgentStreamLines(chat []ScrumChatMessage, delta string) ([]ScrumChatMessage, error) {
	if delta == "" {
		return chat, nil
	}
	lines := strings.Split(delta, "\n")
	pending := make([]ScrumChatMessage, 0, len(lines))
	for index, line := range lines {
		if line == "" && index == len(lines)-1 {
			continue
		}
		if line == "" {
			return chat, fmt.Errorf("external agent stream line %d is empty", index+1)
		}
		messages, err := agentNDJSONLineToChatMessages(line)
		if err != nil {
			return chat, fmt.Errorf("parse external agent stream line %d: %w", index+1, err)
		}
		pending = append(pending, messages...)
	}
	updated := append([]ScrumChatMessage(nil), chat...)
	for _, message := range pending {
		message.ID = newScrumChatMessageID(message.Role, message.Content)
		message.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		updated = append(updated, message)
	}
	return updated, nil
}
