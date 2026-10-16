package codingagent

import (
	"context"
	"fmt"
)

type taskState struct {
	accepted        map[string]bool
	findings        map[string][]Finding
	tasks           map[string]Task
	started         map[string]bool
	round           int64
	session         *ExecutionSession
	reply           string
	inputRequired   *ExecutionInputRequired
	pendingResponse *Result
	pendingReview   *Result
	reviewQuestion  string
	reviewReply     string
	incomplete      *IncompleteTaskError
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
		state.projectReviewEntry(entry)
		if entry.Round > state.round && (entry.Phase == "call_completed" && entry.Role == Execute ||
			entry.Phase == "execution_response" || entry.Phase == "task_reviewed" || entry.Phase == "task_accepted") {
			state.round = entry.Round
		}
		switch entry.Phase {
		case "task_interrupted":
			if entry.Continuation != nil {
				state.incomplete = &IncompleteTaskError{Reason: entry.Continuation.Reason, ThreadID: entry.Continuation.ThreadID, TurnID: entry.Continuation.TurnID}
			}
		case "task_executing":
			state.round = entry.Round - 1
			state.started[entry.TaskID] = false
		case "task_reviewed":
			state.started[entry.TaskID] = false
			state.reply = ""
		case "execution_session":
			state.session = entry.Session
		case "call_completed":
			if entry.Role == Execute && entry.Result != nil {
				state.pendingResponse = entry.Result
				state.session = &ExecutionSession{ThreadID: entry.Result.ThreadID, TurnID: entry.Result.TurnID, State: "idle"}
			}
		case "execution_response":
			state.pendingResponse = nil
			if len(entry.Notes) != 1 {
				continue
			}
			state.reply = ""
			state.started[task.ID] = entry.ResponseKind == ExecutionDone
			if entry.ResponseKind == ExecutionQuestion || entry.ResponseKind == ExecutionPermission {
				state.round = entry.Round - 1
				state.inputRequired = &ExecutionInputRequired{TaskID: task.ID, Kind: entry.ResponseKind, Text: entry.Notes[0]}
			}
		case "task_incomplete":
			if len(entry.Notes) == 1 {
				state.inputRequired = &ExecutionInputRequired{TaskID: task.ID, Role: Plan, Kind: ExecutionQuestion, Text: entry.Notes[0]}
			}
		case "execution_reply":
			if len(entry.Notes) != 2 {
				continue
			}
			// Native tool input resumes its still-live turn, not a new turn.
			if entry.NativeInput != nil {
				continue
			}
			if entry.Role == Review {
				continue
			}
			state.reply = entry.Notes[1]
			state.inputRequired = nil
			state.started[task.ID] = false
		}
	}
	return state
}

func (controller *Controller) runTask(ctx context.Context, task Task, state *taskState) error {
	if state.inputRequired != nil {
		return state.inputRequired
	}
	for _, dependency := range task.Dependencies {
		if !state.accepted[dependency] {
			return fmt.Errorf("task %s dependency %s is not accepted", task.ID, dependency)
		}
	}
	if !state.started[task.ID] {
		state.started[task.ID] = true
		if err := controller.executeTask(ctx, task, state, nil); err != nil {
			return err
		}
	}
	for ctx.Err() == nil {
		if err := controller.Environment.Attest(ctx); err != nil {
			return err
		}
		scope := controller.assignment(Review, &task, controller.Configuration.Reviewer)
		scope.Findings = state.findings[task.ID]
		scope.Response, scope.Reply = state.reviewQuestion, state.reviewReply
		var err error
		scope.Clarifications, err = controller.taskClarifications(ctx, task.ID)
		if err != nil {
			return err
		}
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
		if len(checks) > 0 && !checksPass(checks) {
			state.findings[task.ID] = nil
			if err := controller.executeTask(ctx, task, state, checks); err != nil {
				return err
			}
			continue
		}
		scope.Checks = checks
		review, reviewResult, err := controller.review(ctx, scope, map[string]Task{task.ID: task}, state.round, state.pendingReview)
		if err != nil {
			return err
		}
		if len(review.Findings) == 0 {
			state.accepted[task.ID] = true
			_, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_accepted", TaskID: task.ID, CallID: reviewResult.CallID, Round: state.round, Checks: checks})
			return err
		}
		state.findings[task.ID] = review.Findings
		if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_reviewed", TaskID: task.ID, Round: state.round, Findings: review.Findings, Checks: checks}); err != nil {
			return err
		}
		if err := controller.executeTask(ctx, task, state, checks); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return ctx.Err()
}

func (controller *Controller) executeTask(ctx context.Context, task Task, state *taskState, checks []CheckResult) error {
	if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_executing", TaskID: task.ID, Round: state.round + 1}); err != nil {
		return err
	}
	execution := controller.assignment(Execute, &task, controller.Configuration.Executor)
	var err error
	execution.Clarifications, err = controller.taskClarifications(ctx, task.ID)
	if err != nil {
		return err
	}
	execution.Session, execution.Reply = state.session, state.reply
	execution.Findings, execution.Checks = state.findings[task.ID], checks
	result, err := controller.invoke(ctx, execution, state.round+1)
	if err != nil {
		return err
	}
	if err := controller.handleExecutionResponse(ctx, task, result, state.round+1); err != nil {
		return err
	}
	state.session = &ExecutionSession{ThreadID: result.ThreadID, TurnID: result.TurnID, State: "idle"}
	state.reply = ""
	state.pendingReview = nil
	state.reviewQuestion, state.reviewReply = "", ""
	state.round++
	return nil
}
