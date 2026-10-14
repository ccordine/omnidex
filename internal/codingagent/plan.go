package codingagent

import (
	"context"
	"fmt"
	"sort"
)

// Plan validation is deliberately bounded, but complex workloads can expose a
// different mechanical defect after each correction. Keep enough room for a
// planner to repair several narrow validation notes without turning planning
// into an unbounded retry loop.
const maxPlanValidationAttempts = 5

// Propose returns a validated draft. It cannot freeze or execute the workload;
// only the queue's explicit user approval transition can do that.
func (controller *Controller) Propose(ctx context.Context) (Workload, error) {
	if err := controller.validatePlanning(); err != nil {
		return Workload{}, err
	}
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		return Workload{}, err
	}
	if questions := unansweredQuestions(entries); len(questions) > 0 {
		return Workload{}, &QuestionsPending{Questions: questions}
	}
	files, err := controller.Environment.ReadFiles(ctx)
	if err != nil {
		return Workload{}, err
	}
	sort.Strings(files)
	call := controller.assignment(Plan, nil, controller.Configuration.Planner)
	call.Requirements, call.Objective, call.ReadFiles = controller.Requirements, controller.Objective, files
	call.ArtifactBindings, call.PlanningEnvironment = controller.ArtifactBindings, controller.PlanningEnvironment
	call.ConstructionFacts = controller.ConstructionFacts
	for _, entry := range entries {
		if entry.Phase == "plan_rejected" {
			call.ValidationNotes = entry.Notes
		}
	}
	return controller.plan(ctx, call)
}

func (controller *Controller) plan(ctx context.Context, call Assignment) (Workload, error) {
	for attempt := 0; attempt < maxPlanValidationAttempts && ctx.Err() == nil; attempt++ {
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
			_, err = controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "plan_proposed", CallID: result.CallID, Workload: &workload})
			return workload, err
		}
		call.ValidationNotes = []string{err.Error()}
		if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "plan_rejected", CallID: result.CallID, Notes: call.ValidationNotes}); err != nil {
			return Workload{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Workload{}, err
	}
	return Workload{}, fmt.Errorf("native plan remained invalid after %d attempts", maxPlanValidationAttempts)
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
		for _, file := range task.DeleteFiles {
			writer, exists := catalog[file]
			if !exists {
				return fmt.Errorf("task %s requests deletion of an absent file %s", task.ID, file)
			}
			if writer != "" && !contains(task.Dependencies, writer) {
				return fmt.Errorf("task %s omitted dependency %s for deleted file %s", task.ID, writer, file)
			}
			delete(catalog, file)
		}
	}
	return nil
}
