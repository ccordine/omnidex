package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const MaxChannelSessionControls = 200

type ChannelSessionControlKind string

const (
	ChannelSessionControlInterrupt ChannelSessionControlKind = "interrupt"
	ChannelSessionControlReplan    ChannelSessionControlKind = "replan"
	ChannelSessionControlCancel    ChannelSessionControlKind = "cancel"
)

type ChannelSessionControl struct {
	OperationID LifecycleOperationID      `json:"operation_id"`
	Kind        ChannelSessionControlKind `json:"kind"`
	Text        string                    `json:"text"`
	JobID       int64                     `json:"job_id"`
	Generation  int64                     `json:"generation"`
	Status      string                    `json:"status"`
	CreatedAt   time.Time                 `json:"created_at"`
}

func listChannelSessionControlsTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID model.ChannelID,
) ([]ChannelSessionControl, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT operation.operation_id,operation.kind,
		       CASE operation.kind
		         WHEN 'cancel_job' THEN operation.command_payload->>'reason'
		         ELSE operation.command_payload->>'feedback'
		       END,
		       operation.job_id,operation.result_generation,
		       operation.result_job_status,operation.created_at
		FROM job_lifecycle_operations AS operation
		JOIN jobs AS job ON job.id=operation.job_id
		WHERE job.pipeline='chat' AND job.metadata->>'channel_id'=$1
		  AND operation.kind IN ('interrupt_job','replan_job','cancel_job')
		ORDER BY operation.created_at DESC,operation.operation_id DESC
		LIMIT $2
	`, channelID, MaxChannelSessionControls+1)
	if err != nil {
		return nil, false, fmt.Errorf(
			"read channel %q session controls: %w",
			channelID,
			err,
		)
	}
	defer rows.Close()
	controls := make([]ChannelSessionControl, 0, MaxChannelSessionControls+1)
	for rows.Next() {
		var control ChannelSessionControl
		var kind LifecycleOperationKind
		if err := rows.Scan(
			&control.OperationID,
			&kind,
			&control.Text,
			&control.JobID,
			&control.Generation,
			&control.Status,
			&control.CreatedAt,
		); err != nil {
			return nil, false, fmt.Errorf(
				"scan channel %q session control: %w",
				channelID,
				err,
			)
		}
		if _, err := ParseLifecycleOperationID(string(control.OperationID)); err != nil {
			return nil, false, err
		}
		switch kind {
		case LifecycleInterruptJob:
			control.Kind = ChannelSessionControlInterrupt
			if _, _, err := validateInterruptFeedback(control.Text); err != nil {
				return nil, false, err
			}
			if control.Generation < 2 || control.Status != model.JobStatusWaiting {
				return nil, false, fmt.Errorf(
					"channel %q interrupt operation %q has contradictory result authority",
					channelID,
					control.OperationID,
				)
			}
		case LifecycleReplanJob:
			control.Kind = ChannelSessionControlReplan
			if _, _, err := validateReplanFeedback(control.Text); err != nil {
				return nil, false, err
			}
			if control.Generation < 2 || control.Status != model.JobStatusRunning {
				return nil, false, fmt.Errorf(
					"channel %q replan operation %q has contradictory result authority",
					channelID,
					control.OperationID,
				)
			}
		case LifecycleCancelJob:
			control.Kind = ChannelSessionControlCancel
			if _, err := validateCancelReason(control.Text); err != nil {
				return nil, false, err
			}
			if control.Generation < 1 || control.Status != model.JobStatusCanceled {
				return nil, false, fmt.Errorf(
					"channel %q cancel operation %q has contradictory result authority",
					channelID,
					control.OperationID,
				)
			}
		default:
			return nil, false, fmt.Errorf(
				"channel %q session control has unregistered kind %q",
				channelID,
				kind,
			)
		}
		if control.JobID < 1 || control.CreatedAt.IsZero() {
			return nil, false, fmt.Errorf(
				"channel %q session control has invalid persisted authority",
				channelID,
			)
		}
		controls = append(controls, control)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(controls) > MaxChannelSessionControls
	if truncated {
		controls = controls[:MaxChannelSessionControls]
	}
	for left, right := 0, len(controls)-1; left < right; left, right = left+1, right-1 {
		controls[left], controls[right] = controls[right], controls[left]
	}
	return controls, truncated, nil
}
