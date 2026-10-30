package codingagent

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/model"
)

func (controller *Controller) assignment(role Role, task *Task, route Route) Assignment {
	call := Assignment{JobID: controller.JobID, Generation: controller.Generation, Root: controller.Root, Role: role, Task: task, Route: route}
	if task != nil {
		call.ReadFiles = uniqueFiles(append(append([]string{}, task.ReadFiles...), task.WriteFiles...))
		if role == Execute {
			call.WriteFiles = append([]string{}, task.WriteFiles...)
			call.DeleteFiles = append([]string{}, task.DeleteFiles...)
			call.ReadFiles = uniqueFiles(append(call.ReadFiles, task.DeleteFiles...))
		}
	}
	return call
}

func uniqueFiles(files []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, file := range files {
		if !seen[file] {
			seen[file] = true
			result = append(result, file)
		}
	}
	sort.Strings(result)
	return result
}

func (controller *Controller) invoke(ctx context.Context, call Assignment, round int64) (Result, error) {
	taskID := ""
	if call.Task != nil {
		taskID = call.Task.ID
	}
	callID, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "call_opened", TaskID: taskID, Round: round, Role: call.Role, Assignment: &call})
	if err != nil {
		return Result{}, err
	}
	_ = controller.emitCallActivity(callID, call, model.AgentCallActivityStarted)
	result, err := controller.Providers[call.Route.Provider].Run(ctx, call, func(event NativeEvent) error {
		if event.Activity != nil && controller.Activity != nil {
			controller.Activity(*event.Activity)
		}
		if event.Input != nil {
			if controller.NativeInput == nil {
				return fmt.Errorf("native input is unavailable")
			}
			response, inputErr := controller.NativeInput(ctx, event.Input.Request, callID, taskID)
			if inputErr != nil {
				return inputErr
			}
			event.Input.Response = response
		}
		return nil
	})
	result.CallID = callID
	if err != nil {
		_, saveErr := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "call_failed", CallID: callID, Role: call.Role, TaskID: taskID, Round: round})
		_ = controller.emitCallActivity(callID, call, model.AgentCallActivityFailed)
		if !call.Role.IsNative() && ctx.Err() == nil {
			err = &ParserFailure{Cause: err}
		}
		return result, errors.Join(err, saveErr)
	}
	result.Evidence = nil
	_, err = controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "call_completed", CallID: callID, Role: call.Role, TaskID: taskID, Round: round, Result: &result})
	_ = controller.emitCallActivity(callID, call, model.AgentCallActivityCompleted)
	return result, err
}
