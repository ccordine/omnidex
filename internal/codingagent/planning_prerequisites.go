package codingagent

import (
	"sort"

	"github.com/gryph/omnidex/internal/model"
)

// Earlier queued outcomes inform follow-on planning without becoming mutable
// items or being copied into the selected task's execution context.
type PlanningTaskPrerequisite struct {
	ID          model.LedgerItemID `json:"id"`
	Name        string             `json:"name"`
	Objective   string             `json:"objective,omitempty"`
	Constraints []string           `json:"constraints,omitempty"`
	Acceptance  []string           `json:"acceptance,omitempty"`
}

const selectedItemPlanningScope = "Edit only the supplied ledger item. Keep its identity and implementable scope. Preceding incomplete tasks are read-only planned prerequisites, not completed capabilities or additional work to queue. Build on their stated outcomes without duplicating their scope or copying their instructions into this item. Keep only the requested follow-on changes and any specific dependency references needed by this item. Do not edit predecessors, add tasks, or execute implementation."

func precedingIncompletePlanningTasks(ledger EngineeringLedger, selected model.LedgerItemID, completed []string) []PlanningTaskPrerequisite {
	var target *LedgerTask
	for i := range ledger.Tasks {
		if ledger.Tasks[i].ID == selected {
			target = &ledger.Tasks[i]
			break
		}
	}
	if target == nil {
		return nil
	}
	var earlier []LedgerTask
	for _, task := range ledger.Tasks {
		if task.Disposition != model.LedgerDispositionActive || contains(completed, string(task.ID)) {
			continue
		}
		if task.Rank < target.Rank || task.Rank == target.Rank && task.ID < target.ID {
			earlier = append(earlier, task)
		}
	}
	sort.Slice(earlier, func(i, j int) bool {
		return earlier[i].Rank < earlier[j].Rank || earlier[i].Rank == earlier[j].Rank && earlier[i].ID < earlier[j].ID
	})
	var context []PlanningTaskPrerequisite
	for _, task := range earlier {
		item := PlanningTaskPrerequisite{ID: task.ID, Name: task.Goal, Objective: task.Objective, Constraints: task.Constraints}
		for _, criterion := range task.Acceptance {
			item.Acceptance = append(item.Acceptance, criterion.Text)
		}
		context = append(context, item)
	}
	return context
}
