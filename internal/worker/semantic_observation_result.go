package worker

import (
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

// The journal's validated result is unchanged. Publish every byte in bounded
// observation frames, rather than silently clipping a free-form result.
func publishSemanticResult(runtime typedWorkerRuntime, activity model.PlanningActivity) {
	result := activity.Result
	if len(activity.Options) > 0 && (activity.Validate() == model.ErrPlanningActivityTooLarge || len(result) > model.MaxAgentActivityTextBytes) {
		activity.OptionTotal, activity.Options = len(activity.Options), nil
	}
	for offset := 0; offset < len(result); {
		end := min(offset+model.MaxAgentActivityTextBytes, len(result))
		for !utf8.ValidString(result[offset:end]) {
			end--
		}
		activity.Result = result[offset:end]
		if len(result) > model.MaxAgentActivityTextBytes {
			activity.ResultOffset, activity.ResultTotal = offset, len(result)
		}
		runtime.ObservePlanning(activity)
		offset = end
	}
}
