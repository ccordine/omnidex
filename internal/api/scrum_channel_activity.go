package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

type ChannelActivity struct {
	Activity string   `json:"activity"`
	Title    string   `json:"title,omitempty"`
	Status   string   `json:"status,omitempty"`
	Command  string   `json:"command,omitempty"`
	Tool     string   `json:"tool,omitempty"`
	Path     string   `json:"path,omitempty"`
	Files    []string `json:"files,omitempty"`
	Detail   string   `json:"detail,omitempty"`
	Diff     string   `json:"diff,omitempty"`
}

func formatChannelActivity(activity ChannelActivity) string {
	activity.Activity = strings.TrimSpace(activity.Activity)
	if activity.Activity == "" {
		activity.Activity = "event"
	}
	raw, err := json.Marshal(activity)
	if err != nil {
		return activity.Title
	}
	return string(raw)
}

func parseChannelActivity(content string) (ChannelActivity, bool) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "{") {
		return ChannelActivity{}, false
	}
	var activity ChannelActivity
	if err := json.Unmarshal([]byte(content), &activity); err != nil {
		return ChannelActivity{}, false
	}
	if strings.TrimSpace(activity.Activity) == "" {
		return ChannelActivity{}, false
	}
	return activity, true
}

func activityMessage(role string, activity ChannelActivity) ScrumChatMessage {
	if role == "" {
		role = "tool"
	}
	return ScrumChatMessage{
		Role:    role,
		Content: formatChannelActivity(activity),
	}
}

func commandActivity(command, status, detail string) ScrumChatMessage {
	command = strings.TrimSpace(command)
	status = normalizeActivityStatus(status)
	title := "Run command"
	if command != "" {
		title = command
	}
	return activityMessage("tool", ChannelActivity{
		Activity: "command",
		Title:    title,
		Status:   status,
		Command:  command,
		Detail:   strings.TrimSpace(detail),
	})
}

func fileChangeActivity(files []string, status, detail, diff string) ScrumChatMessage {
	clean := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file != "" {
			clean = append(clean, file)
		}
	}
	title := "File change"
	if len(clean) == 1 {
		title = clean[0]
	} else if len(clean) > 1 {
		title = fmt.Sprintf("%d files changed", len(clean))
	}
	path := ""
	if len(clean) == 1 {
		path = clean[0]
	}
	return activityMessage("tool", ChannelActivity{
		Activity: "file_change",
		Title:    title,
		Status:   normalizeActivityStatus(status),
		Path:     path,
		Files:    clean,
		Detail:   strings.TrimSpace(detail),
		Diff:     trimActivityDiff(diff),
	})
}

func toolCallActivity(name, path, status, detail string) ScrumChatMessage {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "tool"
	}
	title := name
	if path := strings.TrimSpace(path); path != "" {
		title = name + " · " + path
	}
	return activityMessage("tool", ChannelActivity{
		Activity: "tool_call",
		Title:    title,
		Tool:     name,
		Path:     strings.TrimSpace(path),
		Status:   normalizeActivityStatus(status),
		Detail:   strings.TrimSpace(detail),
	})
}

func patchActivity(status string, files []string, detail string) ScrumChatMessage {
	title := "Apply patch"
	if len(files) == 1 {
		title = "Patched " + files[0]
	} else if len(files) > 1 {
		title = fmt.Sprintf("Patched %d files", len(files))
	}
	return activityMessage("tool", ChannelActivity{
		Activity: "patch",
		Title:    title,
		Status:   normalizeActivityStatus(status),
		Files:    files,
		Detail:   strings.TrimSpace(detail),
	})
}

func outputActivity(title, detail string) ScrumChatMessage {
	return activityMessage("tool", ChannelActivity{
		Activity: "output",
		Title:    firstNonEmpty(strings.TrimSpace(title), "Command output"),
		Detail:   trimActivityDetail(detail),
	})
}

func normalizeActivityStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "running", "started", "in_progress", "pending":
		return "running"
	case "completed", "complete", "done", "success", "finished":
		return "completed"
	case "failed", "error", "rejected":
		return "failed"
	default:
		if status == "" {
			return "completed"
		}
		return status
	}
}

func trimActivityDiff(diff string) string {
	diff = strings.TrimSpace(diff)
	const max = 6000
	if len(diff) <= max {
		return diff
	}
	return diff[:max-3] + "..."
}

func trimActivityDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	const max = 2400
	if len(detail) <= max {
		return detail
	}
	return detail[:max-3] + "..."
}

func extractFileDiffsFromRaw(raw json.RawMessage) (files []string, diff string) {
	if len(raw) == 0 {
		return nil, ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, ""
	}
	changes, _ := payload["changes"].([]any)
	if len(changes) == 0 {
		if path := stringFromAnyMap(payload, "path"); path != "" {
			return []string{path}, stringFromAnyMap(payload, "diff")
		}
		return nil, ""
	}
	lines := make([]string, 0, len(changes))
	for _, item := range changes {
		change, _ := item.(map[string]any)
		if change == nil {
			continue
		}
		path := stringFromAnyMap(change, "path")
		if path != "" {
			files = append(files, path)
		}
		if chunk := stringFromAnyMap(change, "diff"); chunk != "" {
			if path != "" {
				lines = append(lines, "--- "+path+" ---", chunk)
			} else {
				lines = append(lines, chunk)
			}
		}
	}
	return files, strings.Join(lines, "\n")
}

func stringFromAnyMap(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	raw, ok := payload[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func sdkToolCallToActivity(payload map[string]any) []ScrumChatMessage {
	if payload == nil {
		return nil
	}
	name := firstNonEmpty(
		stringFromAnyMap(payload, "name"),
		stringFromAnyMap(payload, "tool"),
		stringFromAnyMap(payload, "tool_name"),
	)
	status := firstNonEmpty(
		stringFromAnyMap(payload, "status"),
		stringFromAnyMap(payload, "state"),
	)
	path := firstNonEmpty(
		stringFromAnyMap(payload, "path"),
		stringFromAnyMap(payload, "file"),
		stringFromAnyMap(payload, "file_path"),
	)
	if path == "" {
		if args, ok := payload["args"].(map[string]any); ok {
			path = firstNonEmpty(
				stringFromAnyMap(args, "path"),
				stringFromAnyMap(args, "file"),
				stringFromAnyMap(args, "target"),
			)
		}
	}
	if path == "" {
		if input, ok := payload["input"].(map[string]any); ok {
			path = firstNonEmpty(
				stringFromAnyMap(input, "path"),
				stringFromAnyMap(input, "file"),
			)
		}
	}
	detail := firstNonEmpty(
		stringFromAnyMap(payload, "summary"),
		stringFromAnyMap(payload, "result"),
	)
	if detail == "" {
		if result, ok := payload["result"].(map[string]any); ok {
			detail = firstNonEmpty(stringFromAnyMap(result, "summary"), stringFromAnyMap(result, "stdout"))
		}
	}
	if name == "" {
		return nil
	}
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "edit"), strings.Contains(lower, "write"), strings.Contains(lower, "patch"), strings.Contains(lower, "replace"):
		files := []string{}
		if path != "" {
			files = append(files, path)
		}
		return []ScrumChatMessage{fileChangeActivity(files, status, detail, "")}
	case strings.Contains(lower, "shell"), strings.Contains(lower, "bash"), strings.Contains(lower, "command"), strings.Contains(lower, "run"):
		cmd := firstNonEmpty(path, detail, name)
		return []ScrumChatMessage{commandActivity(cmd, status, detail)}
	default:
		return []ScrumChatMessage{toolCallActivity(name, path, status, detail)}
	}
}

func agentEventToActivity(event struct {
	Type    string
	Message string
	Command string
	Files   []string
	Raw     json.RawMessage
}) []ScrumChatMessage {
	switch strings.ToLower(strings.TrimSpace(event.Type)) {
	case "command":
		cmd := strings.TrimSpace(event.Command)
		if cmd == "" {
			cmd = strings.TrimSpace(event.Message)
		}
		if cmd == "" {
			return nil
		}
		status := strings.TrimSpace(event.Message)
		if strings.EqualFold(status, cmd) {
			status = "running"
		}
		return []ScrumChatMessage{commandActivity(cmd, status, event.Message)}
	case "file_change":
		files := append([]string(nil), event.Files...)
		extraFiles, diff := extractFileDiffsFromRaw(event.Raw)
		if len(files) == 0 {
			files = extraFiles
		}
		if len(files) == 0 && diff == "" {
			if msg := strings.TrimSpace(event.Message); msg != "" {
				return []ScrumChatMessage{fileChangeActivity(nil, "completed", msg, "")}
			}
			return nil
		}
		return []ScrumChatMessage{fileChangeActivity(files, event.Message, "", diff)}
	default:
		return nil
	}
}

func parseStepEventContext(value string) (eventType, summary string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	parts := strings.Fields(value)
	for i, part := range parts {
		if strings.HasPrefix(part, "event=") {
			eventType = strings.TrimPrefix(part, "event=")
			if i+1 < len(parts) {
				summary = strings.Join(parts[i+1:], " ")
			}
			return eventType, summary
		}
	}
	return "", value
}

func stepContextToActivity(ctx model.StepContext) []ScrumChatMessage {
	key := strings.TrimSpace(ctx.Key)
	value := strings.TrimSpace(ctx.Value)
	if value == "" {
		return nil
	}
	switch key {
	case "event":
		eventType, summary := parseStepEventContext(value)
		eventType = strings.ToLower(strings.TrimSpace(eventType))
		switch {
		case strings.Contains(eventType, "patch_apply"):
			status := "completed"
			if strings.Contains(eventType, "failed") {
				status = "failed"
			} else if strings.Contains(eventType, "started") {
				status = "running"
			}
			return []ScrumChatMessage{patchActivity(status, nil, summary)}
		case strings.Contains(eventType, "tool_call"):
			status := "completed"
			if strings.Contains(eventType, "rejected") || strings.Contains(eventType, "failed") {
				status = "failed"
			} else if strings.Contains(eventType, "begin") || strings.Contains(eventType, "started") {
				status = "running"
			}
			return []ScrumChatMessage{toolCallActivity(eventType, "", status, summary)}
		case strings.Contains(eventType, "external_agent"):
			if strings.Contains(eventType, "command") {
				return []ScrumChatMessage{commandActivity(summary, "running", "")}
			}
			if strings.Contains(eventType, "file_change") {
				files := strings.Split(summary, ",")
				return []ScrumChatMessage{fileChangeActivity(files, "completed", "", "")}
			}
			return nil
		case isNoisyStepEvent(eventType):
			return nil
		default:
			if summary == "" {
				return nil
			}
			return []ScrumChatMessage{activityMessage("tool", ChannelActivity{
				Activity: "event",
				Title:    humanizeStepEventType(eventType),
				Status:   "completed",
				Detail:   summary,
			})}
		}
	case "tool_stdout":
		if isLowSignalToolOutput(value) {
			return nil
		}
		title := "stdout"
		if looksLikeDiff(value) {
			return []ScrumChatMessage{fileChangeActivity(nil, "completed", "Command produced diff output", value)}
		}
		return []ScrumChatMessage{outputActivity(title, value)}
	case "tool_stderr":
		if isLowSignalToolOutput(value) {
			return nil
		}
		return []ScrumChatMessage{outputActivity("stderr", value)}
	default:
		return nil
	}
}

func isNoisyStepEvent(eventType string) bool {
	switch eventType {
	case "tooling_begin", "tooling_complete", "tag_begin", "tag_complete",
		"retrieve_begin", "retrieve_embedding", "retrieve_embedding_error", "retrieve_complete",
		"plan_begin", "plan_candidate_error", "plan_complete",
		"web_search_begin", "web_search_degraded", "web_search_complete",
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
	return left.Activity == right.Activity &&
		left.Title == right.Title &&
		left.Command == right.Command &&
		left.Path == right.Path &&
		left.Status == right.Status &&
		strings.Join(left.Files, "|") == strings.Join(right.Files, "|")
}
