package api

import (
	"strings"
)

func isNoisyStepEvent(eventType string) bool {
	switch eventType {
	case "operation_heartbeat",
		"coding_portable_dispatched", "objective_portable_dispatched",
		"coding_worker_started", "objective_worker_started",
		"coding_worker_completed", "objective_worker_completed":
		return true
	default:
		return false
	}
}

func humanizeStepEventType(eventType string) string {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return "Runtime event"
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

func sameChannelActivity(left, right ChannelActivity) bool {
	if left.Activity != right.Activity {
		return false
	}
	switch left.Activity {
	case "file_change":
		return left.Path == right.Path && strings.Join(left.Files, "|") == strings.Join(right.Files, "|")
	case "output", "event":
		return left.Title == right.Title
	default:
		return left.Title == right.Title && left.Path == right.Path
	}
}

func mergeChannelActivity(previous, next ChannelActivity) ChannelActivity {
	merged := next
	merged.Activity = firstNonEmpty(merged.Activity, previous.Activity)
	merged.Title = firstNonEmpty(merged.Title, previous.Title)
	merged.Status = firstNonEmpty(merged.Status, previous.Status)
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
