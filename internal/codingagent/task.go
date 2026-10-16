package codingagent

import (
	"context"
	"fmt"
)

type taskState struct {
	accepted map[string]bool
	findings map[string][]Finding
	tasks    map[string]Task
	started  map[string]bool
	round    int64
}

func projectTaskState(task Task, entries []Entry) *taskState {
	state := &taskState{accepted: map[string]bool{}, findings: map[string][]Finding{}, tasks: map[string]Task{}, started: map[string]bool{}}
	state.tasks[task.ID] = task
	for _, entry := range entries {
		switch entry.Phase {
		case "task_accepted":
			state.accepted[entry.TaskID] = true
			delete(state.findings, entry.TaskID)
		case "task_reviewed":
			state.accepted[entry.TaskID] = false
			state.findings[entry.TaskID] = append([]Finding{}, entry.Findings...)
		}
		if entry.TaskID != task.ID {
			continue
		}
		if entry.Round > state.round {
			state.round = entry.Round
		}
		switch entry.Phase {
		case "task_executing", "task_reviewed":
			// These entries record work that still requires a successful fresh
			// executor result. A provider session ending after task_executing must
			// not make recovery skip execution and advance directly to review.
			state.started[entry.TaskID] = false
		case "call_completed":
			if entry.Role == Execute {
				state.started[entry.TaskID] = true
			}
		}
	}
	return state
}

func (controller *Controller) runTask(ctx context.Context, task Task, state *taskState) error {
	for _, dependency := range task.Dependencies {
		if !state.accepted[dependency] {
			return fmt.Errorf("task %s dependency %s is not accepted", task.ID, dependency)
		}
	}
	if err := controller.Environment.PrepareFiles(ctx, task); err != nil {
		return err
	}
	if !state.started[task.ID] {
		state.started[task.ID] = true
		if err := controller.executeTask(ctx, task, state, nil); err != nil {
			return err
		}
	}
	for state.round < 3 && ctx.Err() == nil {
		if err := controller.Environment.Attest(ctx); err != nil {
			return err
		}
		scope := controller.assignment(Review, &task, controller.Configuration.Reviewer)
		scope.Findings = state.findings[task.ID]
		checks := []CheckResult{}
		if len(task.Checks) > 0 {
			var err error
			checks, err = controller.executeChecks(ctx, scope, task.Checks)
			if err != nil {
				return err
			}
		}
		if err := validateCheckResults(task.Checks, checks); err != nil {
			return err
		}
		scope.Checks = checks
		review, reviewResult, err := controller.review(ctx, scope, map[string]Task{task.ID: task}, state.round+1)
		if err != nil {
			return err
		}
		if len(review.Questions) > 0 {
			return fmt.Errorf("frozen task review cannot request new owner decisions")
		}
		if (len(checks) == 0 || checksPass(checks)) && len(review.Findings) == 0 {
			state.accepted[task.ID] = true
			_, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_accepted", TaskID: task.ID, CallID: reviewResult.CallID, Round: state.round + 1, Checks: checks})
			return err
		}
		state.findings[task.ID] = review.Findings
		if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_reviewed", TaskID: task.ID, Findings: review.Findings, Checks: checks}); err != nil {
			return err
		}
		if err := controller.executeTask(ctx, task, state, checks); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("task %s was not fulfilled after three bounded execution-review rounds", task.ID)
}

func (controller *Controller) executeTask(ctx context.Context, task Task, state *taskState, checks []CheckResult) error {
	if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_executing", TaskID: task.ID, Round: state.round + 1}); err != nil {
		return err
	}
	execution := controller.assignment(Execute, &task, controller.Configuration.Executor)
	execution.Findings, execution.Checks = state.findings[task.ID], checks
	result, err := controller.invoke(ctx, execution, state.round+1)
	if err != nil {
		return err
	}
	var response ExecutionResult
	if err := decodeContract(result.Content, &response); err != nil {
		return err
	}
	if response.Questions == nil {
		return fmt.Errorf("execution agent omitted its questions array")
	}
	if err := validateTextList(response.Questions, "questions"); err != nil {
		return err
	}
	if len(response.Questions) > 0 {
		return fmt.Errorf("frozen task execution cannot request new owner decisions")
	}
	state.round++
	return nil
}
