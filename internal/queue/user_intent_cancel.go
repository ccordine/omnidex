package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
	"github.com/jackc/pgx/v5"
)

// A physical client interruption revokes this message, not a new interpretation
// of it. The operation lock also waits out an in-flight interpretation; the row
// lock waits out execution and returns its committed receipt if it already ran.
func (r *Repository) CancelUserIntent(ctx context.Context, request sessionintent.Request) (sessionintent.Receipt, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return sessionintent.Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockLifecycleOperationIdentityTx(ctx, tx, request.OperationID); err != nil {
		return sessionintent.Receipt{}, err
	}
	if _, err := channelSessionStateTx(ctx, tx, request.State.ChannelID, request.State.WorkspaceIdentity); err != nil {
		return sessionintent.Receipt{}, err
	}
	saved, err := readUserIntentTx(ctx, tx, request.OperationID, request.State.ChannelID, request.State.WorkspaceIdentity)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return sessionintent.Receipt{}, err
	}
	if err == nil {
		original, encodeErr := json.Marshal(saved.request)
		if encodeErr != nil {
			return sessionintent.Receipt{}, encodeErr
		}
		expected, encodeErr := json.Marshal(request)
		if encodeErr != nil {
			return sessionintent.Receipt{}, encodeErr
		}
		if !sameCodingLedgerJSON(original, expected) {
			return sessionintent.Receipt{}, ErrLifecycleOperationConflict
		}
		if saved.receipt != nil {
			return *saved.receipt, tx.Commit(ctx)
		}
	} else {
		if err := rejectLifecycleUserIntentCollisionTx(ctx, tx, request.OperationID); err != nil {
			return sessionintent.Receipt{}, err
		}
		if request.OperationID == "" || request.Text == "" {
			return sessionintent.Receipt{}, fmt.Errorf("message cancellation requires exact request authority")
		}
		requestJSON, err := json.Marshal(request)
		if err != nil {
			return sessionintent.Receipt{}, err
		}
		contextJSON, err := json.Marshal(sessionintent.Context{State: request.State})
		if err != nil {
			return sessionintent.Receipt{}, err
		}
		resultJSON, err := json.Marshal(sessionintent.Result{OperationID: request.OperationID, Patch: []userintent.Patch{}, Error: "The owner interrupted this message."})
		if err != nil {
			return sessionintent.Receipt{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO channel_user_intents(operation_id,channel_id,workspace_identity,request_json,context_json,result_json,evidence_json) VALUES($1,$2,$3,$4,$5,$6,'[]')`, request.OperationID, request.State.ChannelID, request.State.WorkspaceIdentity, requestJSON, contextJSON, resultJSON)
		if err != nil {
			return sessionintent.Receipt{}, err
		}
	}
	receipt := sessionintent.Receipt{OperationID: request.OperationID, Patch: []userintent.Patch{}, Interrupted: true, Summary: "Message interrupted; no action was executed."}
	if err := saveUserIntentReceiptTx(ctx, tx, request.OperationID, receipt); err != nil {
		return receipt, err
	}
	return receipt, tx.Commit(ctx)
}
