package queue

import (
	"context"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// Controls retire ordinary messages. Frozen-bound planning input survives
// interruption/replan on the same job; terminal controls retire it as well.
func retireChannelSessionMessagesTx(ctx context.Context, tx pgx.Tx, job model.Job, generation int64, controlID model.LifecycleOperationID) error {
	rows, err := tx.Query(ctx, `SELECT intake.operation_id FROM lifecycle_operation_registry AS intake
	 WHERE kind='channel_session_message' AND message_job_id=$1
 AND (message_generation=$2 OR ($4 AND message_generation<$2 AND command_payload->'planning_binding' IS NOT NULL))
 AND ($4 OR command_payload->'planning_binding' IS NULL)
	 AND command_payload->>'redirect_operation_id'<>$3
	 AND NOT EXISTS(SELECT 1 FROM evidence WHERE kind='channel_session_message' AND source_ref=intake.operation_id)
	 ORDER BY created_at,operation_id`, job.ID, generation, controlID, terminalJobStatus(job.Status))
	if err != nil {
		return err
	}
	var ids []model.LifecycleOperationID
	for rows.Next() {
		var id model.LifecycleOperationID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		intake, _, err := readChannelSessionMessageTx(ctx, tx, id)
		if err != nil {
			return err
		}
		var stepID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM job_steps WHERE job_id=$1 AND generation=$2 ORDER BY sort_index,id LIMIT 1`, job.ID, generation).Scan(&stepID); err != nil {
			return err
		}
		resolution := channelSessionMessageResolution{OperationID: id, Kind: "retired", Job: job, ControlOperationID: controlID}
		if err := insertChannelSessionMessageResolutionTx(ctx, tx, intake, stepID, resolution); err != nil {
			return err
		}
	}
	return nil
}
