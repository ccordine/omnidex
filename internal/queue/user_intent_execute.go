package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/jackc/pgx/v5"
)

type storedUserIntent struct {
	request sessionintent.Request
	current sessionintent.Context
	result  sessionintent.Result
	receipt *sessionintent.Receipt
}

func readUserIntentTx(ctx context.Context, tx pgx.Tx, id model.LifecycleOperationID, channel model.ChannelID, identity string) (storedUserIntent, error) {
	var saved storedUserIntent
	var request, current, result, receipt []byte
	err := tx.QueryRow(ctx, `SELECT request_json,context_json,result_json,receipt_json FROM channel_user_intents WHERE operation_id=$1 AND channel_id=$2 AND workspace_identity=$3 FOR UPDATE`, id, channel, identity).Scan(&request, &current, &result, &receipt)
	if err != nil {
		return saved, err
	}
	for _, pair := range []struct {
		raw    []byte
		target any
	}{{request, &saved.request}, {current, &saved.current}, {result, &saved.result}} {
		if err := json.Unmarshal(pair.raw, pair.target); err != nil {
			return saved, err
		}
	}
	if receipt != nil {
		var value sessionintent.Receipt
		if err := json.Unmarshal(receipt, &value); err != nil {
			return saved, err
		}
		saved.receipt = &value
	}
	return saved, nil
}

func (r *Repository) ExecuteUserIntent(ctx context.Context, id model.LifecycleOperationID, channel model.ChannelID, identity string) (sessionintent.Receipt, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return sessionintent.Receipt{}, err
	}
	defer tx.Rollback(ctx)
	saved, err := readUserIntentTx(ctx, tx, id, channel, identity)
	if err != nil {
		return sessionintent.Receipt{}, err
	}
	if _, err := channelSessionStateTx(ctx, tx, channel, identity); err != nil {
		return sessionintent.Receipt{}, err
	}
	if saved.receipt != nil {
		return *saved.receipt, tx.Commit(ctx)
	}
	if saved.result.Error != "" || saved.result.RequiresApproval || saved.result.Clarification != "" {
		return sessionintent.Receipt{}, fmt.Errorf("this user intention is awaiting an owner response")
	}
	current, err := lockUserIntentContextTx(ctx, tx, saved.current)
	if err != nil {
		return sessionintent.Receipt{}, err
	}
	if err := validateStoredUserDelta(current, saved); err != nil {
		return sessionintent.Receipt{}, err
	}
	receipt, err := r.applyUserIntentTx(ctx, tx, saved, current)
	if err != nil {
		return receipt, err
	}
	if err := saveUserIntentReceiptTx(ctx, tx, id, receipt); err != nil {
		return receipt, err
	}
	return receipt, tx.Commit(ctx)
}

func lockUserIntentContextTx(ctx context.Context, tx pgx.Tx, bound sessionintent.Context) (sessionintent.Context, error) {
	var job model.Job
	if bound.JobID > 0 {
		var err error
		job, err = lockedJobTx(ctx, tx, bound.JobID)
		if err != nil {
			return bound, err
		}
	}
	authority, err := lockChannelTurnAuthorityTx(ctx, tx, bound.State.ChannelID)
	if err != nil {
		return bound, err
	}
	if bound.JobID > 0 {
		if err := validateChannelSessionJobAuthority(bound.State.ChannelID, authority, bound.State.WorkspaceIdentity, job); err != nil {
			return bound, err
		}
	}
	current, err := userIntentContextTx(ctx, tx, bound.State.ChannelID, bound.State.WorkspaceIdentity)
	if err != nil {
		return current, err
	}
	if !current.State.Equal(bound.State) || current.JobStatus != bound.JobStatus || !sameIntentItems(current.Items, bound.Items) || current.Revision != bound.Revision || current.Recoverable != bound.Recoverable || current.JobID != bound.JobID || current.Generation != bound.Generation || current.LedgerID != bound.LedgerID || current.PlanState != bound.PlanState || current.Planning != bound.Planning {
		return current, ErrUserIntentStale
	}
	if current.Prompt != bound.Prompt || !slices.Equal(current.NativeChoices, bound.NativeChoices) {
		return current, ErrUserIntentStale
	}
	if (current.InputID == nil) != (bound.InputID == nil) || current.InputID != nil && *current.InputID != *bound.InputID {
		return current, ErrUserIntentStale
	}
	return current, nil
}

func saveUserIntentReceiptTx(ctx context.Context, tx pgx.Tx, id model.LifecycleOperationID, receipt sessionintent.Receipt) error {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE channel_user_intents SET receipt_json=$2 WHERE operation_id=$1 AND receipt_json IS NULL`, id, raw)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("user action receipt changed during execution")
	}
	return nil
}

func validateStoredUserDelta(current sessionintent.Context, saved storedUserIntent) error {
	displayed := userIntentDisplayedContext(current, saved.request)
	if err := sessionintent.ValidateRoutedPatch(saved.request, displayed, saved.result.Patch); err != nil {
		return err
	}
	after, err := sessionintent.Apply(displayed, saved.result.Patch)
	if err == nil && after.Approval != nil && !reflect.DeepEqual(current.Pending, saved.current.Pending) {
		return ErrUserIntentStale
	}
	return err
}
func sameIntentItems(a, b []sessionintent.Item) bool { return slices.Equal(a, b) }
