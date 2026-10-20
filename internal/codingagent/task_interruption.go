package codingagent

import (
	"context"
	"errors"
)

// Persist the handoff before asking the planner anything. Parser/minification
// outages or process restarts must not cause the native task to execute again.
func (controller *Controller) RunTask(ctx context.Context, task Task) error {
	err := controller.resumeTask(ctx, task)
	var failure *IncompleteTaskError
	if !errors.As(err, &failure) {
		return err
	}
	entries, readErr := controller.Evidence.Read(ctx)
	if readErr != nil {
		return errors.Join(err, readErr)
	}
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if entry.TaskID == task.ID && (entry.Phase == "task_resume" || entry.Phase == "response_recovery") {
			break
		}
		if entry.Phase == "task_interrupted" && entry.TaskID == task.ID && entry.Continuation != nil {
			return err
		}
	}
	_, saveErr := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_interrupted", TaskID: task.ID,
		Continuation: &TaskContinuation{Task: task, Reason: failure.Reason, ThreadID: failure.ThreadID, TurnID: failure.TurnID}})
	return errors.Join(err, saveErr)
}
