package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) StoreCodingPlanReview(
	ctx context.Context,
	command StoreCodingPlanReviewCommand,
) (model.CodingPlan, error) {
	command, err := normalizeStoreCodingPlanReviewCommand(command)
	if err != nil {
		return model.CodingPlan{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.CodingPlan{}, err
	}
	defer tx.Rollback(ctx)

	locked, err := lockCodingPlanStoreAuthorityTx(ctx, tx, command.Authority)
	if err != nil {
		return model.CodingPlan{}, err
	}
	job, err := scanLockedJobTx(ctx, tx, command.Authority.JobID)
	if err != nil {
		return model.CodingPlan{}, err
	}
	var action string
	if err := tx.QueryRow(ctx, `
		SELECT action FROM job_steps WHERE id=$1 AND job_id=$2
	`, command.Authority.StepID, command.Authority.JobID).Scan(&action); err != nil {
		return model.CodingPlan{}, err
	}
	if action != "v3_coding_plan" {
		return model.CodingPlan{}, fmt.Errorf(
			"coding plan review requires the v3_coding_plan step, received %q", action,
		)
	}
	storedMode, err := codingScopeModeFromJob(job)
	if err != nil {
		return model.CodingPlan{}, err
	}
	if storedMode != command.ScopeMode {
		return model.CodingPlan{}, fmt.Errorf("coding plan scope mode differs from immutable job authority")
	}
	if err := requireCarriedCodingPlanDecisionsTx(
		ctx, tx, job.ID, command.Authority.Generation, command.Leaves,
	); err != nil {
		return model.CodingPlan{}, err
	}

	if locked.AttemptStatus == model.StepAttemptCompleted &&
		locked.StepStatus == model.StepStatusWaiting && job.Status == model.JobStatusWaiting {
		plan, err := readCodingPlan(ctx, tx, job.ID, command.Authority.Generation)
		if err != nil {
			return model.CodingPlan{}, err
		}
		if err := requireStoredCodingPlanCommandTx(ctx, tx, plan, command); err != nil {
			return model.CodingPlan{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return model.CodingPlan{}, err
		}
		return plan, nil
	}
	if err := requireLockedStepAttemptActiveTx(ctx, tx, command.Authority, locked); err != nil {
		return model.CodingPlan{}, err
	}
	if locked.StepStatus != model.StepStatusRunning || job.Status != model.JobStatusRunning {
		return model.CodingPlan{}, staleStepAttemptError(
			command.Authority,
			fmt.Sprintf("coding plan writer job status %q step status %q", job.Status, locked.StepStatus),
			nil,
		)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO coding_plans (
			job_id,generation,revision,state,scope_mode,request_sha256,plan_step_id
		) VALUES ($1,$2,1,$3,$4,$5,$6)
	`, job.ID, command.Authority.Generation, model.CodingPlanStateReview,
		command.ScopeMode, command.RequestSHA256, command.Authority.StepID); err != nil {
		return model.CodingPlan{}, fmt.Errorf("store coding plan review: %w", err)
	}
	for index, write := range command.Leaves {
		if _, err := tx.Exec(ctx, `
			INSERT INTO coding_plan_leaves (
				job_id,generation,leaf_id,sort_index,statement,annotation,decision,
				decision_origin_generation,result_schema,candidate_sha256,
				kind_receipt_sha256,cardinality_receipt_sha256,result_relation
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		`, job.ID, command.Authority.Generation, write.Leaf.ID, index,
			write.Leaf.Statement, write.Leaf.Annotation, write.Leaf.Decision,
			write.DecisionOriginGeneration, write.ResultRelation.Schema,
			write.ResultRelation.CandidateSHA256, write.ResultRelation.KindReceiptSHA256,
			write.ResultRelation.CardinalityReceiptSHA256, write.ResultRelation.Relation); err != nil {
			return model.CodingPlan{}, fmt.Errorf("store coding plan leaf %d: %w", index, err)
		}
	}
	if err := terminalizeStepAttemptTx(
		ctx, tx, command.Authority, model.StepAttemptCompleted,
	); err != nil {
		return model.CodingPlan{}, err
	}
	stepResult, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status=$2,worker_id=NULL,output=NULL,error=NULL,finished_at=NULL,updated_at=NOW()
		WHERE id=$1 AND job_id=$3 AND generation=$4 AND current_attempt=$5
		  AND worker_id=$6 AND status=$7
	`, command.Authority.StepID, model.StepStatusWaiting, job.ID,
		command.Authority.Generation, command.Authority.Attempt,
		command.Authority.WorkerID, model.StepStatusRunning)
	if err != nil {
		return model.CodingPlan{}, err
	}
	if stepResult.RowsAffected() != 1 {
		return model.CodingPlan{}, staleStepAttemptError(command.Authority, "coding plan step lost waiting transition authority", nil)
	}
	jobResult, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status=$2,result=NULL,error=NULL,completed_at=NULL,updated_at=NOW()
		WHERE id=$1 AND current_generation=$3 AND status=$4
	`, job.ID, model.JobStatusWaiting, command.Authority.Generation, model.JobStatusRunning)
	if err != nil {
		return model.CodingPlan{}, err
	}
	if jobResult.RowsAffected() != 1 {
		return model.CodingPlan{}, staleStepAttemptError(command.Authority, "coding plan job lost waiting transition authority", nil)
	}
	plan, err := readCodingPlan(ctx, tx, job.ID, command.Authority.Generation)
	if err != nil {
		return model.CodingPlan{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.CodingPlan{}, err
	}
	return plan, nil
}

func requireCarriedCodingPlanDecisionsTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
	leaves []CodingPlanLeafWrite,
) error {
	for _, write := range leaves {
		var decision model.CodingPlanDecision
		var origin int64
		err := tx.QueryRow(ctx, `
			SELECT leaf.decision,leaf.decision_origin_generation
			FROM coding_plan_leaves AS leaf
			JOIN coding_plans AS plan
			  ON plan.job_id=leaf.job_id AND plan.generation=leaf.generation
			WHERE leaf.job_id=$1 AND leaf.generation<$2 AND leaf.leaf_id=$3
			  AND leaf.decision IN ($4,$5) AND plan.state=$6
			ORDER BY leaf.generation DESC
			LIMIT 1
		`, jobID, generation, write.Leaf.ID,
			model.CodingPlanDecisionApproved, model.CodingPlanDecisionRejected,
			model.CodingPlanStateSuperseded).Scan(&decision, &origin)
		if errors.Is(err, pgx.ErrNoRows) {
			if write.Leaf.Decision != model.CodingPlanDecisionPending ||
				write.DecisionOriginGeneration != generation {
				return fmt.Errorf(
					"coding plan leaf %q has no exact prior user decision to carry",
					write.Leaf.ID,
				)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("load prior decision for coding plan leaf %q: %w", write.Leaf.ID, err)
		}
		if decision != write.Leaf.Decision || origin != write.DecisionOriginGeneration {
			return fmt.Errorf(
				"coding plan leaf %q differs from its exact prior user decision",
				write.Leaf.ID,
			)
		}
	}
	return nil
}

func lockCodingPlanStoreAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
) (lockedStepAttempt, error) {
	if err := validateStepAttemptAuthority(authority); err != nil {
		return lockedStepAttempt{}, err
	}
	var locked lockedStepAttempt
	var currentGeneration, stepGeneration, currentAttempt int64
	var supersededAt *int64
	var stepWorker *string
	var attemptWorker string
	if err := tx.QueryRow(ctx, `
		SELECT status,current_generation FROM jobs WHERE id=$1 FOR UPDATE
	`, authority.JobID).Scan(&locked.JobStatus, &currentGeneration); err != nil {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "job authority is unavailable", err)
	}
	if currentGeneration != authority.Generation {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "job generation changed", nil)
	}
	if err := tx.QueryRow(ctx, `
		SELECT status,generation,superseded_at_generation,current_attempt,worker_id
		FROM job_steps WHERE job_id=$1 AND id=$2 FOR UPDATE
	`, authority.JobID, authority.StepID).Scan(
		&locked.StepStatus, &stepGeneration, &supersededAt, &currentAttempt, &stepWorker,
	); err != nil {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "step authority is unavailable", err)
	}
	if stepGeneration != authority.Generation || supersededAt != nil || currentAttempt != authority.Attempt {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "step authority changed", nil)
	}
	switch locked.StepStatus {
	case model.StepStatusRunning:
		if stepWorker == nil || *stepWorker != authority.WorkerID {
			return lockedStepAttempt{}, staleStepAttemptError(authority, "step worker changed", nil)
		}
	case model.StepStatusWaiting:
		if stepWorker != nil {
			return lockedStepAttempt{}, staleStepAttemptError(authority, "waiting plan retained a worker", nil)
		}
	default:
		return lockedStepAttempt{}, staleStepAttemptError(authority, "plan step is not writable", nil)
	}
	if err := tx.QueryRow(ctx, `
		SELECT status,worker_id,expires_at
		FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		FOR UPDATE
	`, authority.JobID, authority.Generation, authority.StepID, authority.Attempt).Scan(
		&locked.AttemptStatus, &attemptWorker, &locked.ExpiresAt,
	); err != nil {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "attempt authority is unavailable", err)
	}
	if attemptWorker != authority.WorkerID {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "attempt worker changed", nil)
	}
	return locked, nil
}

func requireStoredCodingPlanCommandTx(
	ctx context.Context,
	tx pgx.Tx,
	plan model.CodingPlan,
	command StoreCodingPlanReviewCommand,
) error {
	if plan.Generation != command.Authority.Generation || plan.ScopeMode != command.ScopeMode ||
		plan.RequestSHA256 != command.RequestSHA256 || len(plan.Leaves) != len(command.Leaves) {
		return fmt.Errorf("persisted coding plan differs from exact planning result")
	}
	rows, err := tx.Query(ctx, `
		SELECT leaf_id,decision_origin_generation,result_schema,candidate_sha256,
		       kind_receipt_sha256,cardinality_receipt_sha256,result_relation
		FROM coding_plan_leaves
		WHERE job_id=$1 AND generation=$2 ORDER BY sort_index
	`, plan.JobID, plan.Generation)
	if err != nil {
		return err
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		if index >= len(command.Leaves) {
			return fmt.Errorf("persisted coding plan has unexpected leaves")
		}
		var id model.CodingPlanLeafID
		var origin int64
		var schema, candidate, kind, cardinality, relation *string
		if err := rows.Scan(&id, &origin, &schema, &candidate, &kind, &cardinality, &relation); err != nil {
			return err
		}
		write := command.Leaves[index]
		if id != write.Leaf.ID || plan.Leaves[index] != write.Leaf ||
			origin != write.DecisionOriginGeneration ||
			!sameCodingPlanReceipt(write.ResultRelation, schema, candidate, kind, cardinality, relation) {
			return fmt.Errorf("persisted coding plan leaf %q differs from exact planning result", id)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if index != len(command.Leaves) {
		return fmt.Errorf("persisted coding plan leaf count differs from exact planning result")
	}
	return nil
}

func sameCodingPlanReceipt(
	want *CodingPlanResultRelationReceipt,
	schema, candidate, kind, cardinality, relation *string,
) bool {
	if want == nil {
		return schema == nil && candidate == nil && kind == nil && cardinality == nil && relation == nil
	}
	return schema != nil && candidate != nil && kind != nil && cardinality != nil && relation != nil &&
		*schema == want.Schema && *candidate == want.CandidateSHA256 &&
		*kind == want.KindReceiptSHA256 && *cardinality == want.CardinalityReceiptSHA256 &&
		*relation == want.Relation
}
