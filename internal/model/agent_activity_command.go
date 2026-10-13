package model

import (
	"encoding/json"
	"unicode/utf8"
)

// BoundAgentActivityCommand bounds presentation only. The frozen command and
// native receipt remain the execution/evidence authorities, unchanged.
func BoundAgentActivityCommand(activity AgentActivity) AgentActivity {
	activity.Argv = append([]string(nil), activity.Argv...)
	activity.CWD = boundedActivityPreview(activity.CWD, 4096, &activity.Truncated)
	activity.Command = boundedActivityPreview(activity.Command, 4096, &activity.Truncated)
	if len(activity.Argv) > 128 {
		activity.Argv = activity.Argv[:128]
		activity.Truncated = true
	}
	for index, argument := range activity.Argv {
		activity.Argv[index] = boundedActivityPreview(argument, 4096, &activity.Truncated)
	}
	for {
		encoded, _ := json.Marshal(activity)
		if len(encoded) <= 48*1024 {
			return activity
		}
		activity.Truncated = true
		switch {
		case len(activity.Argv) > 1:
			activity.Argv = activity.Argv[:len(activity.Argv)-1]
		case len(activity.Command) > 1:
			activity.Command = boundedActivityPreview(activity.Command, len(activity.Command)/2, &activity.Truncated)
		case len(activity.Argv) == 1 && len(activity.Argv[0]) > 1:
			activity.Argv[0] = boundedActivityPreview(activity.Argv[0], len(activity.Argv[0])/2, &activity.Truncated)
		case len(activity.CWD) > 1:
			activity.CWD = boundedActivityPreview(activity.CWD, len(activity.CWD)/2, &activity.Truncated)
		default:
			return activity
		}
	}
}

func boundedActivityPreview(value string, limit int, truncated *bool) string {
	if len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	*truncated = true
	return value[:limit]
}
