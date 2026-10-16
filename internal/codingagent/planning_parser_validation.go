package codingagent

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

func (controller *Controller) planningValidationNotes(ctx context.Context, operation string) ([]string, error) {
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		return nil, err
	}
	var notes []string
	for _, entry := range entries {
		if entry.Phase == "planning_parse_rejected" && len(entry.Notes) == 2 && entry.Notes[0] == operation {
			notes = []string{entry.Notes[1]}
		}
	}
	return notes, nil
}

func (controller *Controller) rejectPlanningParse(ctx context.Context, turn PlanningTurn, role Role, callID int64, cause error) error {
	_, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "planning_parse_rejected", Role: role,
		CallID: callID, Notes: []string{turn.OperationID, cause.Error()}})
	if err != nil {
		return err
	}
	return &ParserFailure{Cause: cause}
}

// Newly proposed constraints need consumers in remaining work. Drafts without
// tasks can still record constraints while implementation is being discussed.
// Existing constraints and accepted work are not reopened.
func validateAddedConstraintConsumers(ledger EngineeringLedger, operations []model.LedgerMutationOperation, completed []string) error {
	remaining := []model.LedgerTask{}
	for _, task := range ledger.Tasks {
		if task.Disposition == model.LedgerDispositionActive && !contains(completed, string(task.ID)) {
			remaining = append(remaining, task)
		}
	}
	if len(remaining) == 0 {
		return nil
	}
	for _, operation := range operations {
		if operation.Kind != model.LedgerMutationAdd || operation.Item == nil || operation.Item.Constraint == nil {
			continue
		}
		constraint := operation.Item.Constraint.ID
		bound := false
		for _, task := range remaining {
			for _, id := range task.ConstraintIDs {
				bound = bound || id == constraint
			}
		}
		if !bound {
			return fmt.Errorf("new constraint %s has no remaining task consumer; bind it through the affected tasks' constraint_ids without changing their other fields", constraint)
		}
	}
	return nil
}
