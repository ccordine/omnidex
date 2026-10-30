package worker

import (
	"context"
	"errors"
	"reflect"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

// Retain a completed semantic result when only order or review controls raced
// with it. Later explicit controls win over the older result. A task amendment
// still requires review of its changed content, as enforced by the repository.
func rebasePlanningControls(base, latest model.EngineeringLedger, mutation model.LedgerMutation) (model.LedgerMutation, bool) {
	if len(base.Tasks) != len(latest.Tasks) {
		return mutation, false
	}
	previous := make(map[model.LedgerItemID]model.LedgerTask, len(base.Tasks))
	current := make(map[model.LedgerItemID]model.LedgerTask, len(latest.Tasks))
	for _, task := range base.Tasks {
		previous[task.ID] = task
	}
	for _, task := range latest.Tasks {
		old, exists := previous[task.ID]
		if !exists {
			return mutation, false
		}
		current[task.ID] = task
		old.Rank, old.ReviewState = task.Rank, task.ReviewState
		if !reflect.DeepEqual(old, task) {
			return mutation, false
		}
	}
	for i := range mutation.Operations {
		op := &mutation.Operations[i]
		old, exists := previous[op.TargetID]
		if !exists {
			continue
		}
		now := current[op.TargetID]
		if old.Rank != now.Rank {
			if op.Kind == model.LedgerMutationReorder {
				rank := now.Rank
				op.Rank = &rank
			}
			if op.Kind == model.LedgerMutationAmend && op.Item != nil && op.Item.Task != nil {
				op.Item.Task.Rank = now.Rank
			}
		}
		if old.ReviewState != now.ReviewState && op.Kind == model.LedgerMutationReview {
			op.ReviewState = now.ReviewState
		}
	}
	mutation.BaseRevision = latest.Revision
	return mutation, true
}

func publishCurrentPlanningLedger(ctx context.Context, repository codingLedgerPlanningRepository, controller *codingagent.Controller, turn codingagent.PlanningTurn, ledger queue.CodingLedgerRevision) (queue.CodingLedgerRevision, error) {
	for {
		if err := ctx.Err(); err != nil {
			return ledger, err
		}
		err := controller.PublishPlanningLedger(ctx, turn, ledger.Ledger)
		if !errors.Is(err, queue.ErrCodingLedgerRevision) {
			return ledger, err
		}
		ledger, err = readCompleteCodingLedger(ctx, repository, ledger.LedgerID)
		if err != nil {
			return ledger, err
		}
	}
}
