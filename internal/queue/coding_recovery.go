package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// Recovery consumes only the latest job's unchanged frozen authority. It never
// revives a cancelled/completed ledger or changes an accepted task contract.
func codingRecoveryStepTx(ctx context.Context, tx pgx.Tx, job model.Job) (int64, error) {
	var stepID int64
	err := tx.QueryRow(ctx, `SELECT step.id FROM coding_ledgers ledger
 JOIN job_steps step ON step.job_id=ledger.job_id AND step.generation=$2
 WHERE ledger.job_id=$1 AND ledger.state='frozen' AND ledger.frozen_revision=ledger.current_revision
 AND step.action='v3_coding' AND step.superseded_at_generation IS NULL
 AND (( $3='failed' AND step.status='failed') OR
      ($3='waiting_input' AND step.status='waiting_input' AND EXISTS(
        SELECT 1 FROM job_lifecycle_operations operation WHERE operation.job_id=$1
        AND operation.kind='interrupt_job' AND operation.step_id=step.id
        AND operation.result_generation=$2 AND operation.observed_generation=$2)))
 AND NOT EXISTS(SELECT 1 FROM jobs newer WHERE (newer.metadata->>'channel_id'=(SELECT metadata->>'channel_id' FROM jobs WHERE id=$1) OR (newer.pipeline='coding' AND newer.metadata->>'client_workspace_identity'=(SELECT metadata->>'client_workspace_identity' FROM jobs WHERE id=$1)))
   AND newer.id>$1)
 ORDER BY step.id DESC LIMIT 1`, job.ID, job.CurrentGeneration, job.Status).Scan(&stepID)
	return stepID, err
}

func resumeFrozenCodingTx(ctx context.Context, tx pgx.Tx, job model.Job, ownerText string) (model.Job, error) {
	stepID, err := codingRecoveryStepTx(ctx, tx, job)
	if err != nil {
		return model.Job{}, fmt.Errorf("frozen coding recovery is unavailable: %w", err)
	}
	if err := retainCodingContinuationTx(ctx, tx, job, stepID, ownerText); err != nil {
		return model.Job{}, err
	}
	if job.Status == model.JobStatusFailed {
		// Retain the failed step/attempt as immutable history; the existing evidence
		// reader reconciles matching same-generation execution steps.
		_, err = tx.Exec(ctx, `INSERT INTO job_steps(job_id,generation,action,sort_index,status)
    SELECT job_id,generation,action,sort_index,'pending' FROM job_steps WHERE id=$1`, stepID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE job_steps SET status=CASE WHEN current_attempt=0 THEN 'pending' ELSE 'running' END,worker_id=(SELECT worker_id FROM job_step_attempts WHERE step_id=$1 AND attempt=current_attempt),updated_at=clock_timestamp() WHERE id=$1`, stepID)
	}
	if err != nil {
		return model.Job{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET status='running',error=NULL,completed_at=NULL,updated_at=clock_timestamp() WHERE id=$1`, job.ID); err != nil {
		return model.Job{}, err
	}
	return scanLockedJobTx(ctx, tx, job.ID)
}

func pauseFrozenCodingTx(ctx context.Context, tx pgx.Tx, job model.Job, descriptor lifecycleOperationDescriptor) (LifecycleJobResult, bool, error) {
	var stepID int64
	err := tx.QueryRow(ctx, `SELECT step.id FROM coding_ledgers ledger JOIN job_steps step ON step.job_id=ledger.job_id
 WHERE ledger.job_id=$1 AND ledger.state='frozen' AND ledger.current_revision=ledger.frozen_revision
 AND step.generation=$2 AND step.action='v3_coding' AND step.status IN ('pending','running')
 AND step.superseded_at_generation IS NULL ORDER BY step.id DESC LIMIT 1 FOR UPDATE OF step`, job.ID, job.CurrentGeneration).Scan(&stepID)
	if err == pgx.ErrNoRows {
		return LifecycleJobResult{}, false, nil
	}
	if err != nil {
		return LifecycleJobResult{}, true, err
	}
	var attempt int64
	var worker string
	if err := tx.QueryRow(ctx, `SELECT current_attempt,COALESCE(worker_id,'') FROM job_steps WHERE id=$1`, stepID).Scan(&attempt, &worker); err != nil {
		return LifecycleJobResult{}, true, err
	}
	if attempt > 0 {
		// The owner pause revokes current work even after its worker lease expired.
		// Preserve an already waiting attempt rather than mutating terminal history.
		result, err := tx.Exec(ctx, `UPDATE job_step_attempts SET status='waiting_input',finished_at=clock_timestamp()
    WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5 AND status='active'`, job.ID, job.CurrentGeneration, stepID, attempt, worker)
		if err != nil {
			return LifecycleJobResult{}, true, err
		}
		if result.RowsAffected() != 1 {
			var status model.StepAttemptStatus
			if err := tx.QueryRow(ctx, `SELECT status FROM job_step_attempts WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5`, job.ID, job.CurrentGeneration, stepID, attempt, worker).Scan(&status); err != nil {
				return LifecycleJobResult{}, true, err
			}
			if status != model.StepAttemptWaiting {
				return LifecycleJobResult{}, true, fmt.Errorf("pause lost its exact active or waiting attempt")
			}
		}
	}
	if _, err := reserveLifecycleOperationIdentityTx(ctx, tx, descriptor.ID, descriptor.Kind, descriptor.Payload); err != nil {
		return LifecycleJobResult{}, true, err
	}
	if _, err := tx.Exec(ctx, `UPDATE job_steps SET status='waiting_input',worker_id=NULL,updated_at=clock_timestamp() WHERE id=$1`, stepID); err != nil {
		return LifecycleJobResult{}, true, err
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET status='waiting_input',updated_at=clock_timestamp() WHERE id=$1`, job.ID); err != nil {
		return LifecycleJobResult{}, true, err
	}
	job, err = scanLockedJobTx(ctx, tx, job.ID)
	if err != nil {
		return LifecycleJobResult{}, true, err
	}
	status := model.StepStatusWaiting
	err = insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{ID: descriptor.ID, JobID: job.ID, ObservedGeneration: job.CurrentGeneration, ResultGeneration: job.CurrentGeneration, StepID: &stepID, Kind: descriptor.Kind, ResultJobStatus: job.Status, ResultStepStatus: &status, ResultJob: job})
	return LifecycleJobResult{Job: job, Applied: true}, true, err
}

func retainCodingContinuationTx(ctx context.Context, tx pgx.Tx, job model.Job, stepID int64, ownerText string) error {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT payload_json FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'generation'=$2::bigint::text AND payload_json->>'phase'='task_interrupted' AND EXISTS(SELECT 1 FROM coding_agent_tasks task JOIN coding_ledgers ledger ON ledger.id=task.ledger_id WHERE ledger.job_id=$1 AND ledger.state='frozen' AND task.frozen_revision=ledger.frozen_revision AND task.task_item_id=evidence.payload_json->>'task_id' AND task.state='ready') ORDER BY id DESC LIMIT 1`, job.ID, job.CurrentGeneration).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var interrupted codingagent.Entry
	if err := json.Unmarshal(raw, &interrupted); err != nil {
		return err
	}
	if interrupted.Continuation == nil {
		return fmt.Errorf("interruption lacks its retained task")
	}
	err = tx.QueryRow(ctx, `SELECT payload_json FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'generation'=$2::bigint::text AND payload_json->>'task_id'=$3 AND (payload_json->>'phase'='execution_session' OR (payload_json->>'phase'='call_completed' AND payload_json->>'role' IN ('execute','review'))) ORDER BY id DESC LIMIT 1`, job.ID, job.CurrentGeneration, interrupted.TaskID).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var session codingagent.Entry
	if err := json.Unmarshal(raw, &session); err != nil {
		return err
	}
	if session.Result != nil {
		session.Session = &codingagent.ExecutionSession{ThreadID: session.Result.ThreadID, TurnID: session.Result.TurnID, State: "idle"}
	}
	if session.Session == nil || session.Session.State != "idle" || session.Session.ThreadID != interrupted.Continuation.ThreadID || session.Session.TurnID != interrupted.Continuation.TurnID {
		return nil
	}
	if session.Result != nil {
		// A completed response must be interpreted again before authorizing native continuation.
		entry := codingagent.Entry{Generation: job.CurrentGeneration, Phase: "response_recovery", TaskID: interrupted.TaskID, Role: session.Role, Result: session.Result, Notes: []string{ownerText}}
		raw, err = json.Marshal(entry)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,payload_json) VALUES($1,$2,'coding_agent',$3)`, job.ID, stepID, raw)
		return err
	}
	session.Session.State = "interrupted"
	entry := codingagent.Entry{Generation: job.CurrentGeneration, Phase: "task_resume", TaskID: interrupted.TaskID, Role: session.Role, Session: session.Session, Notes: []string{interrupted.Continuation.Reason, ownerText}}
	raw, err = json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,payload_json) VALUES($1,$2,'coding_agent',$3)`, job.ID, stepID, raw)
	return err
}
