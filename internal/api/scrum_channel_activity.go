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
		panic(fmt.Sprintf("encode channel activity: %v", err))
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
