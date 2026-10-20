package worker

import (
	"context"
	"errors"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type planningConversationRepository interface {
	codingLedgerPlanningRepository
	NextChannelSessionMessage(context.Context, model.StepAttemptAuthority) (*queue.ChannelSessionMessage, error)
	ResolvePlanningSessionMessage(context.Context, model.StepAttemptAuthority, model.LifecycleOperationID) (queue.ChannelSessionMessageResult, error)
	CodingLedgerRevisionForOperation(context.Context, int64, string) (int64, bool, error)
	MarkCodingLedgerReviewReady(context.Context, model.StepAttemptAuthority, int64, int64) error
}

func (runtime *nativeRuntimeV3) finishPlanningConversation(ctx context.Context, controller *codingagent.Controller, ledger queue.CodingLedgerRevision) (queue.CodingLedgerRevision, error) {
	return finishPlanningConversation(ctx, runtime.svc.repo, runtime.claim.Authority, controller, ledger)
}

// Each owner reply is applied to the latest persisted draft, FIFO, on the same
// attempt. Nothing cancels the current planner/parser or retires later replies.
func finishPlanningConversation(ctx context.Context, repository planningConversationRepository, authority model.StepAttemptAuthority, controller *codingagent.Controller, ledger queue.CodingLedgerRevision) (queue.CodingLedgerRevision, error) {
	for {
		message, err := repository.NextChannelSessionMessage(ctx, authority)
		if err != nil {
			return ledger, err
		}
		if message == nil {
			err := repository.MarkCodingLedgerReviewReady(ctx, authority, ledger.LedgerID, ledger.Revision)
			if errors.Is(err, queue.ErrPendingChannelSessionMessage) {
				continue // Intake raced with the waiting transition; it stays durable.
			}
			return ledger, err
		}
		turn := codingagent.PlanningTurn{
			OperationID: string(message.OperationID), OwnerTurnID: string(message.OperationID),
			Text: message.Command.Text, ObservedRevision: ledger.Revision,
		}
		_, applied, err := repository.CodingLedgerRevisionForOperation(ctx, ledger.LedgerID, turn.OperationID)
		if err != nil {
			return ledger, err
		}
		if !applied {
			ledger, err = applyCodingLedgerPlanningTransaction(ctx, repository, controller, ledger.LedgerID, turn, nil)
			if err != nil {
				return ledger, err
			}
		}
		if err := controller.PublishPlanningLedger(ctx, turn, ledger.Ledger); err != nil {
			return ledger, err
		}
		if _, err := repository.ResolvePlanningSessionMessage(ctx, authority, message.OperationID); err != nil {
			return ledger, err
		}
	}
}
