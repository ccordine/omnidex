package codingagent

import (
	"context"
	"fmt"
)

func (controller *Controller) plan(ctx context.Context, call Assignment) (Workload, error) {
	for ctx.Err() == nil {
		result, err := controller.invoke(ctx, call, 1)
		if err != nil {
			return Workload{}, err
		}
		var workload Workload
		err = decodeContract(result.Content, &workload)
		if err == nil && len(workload.Questions) > 0 {
			if err = validateTextList(workload.Questions, "questions"); err == nil {
				return Workload{}, controller.questions(ctx, "", workload.Questions)
			}
		}
		if err == nil {
			err = workload.Validate()
		}
		if err == nil {
			err = bindWorkloadRequirements(&workload, controller.Requirements)
		}
		if err == nil {
			err = validatePlanFiles(workload, call.ReadFiles)
		}
		if err == nil {
			_, err = controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "plan_frozen", CallID: result.CallID, Workload: &workload})
			return workload, err
		}
		call.ValidationNotes = []string{err.Error()}
		if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "plan_rejected", CallID: result.CallID, Notes: call.ValidationNotes}); err != nil {
			return Workload{}, err
		}
	}
	return Workload{}, ctx.Err()
}

func validatePlanFiles(workload Workload, files []string) error {
	catalog := map[string]string{}
	for _, file := range files {
		catalog[file] = ""
	}
	for _, task := range workload.Tasks {
		for _, file := range task.ReadFiles {
			writer, exists := catalog[file]
			if !exists && !contains(task.WriteFiles, file) {
				return fmt.Errorf("task %s requests an ungranted dependency %s", task.ID, file)
			}
			if writer != "" && !contains(task.Dependencies, writer) {
				return fmt.Errorf("task %s omitted dependency %s for %s", task.ID, writer, file)
			}
		}
		for _, file := range task.WriteFiles {
			catalog[file] = task.ID
		}
	}
	return nil
}
