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
		Agent   string   `json:"agent"`
		Type    string   `json:"type"`
		Message string   `json:"message"`
		Command string   `json:"command"`
		Files   []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return []ScrumChatMessage{{Role: "assistant", Content: line}}
	}
	switch strings.ToLower(strings.TrimSpace(event.Type)) {
	case "started":
		msg := strings.TrimSpace(event.Message)
		if msg == "" {
			msg = "External agent session started"
		}
		return []ScrumChatMessage{{Role: "system", Content: msg}}
	case "status":
		return parseAgentStatusMessage(event.Message)
	case "error":
		msg := strings.TrimSpace(event.Message)
		if msg == "" {
			msg = "External agent reported an error"
		}
		return []ScrumChatMessage{{Role: "error", Content: msg}}
	case "completed":
		msg := formatAgentCompletionMessage(event.Message)
		if msg == "" {
			return nil
		}
		return []ScrumChatMessage{{Role: "assistant", Content: msg}}
	case "message":
		return parseAgentSDKPayload(event.Message)
	default:
		if msg := strings.TrimSpace(event.Message); msg != "" {
			return []ScrumChatMessage{{Role: "assistant", Content: msg}}
		}
	}
	if msg := strings.TrimSpace(event.Command); msg != "" {
		files := ""
		if len(event.Files) > 0 {
			files = " · " + strings.Join(event.Files, ", ")
		}
		return []ScrumChatMessage{{Role: "tool", Content: msg + files}}
	}
	return nil
}

func parseAgentStatusMessage(raw string) []ScrumChatMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return []ScrumChatMessage{{Role: "status", Content: raw}}
	}
	status := strings.ToUpper(strings.TrimSpace(fmt.Sprint(payload["status"])))
	switch status {
	case "RUNNING":
		return []ScrumChatMessage{{Role: "status", Content: "Agent running…"}}
	case "FINISHED", "COMPLETED":
		return []ScrumChatMessage{{Role: "status", Content: "Agent finished"}}
	case "ERROR", "FAILED", "CANCELLED":
		detail := strings.TrimSpace(fmt.Sprint(payload["message"]))
		if detail == "" {
			detail = "Agent run ended with status " + strings.ToLower(status)
		}
		return []ScrumChatMessage{{Role: "error", Content: detail}}
	default:
		if status != "" {
			return []ScrumChatMessage{{Role: "status", Content: "Status: " + strings.ToLower(status)}}
		}
	}
	return nil
}

func parseAgentSDKPayload(raw string) []ScrumChatMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return []ScrumChatMessage{{Role: "assistant", Content: raw}}
	}
	kind := strings.ToLower(strings.TrimSpace(fmt.Sprint(payload["type"])))
	switch kind {
	case "assistant":
		if text := extractAssistantText(payload); text != "" {
			return []ScrumChatMessage{{Role: "assistant", Content: text}}
		}
	case "thinking":
		if text := strings.TrimSpace(fmt.Sprint(payload["text"])); text != "" {
			return []ScrumChatMessage{{Role: "thinking", Content: text}}
		}
	case "tool_call":
		name := strings.TrimSpace(fmt.Sprint(payload["name"]))
		if name == "" {
			name = "tool"
		}
		status := strings.TrimSpace(fmt.Sprint(payload["status"]))
		line := name
		if status != "" {
			line += " (" + status + ")"
		}
		if args, ok := payload["args"]; ok && args != nil {
			if blob, err := json.Marshal(args); err == nil && len(blob) > 2 && len(blob) < 400 {
				line += " " + string(blob)
			}
		}
		return []ScrumChatMessage{{Role: "tool", Content: line}}
	case "status":
		return parseAgentStatusMessage(raw)
	}
	if text := strings.TrimSpace(fmt.Sprint(payload["text"])); text != "" && text != "<nil>" {
		return []ScrumChatMessage{{Role: "assistant", Content: text}}
	}
	return []ScrumChatMessage{{Role: "assistant", Content: raw}}
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
			chat = appendScrumChatMessage(chat, msg.Role, msg.Content)
		}
	}
	return chat
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
