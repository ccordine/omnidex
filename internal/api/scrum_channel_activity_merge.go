package api

import (
	"fmt"
	"strings"
)

func isNoisyStepEvent(eventType string) bool {
	switch eventType {
	case "tooling_begin", "tooling_complete", "tag_begin", "tag_complete",
		"retrieve_begin", "retrieve_embedding", "retrieve_embedding_error", "retrieve_complete",
		"plan_begin", "plan_candidate_error", "plan_complete",
		"web_search_begin", "web_search_complete",
		"analyze_begin", "analyze_complete", "response_begin", "response_complete",
		"verify_begin", "verify_complete", "verify_replan",
		"workspace_scan_begin", "workspace_scan_complete":
		return true
	default:
		return strings.HasPrefix(eventType, "v3_") && !strings.Contains(eventType, "patch") && !strings.Contains(eventType, "tool")
	}
}

func humanizeStepEventType(eventType string) string {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return "Agent event"
	}
	return strings.ReplaceAll(strings.ReplaceAll(eventType, "_", " "), "  ", " ")
}

func looksLikeDiff(text string) bool {
	text = strings.TrimSpace(text)
	return strings.Contains(text, "@@") || strings.Contains(text, "+++ ") || strings.Contains(text, "--- ")
}

func isLowSignalToolOutput(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return true
	}
	if len(text) < 4 {
		return true
	}
	lower := strings.ToLower(text)
	if lower == "ok" || lower == "done" || lower == "success" {
		return true
	}
	return false
}

func contextSyncMarker(contextID int64) string {
	return fmt.Sprintf("[[context-sync:%d]]", contextID)
}

func syncedStepContextID(chat []ScrumChatMessage) int64 {
	for i := len(chat) - 1; i >= 0; i-- {
		content := strings.TrimSpace(chat[i].Content)
		if !strings.HasPrefix(content, "[[context-sync:") {
			continue
		}
		var id int64
		if _, err := fmt.Sscanf(content, "[[context-sync:%d]]", &id); err == nil {
			return id
		}
	}
	return 0
}

func setStepContextSyncMarker(chat []ScrumChatMessage, contextID int64) []ScrumChatMessage {
	if contextID <= 0 {
		return chat
	}
	marker := contextSyncMarker(contextID)
	for i := len(chat) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(chat[i].Content), "[[context-sync:") {
			chat[i].Content = marker
			chat[i].Role = "system"
			return chat
		}
	}
	return appendScrumChatMessage(chat, "system", marker)
}

func sameChannelActivity(left, right ChannelActivity) bool {
	if left.Activity != right.Activity {
		return false
	}
	switch left.Activity {
	case "command":
		return left.Command == right.Command
	case "file_change":
		return left.Path == right.Path && strings.Join(left.Files, "|") == strings.Join(right.Files, "|")
	case "patch":
		return true
	case "tool_call":
		return normalizeToolLifecycleName(left.Tool) == normalizeToolLifecycleName(right.Tool) && left.Path == right.Path
	case "output", "event":
		return left.Title == right.Title
	default:
		return left.Title == right.Title && left.Command == right.Command && left.Path == right.Path
	}
}

func mergeChannelActivity(previous, next ChannelActivity) ChannelActivity {
	merged := next
	merged.Activity = firstNonEmpty(merged.Activity, previous.Activity)
	merged.Title = firstNonEmpty(merged.Title, previous.Title)
	merged.Status = firstNonEmpty(merged.Status, previous.Status)
	merged.Command = firstNonEmpty(merged.Command, previous.Command)
	merged.Tool = firstNonEmpty(merged.Tool, previous.Tool)
	merged.Path = firstNonEmpty(merged.Path, previous.Path)
	if len(merged.Files) == 0 {
		merged.Files = append([]string(nil), previous.Files...)
	}
	if merged.Activity == "output" {
		merged.Detail = mergeActivityText(previous.Detail, next.Detail, trimActivityDetail)
	} else {
		merged.Detail = firstNonEmpty(strings.TrimSpace(next.Detail), strings.TrimSpace(previous.Detail))
	}
	merged.Diff = mergeActivityText(previous.Diff, next.Diff, trimActivityDiff)
	return merged
}

func mergeActivityText(previous, next string, trim func(string) string) string {
	previous = strings.TrimSpace(previous)
	next = strings.TrimSpace(next)
	switch {
	case previous == "":
		return trim(next)
	case next == "":
		return trim(previous)
	case next == previous || strings.HasPrefix(next, previous):
		return trim(next)
	case strings.HasPrefix(previous, next):
		return trim(previous)
	default:
		return trim(previous + "\n" + next)
	}
}
