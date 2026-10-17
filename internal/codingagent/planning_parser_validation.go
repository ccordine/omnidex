package codingagent

import (
	"context"
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
