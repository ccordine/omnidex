package worker

import (
	"encoding/json"
	"errors"

	"github.com/gryph/omnidex/internal/model"
)

// Page only the observation to fit the existing wire boundary. All offered
// choices are shown; the original request, response and inference stay intact.
func publishSemanticOpening(runtime typedWorkerRuntime, activity model.PlanningActivity) error {
	validationErr := activity.Validate()
	if validationErr == nil {
		runtime.ObservePlanning(activity)
		return nil
	}
	if !errors.Is(validationErr, model.ErrPlanningActivityTooLarge) {
		return validationErr
	}
	for offset := 0; offset < len(activity.Options); {
		page := activity
		page.OptionOffset, page.OptionTotal = offset, len(activity.Options)
		page.Options = nil
		for end := offset; end < len(activity.Options); end++ {
			page.Options = append(page.Options, activity.Options[end])
			encoded, err := json.Marshal(page)
			if err != nil {
				return err
			}
			if len(encoded) > model.MaxPlanningActivityBytes {
				page.Options = page.Options[:len(page.Options)-1]
				break
			}
		}
		if err := page.Validate(); err != nil {
			return err
		}
		runtime.ObservePlanning(page)
		offset += len(page.Options)
	}
	return nil
}
