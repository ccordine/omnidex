package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ledgerapi"
	"github.com/gryph/omnidex/internal/model"
)

// Opening an executable revision is a deterministic owner operation. The
// existing worker opens it without invoking a model, then the same pending edit
// is applied through the normal revision transaction.
func (session *chatSession) prepareLedgerManualEdit(pending *pendingPlanMutation, authoritative bool) error {
	digest := sha256.Sum256([]byte(string(pending.operationID) + ":open-manual-edit"))
	turn := client.PlanningTurn{OperationID: fmt.Sprintf("lifecycle_operation_%x", digest), OwnerTurnID: "manual-edit", Text: "Open the existing ledger for this explicit manual edit. Preserve every item and accepted result; no AI planning is requested.", ObservedRevision: pending.revision}
	request := func(ctx context.Context) (ledgerapi.CodingLedgerRevision, error) {
		_, err := session.client.PlanCodingLedgerItem(ctx, session.channel, session.workspaceIdentity, pending.jobID, turn, pending.ledgerAction.TargetID, true)
		if err != nil {
			return ledgerapi.CodingLedgerRevision{}, err
		}
		for {
			ledger, err := session.client.CodingLedger(ctx, session.channel, session.workspaceIdentity, pending.jobID)
			if err != nil {
				return ledger, err
			}
			if ledger.State == "proposed" {
				return ledger, nil
			}
			details, err := session.client.Job(ctx, pending.jobID)
			if err != nil {
				return ledger, err
			}
			if details.Job.Status == model.JobStatusFailed || details.Job.Status == model.JobStatusCanceled {
				return ledger, fmt.Errorf("opening ledger for editing: %s", details.Job.Error)
			}
			timer := time.NewTimer(150 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ledger, ctx.Err()
			case <-timer.C:
			}
		}
	}
	var ledger ledgerapi.CodingLedgerRevision
	var err error
	if authoritative {
		ledger, err = awaitAuthoritativeChatRequest(session.ctx, session.signals, request)
	} else {
		ledger, err = awaitChatRequest(session.ctx, session.signals, request)
	}
	if err != nil {
		return err
	}
	pending.generation, pending.revision, pending.prepared = ledger.Generation, ledger.Revision, true
	if session.active != nil {
		session.active.Job.CurrentGeneration = ledger.Generation
	}
	if view := session.ledgerWorkspace(); view != nil {
		return view.acceptLedger(ledger)
	}
	return nil
}
