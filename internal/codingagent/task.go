package codingagent

import (
	"context"
	"encoding/json"
)

type taskState struct {
	accepted          map[string]bool
	findings          map[string][]Finding
	tasks             map[string]Task
	started           map[string]bool
	round             int64
	session           *ExecutionSession
	reviewSession     *ExecutionSession
	resumeIncomplete  bool
	continueExecution bool
	reply             string
	inputRequired     *ExecutionInputRequired
	executionReport   *ExecutionReport
	pendingResponse   *Result
	pendingReview     *Result
	reviewQuestion    string
	reviewReply       string
	incomplete        *IncompleteTaskError
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
		case "response_recovery":
			if entry.Result != nil && len(entry.Notes) == 1 {
				state.incomplete = nil
				state.resumeIncomplete = true
				if entry.Role == Review {
					state.pendingReview = entry.Result
					state.reviewReply = entry.Notes[0]
				} else {
					state.pendingResponse = entry.Result
					state.reply = entry.Notes[0]
				}
			}
		case "task_resume":
			if entry.Session != nil && len(entry.Notes) == 2 {
				state.incomplete = nil
				state.resumeIncomplete = true
				if entry.Role == Review {
					state.pendingReview = nil
					state.reviewSession = entry.Session
					state.reviewQuestion, state.reviewReply = entry.Notes[0], entry.Notes[1]
				} else {
					state.pendingResponse = nil
					state.session = entry.Session
					state.reply = entry.Notes[1]
				}
			}
		case "task_interrupted":
			if entry.Continuation != nil {
				state.incomplete = &IncompleteTaskError{Reason: entry.Continuation.Reason, ThreadID: entry.Continuation.ThreadID, TurnID: entry.Continuation.TurnID}
			}
		case "task_executing":
			state.round = entry.Round - 1
			state.started[entry.TaskID] = false
		case "task_reviewed":
			state.started[entry.TaskID] = false
			state.continueExecution = false
			state.reply = ""
		case "execution_session":
			if entry.Role == Review {
				state.reviewSession = entry.Session
			} else {
				state.session = entry.Session
			}
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
			state.continueExecution = entry.ResponseKind == ExecutionContinue
			if entry.ResponseKind == ExecutionDone || entry.ResponseKind == ExecutionContinue {
				state.executionReport = &ExecutionReport{CallID: entry.CallID, Round: entry.Round, Response: entry.Notes[0]}
			}
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
	if !state.started[task.ID] {
		state.started[task.ID] = true
		if err := controller.executeTask(ctx, task, state); err != nil {
			return err
		}
	}
	for ctx.Err() == nil {
		if err := controller.Environment.Attest(ctx); err != nil {
			return err
		}
		scope := controller.assignment(Review, &task, controller.Configuration.Reviewer)
		scope.ExecutionReport = state.executionReport
		scope.Findings = state.findings[task.ID]
		scope.Session = state.reviewSession
		scope.Response, scope.Reply = state.reviewQuestion, state.reviewReply
		var err error
		scope.Clarifications, err = controller.taskClarifications(ctx, task.ID)
		if err != nil {
			return err
		}
		review, reviewResult, err := controller.review(ctx, scope, map[string]Task{task.ID: task}, state.round, state.pendingReview)
		if err != nil {
			return err
		}
		state.resumeIncomplete = false
		if len(review.Findings) == 0 {
			state.accepted[task.ID] = true
			_, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_accepted", TaskID: task.ID, CallID: reviewResult.CallID, Round: state.round})
			return err
		}
		state.findings[task.ID] = review.Findings
		if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_reviewed", TaskID: task.ID, Round: state.round, Findings: review.Findings}); err != nil {
			return err
		}
		if err := controller.executeTask(ctx, task, state); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return ctx.Err()
}

func (controller *Controller) executeTask(ctx context.Context, task Task, state *taskState) error {
	for ctx.Err() == nil {
		if err := controller.executeTaskTurn(ctx, task, state); err != nil {
			return err
		}
		if !state.continueExecution {
			return nil
		}
	}
	return ctx.Err()
}

func (controller *Controller) executeTaskTurn(ctx context.Context, task Task, state *taskState) error {
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
	// Corrections start fresh. Ordinary unfinished execution continues its
	// existing thread with the authoritative task and persisted progress report.
	// Running turns still retain their exact identity solely for recovery.
	if state.continueExecution {
		execution.ExecutionReport = state.executionReport
	}
	if execution.Session != nil && execution.Session.State == "idle" && !state.resumeIncomplete && !state.continueExecution {
		execution.Session = nil
	}
	execution.Findings = state.findings[task.ID]
	result, err := controller.invoke(ctx, execution, state.round+1)
	if err != nil {
		return err
	}
	kind, err := controller.handleExecutionResponse(ctx, task, result, state.round+1)
	if err != nil {
		return err
	}
	return state.recordExecutionResult(task.ID, result, state.round+1, kind)
}

func (state *taskState) recordExecutionResult(taskID string, result Result, round int64, kind ExecutionResponseKind) error {
	var response string
	if err := json.Unmarshal(result.Content, &response); err != nil {
		return err
	}
	state.executionReport = &ExecutionReport{CallID: result.CallID, Round: round, Response: response}
	state.continueExecution = kind == ExecutionContinue
	state.started[taskID] = kind == ExecutionDone
	state.resumeIncomplete = false
	state.session = &ExecutionSession{ThreadID: result.ThreadID, TurnID: result.TurnID, State: "idle"}
	state.pendingResponse = nil
	state.reply = ""
	state.pendingReview = nil
	state.reviewSession = nil
	state.reviewQuestion, state.reviewReply = "", ""
	state.round = round
	return nil
}
