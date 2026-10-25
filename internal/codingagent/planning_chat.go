package codingagent

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gryph/omnidex/internal/model"
)

// planningResponse runs the conversation first. Its ordinary text is retained
// independently of the parser, so a response is never replaced by ledger JSON.
func (controller *Controller) planningResponse(ctx context.Context, turn PlanningTurn, ledger *EngineeringLedger) (string, error) {
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		return "", err
	}
	conversation := []PlanningMessage{}
	var continuation *TaskContinuation
	for _, entry := range entries {
		if entry.Phase == "planning_incomplete" && len(entry.Notes) == 1 {
			conversation = append(conversation, PlanningMessage{Role: "assistant", Text: entry.Notes[0]})
			continue
		}
		if entry.Phase == "task_incomplete" && entry.Continuation != nil && len(entry.Notes) == 1 {
			continuation = entry.Continuation
			conversation = append(conversation, PlanningMessage{Role: "assistant", Text: entry.Notes[0]})
			continue
		}
		if (entry.Phase != "planning_draft" && !(entry.Phase == "planning_response" && entry.PlanningProposal == nil)) || len(entry.Notes) != 3 {
			continue
		}
		if entry.Notes[0] == turn.OperationID && samePlanningDirection(entry.NewerOwnerDirection, turn.NewerOwnerDirection) {
			return entry.Notes[2], nil
		}
		conversation = append(conversation, PlanningMessage{Role: "user", Text: entry.Notes[1]},
			PlanningMessage{Role: "assistant", Text: entry.Notes[2]})
	}
	call := controller.assignment(Plan, nil, controller.Configuration.Planner)
	call.Objective = &Objective{Instruction: turn.Text}
	call.PlanningLedger, call.PlanningConversation = ledger, conversation
	call.NewerOwnerDirection = turn.NewerOwnerDirection
	call.Continuation = continuation
	call.CompletedTaskIDs = append([]string{}, controller.CompletedTaskIDs...)
	call.Clarifications, err = controller.taskClarifications(ctx, "")
	if err != nil {
		return "", err
	}
	result, err := controller.invoke(ctx, call, 1)
	if err != nil {
		var incomplete *IncompleteTaskError
		if errors.As(err, &incomplete) {
			text := "Planning was interrupted before completion: " + incomplete.Reason + ". Reply with how to continue planning; no additional scope has been approved."
			if _, saveErr := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "planning_incomplete", CallID: result.CallID, Role: Plan, ResponseKind: ExecutionQuestion, Notes: []string{text}}); saveErr != nil {
				return "", saveErr
			}
			return "", &ExecutionInputRequired{Role: Plan, Kind: ExecutionQuestion, Text: text}
		}
		return "", err
	}
	var response string
	if err := json.Unmarshal(result.Content, &response); err != nil {
		return "", err
	}
	_, err = controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "planning_draft",
		Role: Plan, CallID: result.CallID, NewerOwnerDirection: turn.NewerOwnerDirection,
		Notes: []string{turn.OperationID, turn.Text, response}})
	return response, err
}

func samePlanningDirection(left, right *PlanningDirection) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func parsedPlanningDisposition(ledger EngineeringLedger, operations []model.LedgerMutationOperation) model.OwnerTurnDisposition {
	ids := []model.LedgerItemID{}
	if operations == nil {
		for _, item := range planningItemPointers(&ledger) {
			if item.Disposition == model.LedgerDispositionActive {
				ids = append(ids, item.ID)
			}
		}
	} else {
		seen := map[model.LedgerItemID]bool{}
		for _, operation := range operations {
			id := operation.TargetID
			if operation.Kind == model.LedgerMutationAdd {
				id = planningMutationItemID(operation.Item)
			}
			if id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return model.OwnerTurnDisposition{Statements: []model.OwnerStatementDisposition{}}
	}
	return model.RepresentedOwnerTurn(ids)
}
