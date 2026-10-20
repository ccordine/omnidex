package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// An amendment is durable intake, never an executable revision. The original
// message retains its owner/session/generation and frozen-ledger binding.
func queueCodingPlanningAmendmentTx(ctx context.Context, tx pgx.Tx, message ChannelSessionMessage, authority model.StepAttemptAuthority) (bool, error) {
	binding := message.PlanningBinding
	var valid bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM coding_ledgers WHERE id=$1 AND job_id=$2
  AND state='frozen' AND frozen_revision=$3)`, binding.LedgerID, authority.JobID, binding.Revision).Scan(&valid); err != nil {
		return false, err
	}
	if !valid {
		return false, fmt.Errorf("%w: amendment frozen revision changed", ErrCodingLedgerRevision)
	}
	result, err := tx.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
 SELECT $1,$2,'coding_planning_amendment','lifecycle_operation',$3,jsonb_build_object('ledger_id',$4::bigint,'revision',$5::bigint)
 WHERE NOT EXISTS(SELECT 1 FROM evidence WHERE kind='coding_planning_amendment' AND source_ref=$3)`, authority.JobID, authority.StepID, message.OperationID, binding.LedgerID, binding.Revision)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE ai_channels SET updated_at=clock_timestamp() WHERE id=$1`, message.Command.ChannelID)
	return result.RowsAffected() == 1, err
}

func requireCodingPlanningAmendmentTransitionTx(ctx context.Context, tx pgx.Tx, message ChannelSessionMessage, authority model.StepAttemptAuthority) error {
	var valid bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM coding_ledgers ledger
 JOIN job_steps step ON step.job_id=ledger.job_id AND step.id=$1 AND step.generation=$2
 WHERE ledger.id=$3 AND ledger.job_id=$4 AND ledger.state IN ('revising','proposed')
 AND step.action='v3_coding_plan' AND $2>$5 AND ledger.current_revision>=$6)`, authority.StepID, authority.Generation, message.PlanningBinding.LedgerID, authority.JobID, message.AdmittedJob.CurrentGeneration, message.PlanningBinding.Revision).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("%w: queued amendment requires the same ledger's planning transition", ErrCodingLedgerState)
	}
	return nil
}

// ReplanPendingCodingAmendments is called at a task boundary, after its execution
// and independent review have been accepted. Replan preserves that acceptance;
// the resulting proposal still requires the ordinary owner sign-off.
func (r *Repository) ReplanPendingCodingAmendments(ctx context.Context, authority model.StepAttemptAuthority) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return false, err
	}
	var execution bool
	if err := tx.QueryRow(ctx, `SELECT action='v3_coding' FROM job_steps WHERE id=$1`, authority.StepID).Scan(&execution); err != nil {
		return false, err
	}
	if !execution {
		return false, fmt.Errorf("amendment transition requires the execution step")
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return false, staleStepAttemptError(authority, "amendment transition requires running execution", nil)
	}
	var id model.LifecycleOperationID
	err = tx.QueryRow(ctx, `SELECT intake.operation_id FROM lifecycle_operation_registry intake
 WHERE intake.kind='channel_session_message' AND intake.message_job_id=$1 AND intake.message_generation<=$2
 AND EXISTS(SELECT 1 FROM evidence WHERE kind='coding_planning_amendment' AND source_ref=intake.operation_id)
 AND NOT EXISTS(SELECT 1 FROM evidence WHERE kind='channel_session_message' AND source_ref=intake.operation_id)
 ORDER BY intake.created_at,intake.operation_id LIMIT 1`, authority.JobID, authority.Generation).Scan(&id)
	if err == pgx.ErrNoRows {
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	message, _, err := readChannelSessionMessageTx(ctx, tx, id)
	if err != nil {
		return false, err
	}
	channel, err := lockChannelTurnAuthorityTx(ctx, tx, message.Command.ChannelID)
	if err != nil {
		return false, err
	}
	if err = validateAssistantSessionAuthority(message.Command, channel); err != nil {
		return false, err
	}
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT current_revision FROM coding_ledgers WHERE id=$1 AND job_id=$2 AND state='frozen' FOR UPDATE`, message.PlanningBinding.LedgerID, authority.JobID).Scan(&revision); err != nil {
		return false, err
	}
	command := ReplanJobCommand{OperationID: message.RedirectOperationID, JobID: authority.JobID, ObservedLedgerRevision: revision, Feedback: message.Command.Text, WorkspaceRoot: message.Command.WorkspaceRoot, WorkspaceIdentity: message.Command.WorkspaceIdentity}
	descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleReplanJob, command)
	if err != nil {
		return false, err
	}
	if _, err = replanJobTx(ctx, tx, command, descriptor); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func pendingCodingPlanningAmendmentsTx(ctx context.Context, tx pgx.Tx, jobID int64) ([]CodingPlanningAmendment, error) {
	var amendments []CodingPlanningAmendment
	rows, err := tx.Query(ctx, `SELECT intake.operation_id,(intake.command_payload->'planning_binding'->>'revision')::bigint,intake.command_payload->'command'->>'text'
 FROM lifecycle_operation_registry intake WHERE intake.kind='channel_session_message' AND intake.message_job_id=$1
 AND intake.command_payload->'planning_binding' IS NOT NULL
 AND EXISTS(SELECT 1 FROM evidence WHERE kind='coding_planning_amendment' AND source_ref=intake.operation_id)
 AND NOT EXISTS(SELECT 1 FROM evidence WHERE kind='channel_session_message' AND source_ref=intake.operation_id)
 ORDER BY intake.created_at,intake.operation_id`, jobID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var pending CodingPlanningAmendment
		if err := rows.Scan(&pending.OperationID, &pending.Revision, &pending.Text); err != nil {
			rows.Close()
			return nil, err
		}
		amendments = append(amendments, pending)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}

	return amendments, nil
}
