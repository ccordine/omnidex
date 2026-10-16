package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func codingExecutionResponseTx(ctx context.Context, tx pgx.Tx, jobID, stepID, generation int64) (codingagent.Entry, error) {
	var raw []byte
	var id int64
	if err := tx.QueryRow(ctx, `SELECT id,payload_json FROM evidence
		WHERE job_id=$1 AND step_id=$2 AND kind='coding_agent'
		  AND payload_json->>'generation'=$3::bigint::text AND payload_json->>'phase' IN ('execution_response','review_response','execution_native_input','task_incomplete','planning_incomplete')
		ORDER BY id DESC LIMIT 1`, jobID, stepID, generation).Scan(&id, &raw); err != nil {
		return codingagent.Entry{}, err
	}
	var response codingagent.Entry
	if err := json.Unmarshal(raw, &response); err != nil {
		return response, err
	}
	response.ID = id
	if response.TaskID == "" && response.NativeInput == nil && response.Phase != "planning_incomplete" || response.CallID < 1 || len(response.Notes) != 1 ||
		(response.NativeInput == nil && response.ResponseKind != codingagent.ExecutionQuestion && response.ResponseKind != codingagent.ExecutionPermission) {
		return response, fmt.Errorf("execution is not awaiting a parsed question or permission request")
	}
	return response, nil
}

// WaitForCodingExecutionInput retains the frozen task and ordinary response.
// Only a separately parsed request for owner input enters this boundary.
func (r *Repository) WaitForCodingExecutionInput(ctx context.Context, authority model.StepAttemptAuthority, taskID string) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return staleStepAttemptError(authority, "execution input requires the running step", nil)
	}
	response, err := codingExecutionResponseTx(ctx, tx, authority.JobID, authority.StepID, authority.Generation)
	if err != nil {
		return err
	}
	if response.TaskID != taskID {
		return fmt.Errorf("execution input changed its selected task")
	}
	if err := terminalizeStepAttemptTx(ctx, tx, authority, model.StepAttemptWaiting); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE job_steps SET status='waiting_input',worker_id=NULL,output=$5,updated_at=clock_timestamp()
		WHERE id=$1 AND job_id=$2 AND generation=$3 AND current_attempt=$4 AND action IN ('v3_coding','v3_coding_plan') AND status='running'`,
		authority.StepID, authority.JobID, authority.Generation, authority.Attempt, response.Notes[0])
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("execution step changed before waiting for input")
	}
	result, err = tx.Exec(ctx, `UPDATE jobs SET status='waiting_input',updated_at=clock_timestamp()
		WHERE id=$1 AND current_generation=$2 AND status='running'`, authority.JobID, authority.Generation)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("execution job changed before waiting for input")
	}
	return tx.Commit(ctx)
}

func resumeCodingExecutionTx(ctx context.Context, tx pgx.Tx, command SubmitJobFeedbackCommand, job model.Job) (model.Job, int64, error) {
	if command.ExpectedGeneration != nil && *command.ExpectedGeneration != job.CurrentGeneration {
		return model.Job{}, 0, fmt.Errorf("%w: observed %d, current %d", ErrJobFeedbackGeneration, *command.ExpectedGeneration, job.CurrentGeneration)
	}
	if job.Status != model.JobStatusWaiting {
		return model.Job{}, 0, fmt.Errorf("%w: execution reply requires a waiting job", ErrStepNotWritable)
	}
	var stepID, attempt int64
	if err := tx.QueryRow(ctx, `SELECT id,current_attempt FROM job_steps
		WHERE job_id=$1 AND generation=$2 AND action IN ('v3_coding','v3_coding_plan')
		  AND status='waiting_input' AND superseded_at_generation IS NULL FOR UPDATE`,
		job.ID, job.CurrentGeneration).Scan(&stepID, &attempt); err != nil {
		return model.Job{}, 0, fmt.Errorf("%w: no waiting execution step: %v", ErrStepNotWritable, err)
	}
	if command.ExpectedWaitingAttempt != nil && *command.ExpectedWaitingAttempt != attempt {
		return model.Job{}, 0, ErrJobFeedbackWaitingAttempt
	}
	response, err := codingExecutionResponseTx(ctx, tx, job.ID, stepID, job.CurrentGeneration)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Job{}, 0, fmt.Errorf("%w: planning discussion requires the planning conversation, not an execution reply", ErrStepNotWritable)
		}
		return model.Job{}, 0, err
	}
	entry := codingagent.Entry{Generation: job.CurrentGeneration, Phase: "execution_reply", TaskID: response.TaskID,
		CallID: response.CallID, Role: response.Role, Notes: []string{string(command.OperationID), command.Feedback}}
	if response.NativeInput != nil {
		if command.ExpectedInputID == nil || *command.ExpectedInputID != response.ID {
			return model.Job{}, 0, fmt.Errorf("%w: native reply differs from its observed input request", ErrStepNotWritable)
		}
		entry.NativeReply, err = response.NativeInput.Reply(command.Feedback)
		if err != nil {
			return model.Job{}, 0, fmt.Errorf("%w: %v", ErrStepNotWritable, err)
		}
		entry.InputID = response.ID
		entry.NativeInput = response.NativeInput
	} else if command.ExpectedInputID != nil {
		return model.Job{}, 0, fmt.Errorf("%w: native input request is no longer waiting", ErrStepNotWritable)
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return model.Job{}, 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,payload_json) VALUES($1,$2,'coding_agent',$3::jsonb)`,
		job.ID, stepID, string(raw)); err != nil {
		return model.Job{}, 0, err
	}
	if response.Phase == "task_incomplete" || response.Phase == "planning_incomplete" {
		replanned, err := replanIncompleteCodingTaskTx(ctx, tx, command, job, response)
		return replanned, stepID, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ai_channel_messages(channel_id,role,content)
		SELECT metadata->>'channel_id','user',$2 FROM jobs
		WHERE id=$1 AND pipeline='chat' AND metadata->>'channel_mode'='assistant'`, job.ID, command.Feedback); err != nil {
		return model.Job{}, 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE job_steps SET status='running',output=NULL,
		worker_id=(SELECT worker_id FROM job_step_attempts WHERE step_id=$1 AND attempt=$2),
		updated_at=clock_timestamp() WHERE id=$1`, stepID, attempt); err != nil {
		return model.Job{}, 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET status='running',updated_at=clock_timestamp() WHERE id=$1`, job.ID); err != nil {
		return model.Job{}, 0, err
	}
	job, err = scanJob(tx.QueryRow(ctx, `SELECT id,instruction,pipeline,status,result,error,metadata,current_generation,
		created_at,updated_at,completed_at FROM jobs WHERE id=$1`, job.ID))
	return job, stepID, err
}

func requireCodingExecutionReplyTx(ctx context.Context, tx pgx.Tx, jobID, stepID, generation int64, operationID model.LifecycleOperationID, text string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evidence
		WHERE job_id=$1 AND step_id=$2 AND kind='coding_agent' AND payload_json->>'generation'=$3::bigint::text
		  AND payload_json->>'phase'='execution_reply' AND payload_json->'notes'->>0=$4 AND payload_json->'notes'->>1=$5)`,
		jobID, stepID, generation, operationID, text).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("execution reply lacks its exact persisted response")
	}
	return nil
}
