package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// A native tool request keeps its current attempt and process alive. A final
// prose question instead closes its turn and resumes its persisted thread later.
func (r *Repository) WaitForCodingNativeInput(ctx context.Context, authority model.StepAttemptAuthority, request codingagent.NativeInputRequest, callID int64, taskID string) (int64, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return 0, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return 0, staleStepAttemptError(authority, "native input requires the running attempt", nil)
	}
	var bound bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evidence call
		JOIN job_steps step ON step.id=call.step_id AND step.job_id=call.job_id
		WHERE call.id=$1 AND call.job_id=$2 AND call.step_id=$3 AND step.action IN ('v3_coding','v3_coding_plan')
		  AND call.kind='coding_agent' AND call.payload_json->>'generation'=$4::bigint::text
		  AND call.payload_json->>'phase'='call_opened' AND COALESCE(call.payload_json->>'task_id','')=$5
		  AND ((step.action='v3_coding' AND call.payload_json->>'role' IN ('execute','review')) OR
		       (step.action='v3_coding_plan' AND call.payload_json->>'role'='plan') OR
		       (step.action='v3_coding' AND call.payload_json->>'role'='plan' AND EXISTS (
		         SELECT 1 FROM coding_ledgers ledger JOIN coding_agent_tasks task ON task.ledger_id=ledger.id
		         WHERE ledger.job_id=step.job_id AND ledger.state='frozen' AND task.frozen_revision=ledger.frozen_revision
		           AND task.task_item_id=call.payload_json->'assignment'->'continuation'->'task'->>'id' AND task.state='ready'
		           AND ledger.frozen_revision=(call.payload_json->'assignment'->'continuation'->>'ledger_revision')::bigint))))`, callID, authority.JobID, authority.StepID, authority.Generation, taskID).Scan(&bound); err != nil {
		return 0, err
	}
	if !bound || request.ThreadID == "" || request.Method == "" || !json.Valid(request.ID) {
		return 0, fmt.Errorf("native request lacks its exact active task and call")
	}
	entry := codingagent.Entry{Generation: authority.Generation, Phase: "execution_native_input", TaskID: taskID,
		CallID: callID, NativeInput: &request, Notes: []string{request.Prompt}}
	raw, err := json.Marshal(entry)
	if err != nil || len(raw) > llm.MaxExactPreparedProviderResponseBytes {
		return 0, fmt.Errorf("native input exceeds its typed transport boundary")
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO evidence(job_id,step_id,kind,payload_json)
		VALUES($1,$2,'coding_agent',$3::jsonb) RETURNING id`, authority.JobID, authority.StepID, string(raw)).Scan(&id); err != nil {
		return 0, err
	}
	if err := appendCodingAssistantMessagesTx(ctx, tx, authority.JobID, request.Prompt); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE job_steps SET status='waiting_input',output=$2,updated_at=clock_timestamp() WHERE id=$1`, authority.StepID, request.Prompt); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET status='waiting_input',updated_at=clock_timestamp() WHERE id=$1`, authority.JobID); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func (r *Repository) CodingNativeInputReply(ctx context.Context, authority model.StepAttemptAuthority, inputID int64) (json.RawMessage, bool, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT payload_json->'native_reply' FROM evidence
		WHERE job_id=$1 AND step_id=$2 AND kind='coding_agent' AND payload_json->>'generation'=$3::bigint::text
		  AND payload_json->>'phase'='execution_reply' AND payload_json->>'input_id'=$4::bigint::text
		ORDER BY id LIMIT 1`, authority.JobID, authority.StepID, authority.Generation, inputID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	return json.RawMessage(raw), err == nil, err
}

// This is the only waiting state that retains an active native execution lease.
func codingNativeInputWaitingTx(ctx context.Context, tx pgx.Tx, authority model.StepAttemptAuthority) (bool, error) {
	var waiting bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evidence request
		JOIN job_steps step ON step.id=request.step_id AND step.job_id=request.job_id
		WHERE request.job_id=$1 AND request.step_id=$2 AND step.action IN ('v3_coding','v3_coding_plan')
		  AND request.kind='coding_agent' AND request.payload_json->>'generation'=$3::bigint::text
		  AND request.payload_json->>'phase'='execution_native_input'
		  AND NOT EXISTS(SELECT 1 FROM evidence later WHERE later.job_id=request.job_id
		    AND later.step_id=request.step_id AND later.id>request.id AND later.kind='coding_agent'
		    AND later.payload_json->>'phase' IN ('execution_reply','execution_native_input','execution_response','review_response','task_incomplete')))
	`, authority.JobID, authority.StepID, authority.Generation).Scan(&waiting)
	return waiting, err
}
