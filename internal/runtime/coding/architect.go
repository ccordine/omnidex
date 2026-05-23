package coding

import (
	"context"
	"strings"
)

type deterministicArchitect struct{}

func (deterministicArchitect) Queue(_ context.Context, task CodingPlannerTask) (ArchitectQueue, error) {
	intent := strings.TrimSpace(task.Objective)
	if intent == "" {
		intent = "apply requested coding change"
	}
	return ArchitectQueue{
		TaskID: task.ID,
		Reads: []ReadStep{{
			ID:     "read_1",
			Intent: "inspect files in task scope",
		}},
		Tests:   []ChangeStep{},
		Deletes: []ChangeStep{},
		Validations: []ValidationStep{{
			ID:     "validation_1",
			Kind:   "evidence_only",
			Intent: "validate the completed task",
		}},
		Writes: []ChangeStep{{
			ID:        "change_1",
			Kind:      "update_code",
			Intent:    intent,
			Validator: "change_validator",
		}},
	}, nil
}
