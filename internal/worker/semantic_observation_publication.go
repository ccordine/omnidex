package worker

import "github.com/gryph/omnidex/internal/model"

func publishSemanticOpening(runtime typedWorkerRuntime, activity model.PlanningActivity) error {
	if err := activity.Validate(); err != nil {
		return err
	}
	runtime.ObservePlanning(activity)
	return nil
}
