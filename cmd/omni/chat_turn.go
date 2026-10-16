package main

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

type pendingChatJob struct{ jobID, generation int64 }

func (session *chatSession) submitPendingTurn(ctx context.Context, pending *pendingSessionTurn) (client.SessionTurnReceipt, error) {
	if pending == nil {
		return client.SessionTurnReceipt{}, fmt.Errorf("session turn lacks pending operation authority")
	}
	if pending.executionInputID != nil {
		return session.client.SubmitExecutionInput(ctx, session.channel, session.workspaceIdentity, pending.operationID, pending.exactText, *pending.executionInputID)
	}
	if job := pending.activeJob; job != nil {
		return session.client.SubmitObservedSessionTurn(ctx, session.channel, session.workspaceIdentity,
			pending.operationID, pending.exactText, pending.codingMode, job.jobID, job.generation)
	}
	return session.client.SubmitSessionTurn(ctx, session.channel, session.workspaceIdentity,
		pending.operationID, pending.exactText, pending.codingMode)
}

func validatePendingSessionTurn(turn model.ChannelSessionTurn, pending *pendingSessionTurn) error {
	if turn.Text != pending.exactText {
		return fmt.Errorf("persisted session turn differs from pending operation authority")
	}
	if job := pending.activeJob; job != nil && (turn.Disposition != model.ChannelSessionTurnQueued || turn.JobID != job.jobID || turn.Generation != job.generation) {
		return fmt.Errorf("persisted message differs from the observed job authority")
	}
	return nil
}

func (session *chatSession) renderPendingTurnAccepted(receipt client.SessionTurnReceipt, _ *pendingSessionTurn) error {
	return session.renderTurnAccepted(receipt)
}
