package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

func agentNDJSONLineToChatMessages(line string) []ScrumChatMessage {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	var event struct {
		Agent   string          `json:"agent"`
		Type    string          `json:"type"`
		Message string          `json:"message"`
		Command string          `json:"command"`
		Files   []string        `json:"files"`
		Raw     json.RawMessage `json:"raw,omitempty"`
	}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		if strings.HasPrefix(strings.TrimSpace(line), "{") || strings.HasPrefix(strings.TrimSpace(line), "[") {
			return nil
		}
		return []ScrumChatMessage{{Role: "assistant", Content: line}}
	}
	switch strings.ToLower(strings.TrimSpace(event.Type)) {
	case "started":
		return nil
	case "status":
		return nil
	case "error":
		msg := strings.TrimSpace(event.Message)
		if msg == "" {
			msg = "External agent reported an error"
		}
		return []ScrumChatMessage{{Role: "error", Content: msg}}
	case "command", "file_change":
		return agentEventToActivity(struct {
			Type    string
			Message string
			Command string
			Files   []string
			Raw     json.RawMessage
		}{
			Type:    event.Type,
			Message: event.Message,
			Command: event.Command,
			Files:   event.Files,
			Raw:     event.Raw,
		})
	case "thinking", "reasoning":
		msg := strings.TrimSpace(event.Message)
		if msg == "" {
			msg = textFromRawAgentPayload(event.Raw)
		}
		if msg == "" {
			return parseAgentSDKPayloadBytes(event.Raw)
		}
		if msg == "" {
			return nil
		}
		return []ScrumChatMessage{{Role: "thinking", Content: msg}}
	case "tool", "mcp_tool_call", "web_search":
		if len(event.Raw) > 0 {
			if msgs := parseAgentSDKPayloadBytes(event.Raw); len(msgs) > 0 {
				return msgs
			}
		}
		msg := strings.TrimSpace(event.Message)
		if msg == "" {
			return nil
		}
		return []ScrumChatMessage{toolCallActivity(msg, "", "completed", "")}
	case "completed":
		msg := formatAgentCompletionMessage(event.Message)
		if msg == "" || isScrumChannelNoiseContent("assistant", msg) {
			return nil
		}
		return []ScrumChatMessage{{Role: "assistant", Content: msg}}
	case "message":
		if msgs := parseAgentSDKPayload(event.Message); len(msgs) > 0 {
			return msgs
		}
		return parseAgentSDKPayloadBytes(event.Raw)
	default:
		if len(event.Raw) > 0 {
			if msgs := parseAgentSDKPayloadBytes(event.Raw); len(msgs) > 0 {
				return msgs
			}
		}
		if msg := strings.TrimSpace(event.Message); msg != "" {
			return []ScrumChatMessage{{Role: "assistant", Content: msg}}
		}
	}
	if msg := strings.TrimSpace(event.Command); msg != "" {
		return agentEventToActivity(struct {
			Type    string
			Message string
			Command string
			Files   []string
			Raw     json.RawMessage
		}{
			Type:    "command",
			Message: event.Message,
			Command: event.Command,
			Files:   event.Files,
			Raw:     event.Raw,
		})
	}
	return nil
}

func parseAgentSDKPayloadBytes(raw json.RawMessage) []ScrumChatMessage {
	if len(raw) == 0 {
		return nil
	}
	return parseAgentSDKPayload(string(raw))
}

func parseAgentSDKPayload(raw string) []ScrumChatMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
			return nil
		}
		return []ScrumChatMessage{{Role: "assistant", Content: raw}}
	}
	kind := strings.ToLower(strings.TrimSpace(fmt.Sprint(payload["type"])))
	switch kind {
	case "assistant", "agent_message":
		if text := extractAssistantText(payload); text != "" {
			return []ScrumChatMessage{{Role: "assistant", Content: text}}
		}
		if text := textFromAgentPayload(payload); text != "" {
			return []ScrumChatMessage{{Role: "assistant", Content: text}}
		}
	case "thinking", "reasoning":
		if text := textFromAgentPayload(payload); text != "" {
			return []ScrumChatMessage{{Role: "thinking", Content: text}}
		}
	case "tool_call", "tool", "function_call", "mcp_tool_call":
		if msgs := sdkToolCallToActivity(payload); len(msgs) > 0 {
			return msgs
		}
		return nil
	case "web_search":
		query := firstNonEmpty(stringFromAnyMap(payload, "query"), stringFromAnyMap(payload, "text"))
		if query == "" {
			return nil
		}
		return []ScrumChatMessage{toolCallActivity("web_search", "", "completed", query)}
	case "todo_list":
		if text := todoListTextFromAgentPayload(payload); text != "" {
			return []ScrumChatMessage{{Role: "thinking", Content: text}}
		}
		return nil
	case "command_execution":
		cmd := firstNonEmpty(stringFromAnyMap(payload, "command"), stringFromAnyMap(payload, "cmd"))
		status := firstNonEmpty(stringFromAnyMap(payload, "status"), stringFromAnyMap(payload, "state"))
		detail := firstNonEmpty(stringFromAnyMap(payload, "aggregated_output"), stringFromAnyMap(payload, "output"))
		if cmd == "" && detail == "" {
			return nil
		}
		return []ScrumChatMessage{commandActivity(cmd, status, detail)}
	case "command_output":
		if text := textFromAgentPayload(payload); text != "" {
			return []ScrumChatMessage{outputActivity("Command output", text)}
		}
		return nil
	case "edit", "write", "apply_patch", "replace":
		path := firstNonEmpty(
			stringFromAnyMap(payload, "path"),
			stringFromAnyMap(payload, "file"),
		)
		status := firstNonEmpty(stringFromAnyMap(payload, "status"), stringFromAnyMap(payload, "state"))
		files := []string{}
		if path != "" {
			files = append(files, path)
		}
		return []ScrumChatMessage{fileChangeActivity(files, status, "", "")}
	case "shell", "command", "terminal":
		cmd := firstNonEmpty(
			stringFromAnyMap(payload, "command"),
			stringFromAnyMap(payload, "cmd"),
			stringFromAnyMap(payload, "text"),
		)
		status := firstNonEmpty(stringFromAnyMap(payload, "status"), stringFromAnyMap(payload, "state"))
		return []ScrumChatMessage{commandActivity(cmd, status, "")}
	case "status":
		return nil
	}
	if text := textFromAgentPayload(payload); text != "" {
		return []ScrumChatMessage{{Role: "assistant", Content: text}}
	}
	return []ScrumChatMessage{{Role: "assistant", Content: raw}}
}

func textFromRawAgentPayload(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return textFromAgentPayload(payload)
}

func textFromAgentPayload(payload map[string]any) string {
	for _, key := range []string{"text", "message", "summary", "content", "aggregated_output", "output"} {
		text := stringFromAnyMap(payload, key)
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func todoListTextFromAgentPayload(payload map[string]any) string {
	items, _ := payload["items"].([]any)
	if len(items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		text := stringFromAnyMap(item, "text")
		if text == "" {
			continue
		}
		prefix := "[ ]"
		if done, _ := item["completed"].(bool); done {
			prefix = "[x]"
		}
		lines = append(lines, prefix+" "+text)
	}
	return strings.Join(lines, "\n")
}

func extractAssistantText(payload map[string]any) string {
	message, _ := payload["message"].(map[string]any)
	content, _ := message["content"].([]any)
	parts := make([]string, 0, len(content))
	for _, item := range content {
		block, _ := item.(map[string]any)
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(block["type"])), "text") {
			if text := strings.TrimSpace(fmt.Sprint(block["text"])); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "")
}

func formatAgentCompletionMessage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "External agent session completed"
	}
	if strings.HasPrefix(raw, "{") {
		if text := extractAssistantText(parseJSONMap(raw)); text != "" {
			return text
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			if status := strings.ToUpper(strings.TrimSpace(fmt.Sprint(payload["status"]))); status == "FINISHED" || status == "COMPLETED" {
				return "External agent session completed"
			}
		}
	}
	return raw
}

func parseJSONMap(raw string) map[string]any {
	var payload map[string]any
	_ = json.Unmarshal([]byte(raw), &payload)
	return payload
}

func appendParsedAgentStreamLines(chat []ScrumChatMessage, delta string) []ScrumChatMessage {
	if strings.TrimSpace(delta) == "" {
		return chat
	}
	for _, line := range strings.Split(delta, "\n") {
		for _, msg := range agentNDJSONLineToChatMessages(line) {
			if shouldSkipDuplicateChannelMessage(chat, msg) {
				continue
			}
			chat = appendOrMergeChannelMessage(chat, msg)
		}
	}
	return chat
}

func appendOrMergeChannelMessage(chat []ScrumChatMessage, msg ScrumChatMessage) []ScrumChatMessage {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return chat
	}
	role := normalizeScrumChannelRole(msg.Role)
	if len(chat) == 0 {
		return appendScrumChatMessage(chat, role, content)
	}
	lastIdx := len(chat) - 1
	last := chat[lastIdx]
	lastRole := normalizeScrumChannelRole(last.Role)
	if role != lastRole {
		return appendScrumChatMessage(chat, role, content)
	}
	switch role {
	case "assistant":
		merged := mergeAssistantStreamContent(stripAssistantStreamMarker(last.Content), content)
		if merged == strings.TrimSpace(stripAssistantStreamMarker(last.Content)) {
			return chat
		}
		last.Content = merged
	case "thinking":
		merged := mergePilotThoughtText(stripAssistantStreamMarker(last.Content), content)
		if merged == strings.TrimSpace(stripAssistantStreamMarker(last.Content)) {
			return chat
		}
		last.Content = merged
	default:
		if strings.TrimSpace(last.Content) == content {
			return chat
		}
		return appendScrumChatMessage(chat, role, content)
	}
	if ts := strings.TrimSpace(msg.CreatedAt); ts != "" {
		last.CreatedAt = ts
	}
	chat[lastIdx] = last
	return chat
}

func mergeAssistantStreamContent(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if existing == "" {
		return next
	}
	if next == "" || existing == next {
		return existing
	}
	if strings.HasPrefix(next, existing) {
		return next
	}
	if strings.HasPrefix(existing, next) {
		return existing
	}
	if strings.Contains(existing, next) {
		return existing
	}
	return strings.TrimRight(existing, "\n") + "\n" + next
}

func shouldSkipDuplicateChannelMessage(chat []ScrumChatMessage, msg ScrumChatMessage) bool {
	if len(chat) == 0 {
		return false
	}
	last := chat[len(chat)-1]
	return last.Role == msg.Role && strings.TrimSpace(last.Content) == strings.TrimSpace(msg.Content)
}

func channelSyncMarkerIndex(chat []ScrumChatMessage) int {
	for i := len(chat) - 1; i >= 0; i-- {
		if strings.Contains(chat[i].Content, "[[agent-stream-len:") {
			return i
		}
	}
	return -1
}

func syncedAgentStreamLenFromChat(chat []ScrumChatMessage) int {
	idx := channelSyncMarkerIndex(chat)
	if idx < 0 {
		return 0
	}
	return syncedAgentStreamLen(chat[idx].Content)
}

func setChannelSyncMarker(chat []ScrumChatMessage, syncedLen int) []ScrumChatMessage {
	if syncedLen <= 0 {
		return chat
	}
	marker := agentStreamMarker(syncedLen)
	idx := channelSyncMarkerIndex(chat)
	if idx >= 0 {
		chat[idx].Content = marker
		chat[idx].Role = "system"
		return chat
	}
	return appendScrumChatMessage(chat, "system", marker)
}
