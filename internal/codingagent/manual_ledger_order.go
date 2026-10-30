package codingagent

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/model"
)

func moveLedgerItem(ledger EngineeringLedger, turn PlanningTurn, action DirectPlanningAction) (LedgerMutation, error) {
	var tasks []LedgerTask
	for _, task := range ledger.Tasks {
		if task.Disposition == model.LedgerDispositionActive {
			tasks = append(tasks, task)
		}
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].Rank < tasks[j].Rank || tasks[i].Rank == tasks[j].Rank && tasks[i].ID < tasks[j].ID
	})
	selected := -1
	for i, task := range tasks {
		if task.ID == action.TargetID {
			selected = i
			break
		}
	}
	if selected < 0 {
		return LedgerMutation{}, fmt.Errorf("selected task is unavailable")
	}
	neighbor := max(0, min(len(tasks)-1, selected+action.Move))
	selectedRank, neighborRank := tasks[selected].Rank, tasks[neighbor].Rank
	unique := selected != neighbor && selectedRank != neighborRank
	for i, task := range tasks {
		if i != selected && i != neighbor && (task.Rank == selectedRank || task.Rank == neighborRank) {
			unique = false
		}
	}
	if unique {
		operations := []model.LedgerMutationOperation{{Kind: model.LedgerMutationReorder, TargetID: tasks[selected].ID, Rank: &neighborRank, Rationale: action.Rationale}, {Kind: model.LedgerMutationReorder, TargetID: tasks[neighbor].ID, Rank: &selectedRank, Rationale: action.Rationale}}
		mutation := LedgerMutation{OperationID: turn.OperationID, OwnerTurnID: turn.OwnerTurnID, BaseRevision: turn.ObservedRevision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{tasks[selected].ID, tasks[neighbor].ID}), Operations: operations}
		return mutation, mutation.Validate()
	}
	tasks[selected], tasks[neighbor] = tasks[neighbor], tasks[selected]
	operations := []model.LedgerMutationOperation{}
	ids := []model.LedgerItemID{}
	for i, task := range tasks {
		rank := int64(i)
		if task.Rank != rank {
			operations = append(operations, model.LedgerMutationOperation{Kind: model.LedgerMutationReorder, TargetID: task.ID, Rank: &rank, Rationale: action.Rationale})
			ids = append(ids, task.ID)
		}
	}
	if len(operations) == 0 {
		rank := tasks[neighbor].Rank
		operations = append(operations, model.LedgerMutationOperation{Kind: model.LedgerMutationReorder, TargetID: action.TargetID, Rank: &rank, Rationale: action.Rationale})
		ids = append(ids, action.TargetID)
	}
	mutation := LedgerMutation{OperationID: turn.OperationID, OwnerTurnID: turn.OwnerTurnID, BaseRevision: turn.ObservedRevision, OwnerTurnDisposition: model.RepresentedOwnerTurn(ids), Operations: operations}
	return mutation, mutation.Validate()
}
