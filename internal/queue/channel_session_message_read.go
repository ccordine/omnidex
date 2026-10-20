package queue

import (
	"context"
	"encoding/json"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) NextChannelSessionMessage(ctx context.Context, authority model.StepAttemptAuthority) (*ChannelSessionMessage, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return nil, err
	}
	if jobStatus == model.JobStatusWaiting && stepStatus == model.StepStatusWaiting {
		waiting, err := codingNativeInputWaitingTx(ctx, tx, authority)
		if err != nil {
			return nil, err
		}
		if waiting {
			return nil, tx.Commit(ctx)
		}
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return nil, staleStepAttemptError(authority, "message consumer requires running authority", nil)
	}
	var planning bool
	if err := tx.QueryRow(ctx, `SELECT action='v3_coding_plan' FROM job_steps WHERE id=$1`, authority.StepID).Scan(&planning); err != nil {
		return nil, err
	}
	var operationID model.LifecycleOperationID
	err = tx.QueryRow(ctx, `SELECT operation_id FROM lifecycle_operation_registry AS intake
	 WHERE kind='channel_session_message' AND message_job_id=$1
 AND (message_generation=$2 OR ($3 AND message_generation<$2 AND command_payload->'planning_binding' IS NOT NULL))
 AND ($3 OR NOT EXISTS(SELECT 1 FROM evidence WHERE kind='coding_planning_amendment' AND source_ref=intake.operation_id))
	 AND NOT EXISTS(SELECT 1 FROM evidence WHERE kind='channel_session_message' AND source_ref=intake.operation_id)
	 ORDER BY created_at,operation_id LIMIT 1`, authority.JobID, authority.Generation, planning).Scan(&operationID)
	if err == pgx.ErrNoRows {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	message, _, err := readChannelSessionMessageTx(ctx, tx, operationID)
	if err != nil {
		return nil, err
	}
	channel, err := lockChannelTurnAuthorityTx(ctx, tx, message.Command.ChannelID)
	if err != nil {
		return nil, err
	}
	if err := validateAssistantSessionAuthority(message.Command, channel); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &message, nil
}

func readChannelSessionMessageResolutionTx(ctx context.Context, tx pgx.Tx, id model.LifecycleOperationID) (channelSessionMessageResolution, bool, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT payload_json FROM evidence WHERE kind='channel_session_message' AND source_ref=$1`, id).Scan(&raw)
	if err == pgx.ErrNoRows {
		return channelSessionMessageResolution{}, false, nil
	}
	if err != nil {
		return channelSessionMessageResolution{}, false, err
	}
	var resolution channelSessionMessageResolution
	if err := json.Unmarshal(raw, &resolution); err != nil {
		return resolution, false, err
	}
	return resolution, true, nil
}

func requireNoPendingChannelSessionMessagesTx(ctx context.Context, tx pgx.Tx, jobID, generation int64) error {
	var pending bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_operation_registry AS intake
	 WHERE kind='channel_session_message' AND message_job_id=$1 AND message_generation=$2
	 AND NOT EXISTS(SELECT 1 FROM evidence WHERE kind='channel_session_message' AND source_ref=intake.operation_id))`, jobID, generation).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return ErrPendingChannelSessionMessage
	}
	return nil
}
