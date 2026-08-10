package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func insertCognitionActionEventTx(
	ctx context.Context,
	tx pgx.Tx,
	actionID cognition.ActionID,
	authority model.StepAttemptAuthority,
	status CognitionActionStatus,
	detail any,
) error {
	sequence := 0
	switch status {
	case CognitionActionPrepared:
		sequence = 1
	case CognitionActionDispatched:
		sequence = 2
	case CognitionActionSucceeded, CognitionActionFailed:
		sequence = 3
	default:
		return fmt.Errorf("unregistered cognition action event status %q", status)
	}
	payload := struct {
		Schema   string                     `json:"schema"`
		ActionID cognition.ActionID         `json:"action_id"`
		Status   CognitionActionStatus      `json:"status"`
		Actor    model.StepAttemptAuthority `json:"actor"`
		Detail   any                        `json:"detail,omitempty"`
	}{cognitionQueueIdentitySchemaV1, actionID, status, authority, detail}
	raw, digest, err := cognitionJSON(payload)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_action_events (
			action_id,sequence,status,job_id,generation,step_id,actor_attempt,
			actor_worker_id,event_json,event_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, actionID, sequence, status, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID, string(raw), digest); err != nil {
		return fmt.Errorf("insert cognition action %q event %q: %w", actionID, status, err)
	}
	return nil
}
