package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// IncompleteTaskError is a code-owned handoff, never an agent-authored queue
// decision. The task remains unaccepted until owner-approved planning resolves it.
type IncompleteTaskError struct {
	Reason   string
	ThreadID string
	TurnID   string
}

func (failure *IncompleteTaskError) Error() string { return failure.Reason }

type TaskContinuation struct {
	LedgerRevision int64               `json:"ledger_revision,omitempty"`
	Task           Task                `json:"task"`
	Reason         string              `json:"reason"`
	ThreadID       string              `json:"thread_id,omitempty"`
	TurnID         string              `json:"turn_id,omitempty"`
	Clarifications []TaskClarification `json:"clarifications,omitempty"`
}

func (controller *Controller) PlanContinuation(ctx context.Context, task Task, failure *IncompleteTaskError, ledger EngineeringLedger) error {
	call := controller.assignment(Plan, nil, controller.Configuration.Planner)
	call.Objective = &Objective{Instruction: "Plan the remaining work for this incomplete approved task. Inspect the actual partial work using normal tools. Preserve completed work and the existing scope. Propose remaining ledger items ahead of untouched work, and ask the owner to decide any scope expansion. Do not execute the proposed work."}
	call.PlanningLedger = &ledger
	call.CompletedTaskIDs = append([]string{}, controller.CompletedTaskIDs...)
	call.Continuation = &TaskContinuation{LedgerRevision: ledger.Revision, Task: task, Reason: failure.Reason, ThreadID: failure.ThreadID, TurnID: failure.TurnID}
	var err error
	call.Continuation.Clarifications, err = controller.taskClarifications(ctx, task.ID)
	if err != nil {
		return err
	}
	call.Clarifications, err = controller.taskClarifications(ctx, "")
	if err != nil {
		return err
	}
	result, err := controller.invoke(ctx, call, 0)
	var response string
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var parserFailure *ParserFailure
		if errors.As(err, &parserFailure) {
			return err
		}
		response = "The approved task is incomplete. Continuation planning was interrupted: " + err.Error() + ". Reply to continue planning; no new work has been approved or executed."
	} else if err := json.Unmarshal(result.Content, &response); err != nil {
		return fmt.Errorf("decode continuation planning response: %w", err)
	}
	_, err = controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_incomplete", TaskID: task.ID,
		CallID: result.CallID, Role: Plan, ResponseKind: ExecutionQuestion, Notes: []string{response}, Continuation: call.Continuation})
	return err
}
