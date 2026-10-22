package worker

import (
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

// The planner supplies task meaning, while code retains the exact source text.
// Persist it in the existing task payload before approval; execution and review
// then consume the same frozen packet without reconstructing source material.
func bindPlanningTaskSource(task, retained *model.LedgerTask, turn codingagent.PlanningTurn, revision int64) {
	task.OwnerContext = append([]model.TaskContext{}, task.OwnerContext...)
	bind := func(source model.TaskContext) {
		for index := range task.OwnerContext {
			if task.OwnerContext[index].Text == source.Text {
				task.OwnerContext[index].Provenance = source.Provenance
				return
			}
		}
		task.OwnerContext = append(task.OwnerContext, source)
	}
	if retained != nil {
		for _, source := range retained.OwnerContext {
			if source.Provenance.Kind == "owner_turn" {
				bind(source)
			}
		}
	}
	bind(model.TaskContext{Text: turn.Text, Provenance: model.LedgerProvenance{
		Kind: "owner_turn", Reference: turn.OwnerTurnID, Revision: revision,
	}})
}
