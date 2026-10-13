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
		}
	}
	return call
}

func (controller *Controller) overallAssignment(workload Workload) Assignment {
	call := controller.assignment(OverallReview, nil, controller.Configuration.Reviewer)
	call.Workload, call.Requirements = &workload, controller.Requirements
	call.Objective = controller.Objective
	call.ArtifactBindings = controller.ArtifactBindings
	for _, task := range workload.Tasks {
		call.ReadFiles = append(call.ReadFiles, task.ReadFiles...)
		call.ReadFiles = append(call.ReadFiles, task.WriteFiles...)
	}
	call.ReadFiles = uniqueFiles(call.ReadFiles)
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
	if err := call.ValidateTaskContext(); err != nil {
		return Result{}, err
	}
	if err := controller.Environment.Attest(ctx); err != nil {
		return Result{}, err
	}
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		return Result{}, err
	}
	taskID := ""
	if call.Task != nil {
		taskID = call.Task.ID
	}
	questions := map[int64]Entry{}
	for _, entry := range entries {
		if entry.Phase == "questions" {
			questions[entry.ID] = entry
		}
		if entry.Phase == "answers" {
			question, known := questions[entry.QuestionID]
			if known && (question.TaskID == taskID && taskID != "" || call.Role == Plan && question.TaskID == "" || call.Role == OverallReview) {
				call.QuestionContext = append(call.QuestionContext, QuestionContext{Questions: question.Questions, Answers: entry.Answers})
			}
		}
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
	var writer FileWriter
	if call.Role == Execute {
		writer = taskFileWriter{environment: controller.Environment, files: call.WriteFiles}
	}
	frames := 0
	result, runErr := provider.Run(ctx, providerCall, writer, func(event NativeEvent) error {
		if len(event.Frame) != 0 {
			if !json.Valid(event.Frame) {
				return fmt.Errorf("agent emitted an invalid native receipt")
			}
			frames++
			if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "native_event", CallID: callID, Role: call.Role, TaskID: taskID, Round: round, Frame: event.Frame}); err != nil {
				return err
			}
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
				granted := call.ReadFiles
				if activity.Kind == model.AgentActivityFileEdit {
					granted = call.WriteFiles
				}
				bound := false
				for _, allowed := range granted {
					if file == allowed {
						bound = true
						break
					}
				}
				if !bound {
					return fmt.Errorf("agent activity names an ungranted file")
				}
			}
			activity.Paths = append([]string(nil), activity.Paths...)
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
