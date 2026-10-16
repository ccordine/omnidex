package codingagent

import (
	"context"
	"encoding/json"
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
	result, err := controller.invokeOnce(ctx, call, round)
	if err != nil && !call.Role.IsNative() && ctx.Err() == nil {
		return result, &ParserFailure{Cause: err}
	}
	return result, err
}

func (controller *Controller) invokeOnce(ctx context.Context, call Assignment, round int64) (Result, error) {
	if err := call.ValidateTaskContext(); err != nil {
		return Result{}, err
	}
	if controller.MinifyContext != nil && !(call.Role == Execute && call.Session != nil && call.Reply != "") {
		var err error
		call.Context, err = controller.MinifyContext(ctx, call)
		if err != nil {
			return Result{}, err
		}
	}
	if err := controller.Environment.Attest(ctx); err != nil {
		return Result{}, err
	}
	taskID := ""
	if call.Task != nil {
		taskID = call.Task.ID
	}
	entry := Entry{Generation: controller.Generation, Phase: "call_opened", TaskID: taskID, Round: round, Role: call.Role, Assignment: &call}
	callID, err := controller.Evidence.Append(ctx, entry)
	if err != nil {
		return Result{}, err
	}
	if err := controller.emitCallActivity(callID, call, model.AgentCallActivityStarted); err != nil {
		return Result{}, err
	}
	provider := controller.Providers[call.Route.Provider]
	encodedCall, err := json.Marshal(call)
	if err != nil {
		return Result{}, err
	}
	var providerCall Assignment
	if err := json.Unmarshal(encodedCall, &providerCall); err != nil {
		return Result{}, err
	}
	frames := 0
	result, runErr := provider.Run(ctx, providerCall, func(event NativeEvent) error {
		if len(event.Frame) != 0 {
			if !json.Valid(event.Frame) {
				return fmt.Errorf("agent emitted an invalid native receipt")
			}
			frames++
			if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "native_event", CallID: callID, Role: call.Role, TaskID: taskID, Round: round, Frame: event.Frame}); err != nil {
				return err
			}
		}
		if event.Session != nil {
			if call.Role != Execute || event.Session.ThreadID == "" {
				return fmt.Errorf("native continuation lacks its exact execution assignment")
			}
			if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "execution_session", CallID: callID,
				Role: call.Role, TaskID: taskID, Round: round, Session: event.Session}); err != nil {
				return err
			}
		}
		if event.Input != nil {
			if controller.NativeInput == nil || !call.Role.IsNative() || frames == 0 {
				return fmt.Errorf("native request has no owner conversation handler")
			}
			response, err := controller.NativeInput(ctx, event.Input.Request, callID, taskID)
			if err != nil {
				return err
			}
			event.Input.Response = response
		}
		if event.Activity != nil {
			if frames == 0 {
				return fmt.Errorf("agent activity preceded its original native evidence")
			}
			activity := *event.Activity
			if activity.Source != model.AgentActivityNative || activity.Role != string(call.Role) || activity.TaskID != taskID {
				return fmt.Errorf("agent activity changed its task assignment")
			}
			if err := activity.Validate(); err != nil {
				return err
			}
			for _, file := range activity.Paths {
				if call.Role.IsNative() {
					continue
				}
				if err := ValidatePath(file); err != nil {
					return fmt.Errorf("agent activity path: %w", err)
				}
				if !contains(call.ReadFiles, file) {
					return fmt.Errorf("agent activity escaped its task packet: %s", file)
				}
			}
			activity.Paths = append([]string(nil), activity.Paths...)
			if call.Role == Execute && activity.Kind == model.AgentActivityFileEdit && activity.Status == model.AgentActivityCompleted {
				if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "files_written", TaskID: taskID, CallID: callID, WrittenFiles: activity.Paths}); err != nil {
					return err
				}
			}
			if controller.Activity != nil {
				controller.Activity(activity)
			}
		}
		return nil
	})
	if runErr == nil && (frames == 0 || result.Provider != call.Route.Provider || result.Model != call.Route.Model || !result.Completed || !json.Valid(result.Content) || result.ThreadID == "" || result.TurnID == "") {
		runErr = fmt.Errorf("agent returned unbound or incomplete native evidence")
	}
	if runErr != nil {
		var incomplete *IncompleteTaskError
		if call.Role.IsNative() && ctx.Err() == nil && result.ThreadID != "" && !errors.As(runErr, &incomplete) {
			runErr = &IncompleteTaskError{Reason: "Native session interrupted; inspect partial work and plan continuation without replaying commands: " + runErr.Error()}
		}
		if errors.As(runErr, &incomplete) && result.ThreadID != "" {
			incomplete.ThreadID, incomplete.TurnID = result.ThreadID, result.TurnID
		}
		result.CallID = callID
		if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "call_failed", CallID: callID, Role: call.Role, TaskID: taskID, Round: round}); err != nil {
			return result, errors.Join(runErr, err)
		}
		return result, errors.Join(runErr, controller.emitCallActivity(callID, call, model.AgentCallActivityFailed))
	}
	result.Evidence = nil // Original frames already belong to the queue journal.
	result.CallID = callID
	_, err = controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "call_completed", CallID: callID, Role: call.Role, TaskID: taskID, Round: round, Result: &result})
	if err != nil {
		return result, err
	}
	return result, controller.emitCallActivity(callID, call, model.AgentCallActivityCompleted)
}
