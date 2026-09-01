package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const MaxChannelSessionTurns = 200

type ChannelSessionTurn struct {
	OperationID LifecycleOperationID          `json:"operation_id"`
	Disposition ChannelSessionTurnDisposition `json:"disposition"`
	Text        string                        `json:"text"`
	JobID       int64                         `json:"job_id"`
	Generation  int64                         `json:"generation"`
	Status      string                        `json:"status"`
	CreatedAt   time.Time                     `json:"created_at"`
}

func listChannelSessionTurnsTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID model.ChannelID,
) ([]ChannelSessionTurn, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT operation_id,disposition,text,job_id,generation,status,created_at
		FROM (
		  SELECT operation.operation_id,operation.disposition,
		         registry.command_payload->>'text' AS text,operation.job_id,
		         operation.result_generation AS generation,
		         operation.result_job->>'status' AS status,operation.created_at
		  FROM channel_session_turn_operations AS operation
		  JOIN lifecycle_operation_registry AS registry
		    ON registry.operation_id=operation.operation_id
		   AND registry.kind=operation.kind
		   AND registry.command_sha256=operation.command_sha256
		  WHERE operation.channel_id=$1
		  UNION ALL
		  SELECT operation.operation_id,'feedback_submitted'::text,
		         operation.command_payload->>'feedback',operation.job_id,
		         operation.result_generation,operation.result_job_status,
		         operation.created_at
		  FROM job_lifecycle_operations AS operation
		  JOIN jobs AS job ON job.id=operation.job_id
		  WHERE job.pipeline='chat' AND job.metadata->>'channel_id'=$1
		    AND operation.kind='submit_feedback'
		) AS history
		ORDER BY created_at DESC,operation_id DESC
		LIMIT $2
	`, channelID, MaxChannelSessionTurns+1)
	if err != nil {
		return nil, false, fmt.Errorf(
			"read channel %q session turn history: %w",
			channelID,
			err,
		)
	}
	defer rows.Close()
	turns := make([]ChannelSessionTurn, 0, MaxChannelSessionTurns+1)
	for rows.Next() {
		var turn ChannelSessionTurn
		if err := rows.Scan(
			&turn.OperationID,
			&turn.Disposition,
			&turn.Text,
			&turn.JobID,
			&turn.Generation,
			&turn.Status,
			&turn.CreatedAt,
		); err != nil {
			return nil, false, err
		}
		if _, err := ParseLifecycleOperationID(string(turn.OperationID)); err != nil {
			return nil, false, err
		}
		if err := validateChannelSessionTurnDisposition(turn.Disposition); err != nil {
			return nil, false, err
		}
		if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, turn.Text); err != nil {
			return nil, false, err
		}
		if turn.JobID < 1 || turn.Generation < 1 || turn.CreatedAt.IsZero() {
			return nil, false, fmt.Errorf(
				"channel %q session history contains invalid turn authority",
				channelID,
			)
		}
		switch turn.Status {
		case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
			model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		default:
			return nil, false, fmt.Errorf(
				"channel %q session turn has unregistered result status %q",
				channelID,
				turn.Status,
			)
		}
		turns = append(turns, turn)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(turns) > MaxChannelSessionTurns
	if truncated {
		turns = turns[:MaxChannelSessionTurns]
	}
	for left, right := 0, len(turns)-1; left < right; left, right = left+1, right-1 {
		turns[left], turns[right] = turns[right], turns[left]
	}
	return turns, truncated, nil
}
