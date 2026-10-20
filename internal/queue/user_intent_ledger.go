package queue

import (
	"context"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/jackc/pgx/v5"
)

func applyUserLedgerDeltaTx(ctx context.Context, tx pgx.Tx, saved storedUserIntent, after sessionintent.Snapshot, receipt *sessionintent.Receipt) error {
	current, request := saved.current, saved.request
	operations := []model.LedgerMutationOperation{}
	ids := []model.LedgerItemID{}
	for i, item := range after.Items {
		old := current.Items[i]
		if old.ReviewState == item.ReviewState && old.Disposition == item.Disposition {
			continue
		}
		kind, review := model.LedgerMutationReview, item.ReviewState
		if item.Disposition != old.Disposition {
			kind, review = model.LedgerMutationRemove, ""
		}
		operations = append(operations, model.LedgerMutationOperation{Kind: kind, TargetID: item.ID, ReviewState: review, Rationale: "Explicit owner state delta against the exact displayed revision."})
		ids = append(ids, item.ID)
	}
	if len(operations) > 0 {
		id := string(request.OperationID) + "_ledger"
		mutation := model.LedgerMutation{OperationID: id, OwnerTurnID: id, BaseRevision: receipt.Revision, Operations: operations, OwnerTurnDisposition: model.RepresentedOwnerTurn(ids)}
		if err := mutation.Validate(); err != nil {
			return err
		}
		updated, err := applyCodingLedgerMutationTx(ctx, tx, ApplyCodingLedgerMutationCommand{LedgerID: current.LedgerID, OwnerTurnText: request.Text, Mutation: mutation}, "planning_control")
		if err != nil {
			return err
		}
		receipt.Revision = updated.Revision
		receipt.Summary = "Displayed plan state updated."
	}
	if after.PlanState != current.PlanState {
		frozen, err := freezeCodingLedgerTx(ctx, tx, FreezeCodingLedgerCommand{LedgerID: current.LedgerID, Revision: receipt.Revision, OperationID: string(request.OperationID) + "_freeze", OwnerTurnID: string(request.OperationID) + "_freeze", OwnerTurnText: request.Text})
		if err != nil {
			return err
		}
		if err := completeCodingLedgerReviewTx(ctx, tx, current.JobID, current.LedgerID, frozen.Revision); err != nil {
			return err
		}
		receipt.Revision, receipt.Status = frozen.Revision, model.JobStatusRunning
		receipt.Summary = "Plan frozen; coding started."
	}
	return nil
}
