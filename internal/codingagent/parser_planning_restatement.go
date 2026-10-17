package codingagent

import (
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

// A response can describe an edit and then quote the resulting wording. Code
// accounts for that exact restatement without asking for the same edit again.
// Only an earlier applied item in this response can supply this evidence, and
// the complete text must still be present in that item's current active state.
func representedPlanningStatement(source string, current EngineeringLedger, operations []model.LedgerMutationOperation) model.LedgerItemID {
	lines := strings.Split(strings.TrimSpace(source), "\n")
	for index, line := range lines {
		lines[index] = strings.TrimPrefix(line, "> ")
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		return ""
	}
	for _, operation := range operations {
		if operation.Item == nil || operation.Item.Base() == nil {
			continue
		}
		id := operation.Item.Base().ID
		retained := retainedPlanningItem(current, id)
		if retained == nil || retained.Base().Disposition != model.LedgerDispositionActive {
			continue
		}
		for _, value := range planningItemStatements(operation.Item) {
			if strings.TrimSpace(value) == text && contains(planningItemStatements(retained), value) {
				return id
			}
		}
	}
	return ""
}

func planningItemStatements(item *model.LedgerMutationItem) []string {
	switch {
	case item.Objective != nil:
		return []string{item.Objective.Outcome}
	case item.Requirement != nil:
		return []string{item.Requirement.Outcome}
	case item.Constraint != nil:
		return []string{item.Constraint.Rule}
	case item.Group != nil:
		return []string{item.Group.Title}
	case item.Task != nil:
		return []string{item.Task.Goal, item.Task.DeliveryObligation}
	case item.AcceptanceCriterion != nil:
		return []string{item.AcceptanceCriterion.Outcome}
	case item.Check != nil:
		return []string{item.Check.Observation}
	default:
		return nil
	}
}
