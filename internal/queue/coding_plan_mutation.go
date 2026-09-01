package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

var (
	ErrCodingPlanRevision = errors.New("coding plan revision conflict")
	ErrCodingPlanState    = errors.New("coding plan state conflict")
)

func (r *Repository) ApplyCodingPlanDecisions(
	ctx context.Context,
	command ApplyCodingPlanDecisionsCommand,
) (CodingPlanMutationResult, error) {
	command, err := normalizeApplyCodingPlanDecisionsCommand(command)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	descriptor, err := describeLifecycleOperation(
		command.OperationID, LifecycleCodingPlanDecisions, command,
	)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return CodingPlanMutationResult{}, err
	}
	job, err := lockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := requireCodingPlanWorkspaceAuthority(
		job, command.WorkspaceRoot, command.WorkspaceIdentity,
	); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if existing, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, command.JobID); err != nil {
		return CodingPlanMutationResult{}, err
	} else if found {
		plan, err := readCodingPlanOperationResultTx(ctx, tx, existing)
		if err != nil {
			return CodingPlanMutationResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CodingPlanMutationResult{}, err
		}
		return CodingPlanMutationResult{Plan: plan, Job: existing.ResultJob}, nil
	}
	if job.CurrentGeneration != command.Generation || job.Status != model.JobStatusWaiting {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: job %d is not waiting on generation %d plan review",
			ErrCodingPlanState, job.ID, command.Generation,
		)
	}
	planStepID, revision, err := lockReviewCodingPlanTx(ctx, tx, command.JobID, command.Generation)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if revision != command.Revision {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: coding plan revision is %d, received %d",
			ErrCodingPlanRevision, revision, command.Revision,
		)
	}
	changed := false
	for _, change := range command.Decisions {
		var current model.CodingPlanDecision
		err := tx.QueryRow(ctx, `
			SELECT decision FROM coding_plan_leaves
			WHERE job_id=$1 AND generation=$2 AND leaf_id=$3
			FOR UPDATE
		`, command.JobID, command.Generation, change.LeafID).Scan(&current)
		if err != nil {
			return CodingPlanMutationResult{}, fmt.Errorf(
				"coding plan decision leaf %q: %w", change.LeafID, err,
			)
		}
		if current == change.Decision {
			continue
		}
		result, err := tx.Exec(ctx, `
			UPDATE coding_plan_leaves
			SET decision=$4,decision_origin_generation=$2,updated_at=clock_timestamp()
			WHERE job_id=$1 AND generation=$2 AND leaf_id=$3 AND decision=$5
		`, command.JobID, command.Generation, change.LeafID, change.Decision, current)
		if err != nil {
			return CodingPlanMutationResult{}, err
		}
		if result.RowsAffected() != 1 {
			return CodingPlanMutationResult{}, fmt.Errorf(
				"%w: coding plan leaf %q changed during decision update",
				ErrCodingPlanRevision, change.LeafID,
			)
		}
		changed = true
	}
	if changed {
		result, err := tx.Exec(ctx, `
			UPDATE coding_plans
			SET revision=revision+1,updated_at=clock_timestamp()
			WHERE job_id=$1 AND generation=$2 AND revision=$3 AND state=$4
		`, command.JobID, command.Generation, command.Revision, model.CodingPlanStateReview)
		if err != nil {
			return CodingPlanMutationResult{}, err
		}
		if result.RowsAffected() != 1 {
			return CodingPlanMutationResult{}, fmt.Errorf("%w: coding plan changed during decision update", ErrCodingPlanRevision)
		}
		if _, err := tx.Exec(ctx, `UPDATE jobs SET updated_at=clock_timestamp() WHERE id=$1`, job.ID); err != nil {
			return CodingPlanMutationResult{}, err
		}
	}
	plan, err := readCodingPlan(ctx, tx, command.JobID, command.Generation)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	job, err = scanLockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	stepStatus := model.StepStatusWaiting
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: job.ID,
		ObservedGeneration: command.Generation, ResultGeneration: command.Generation,
		StepID: &planStepID, Kind: descriptor.Kind, CommandSHA256: descriptor.SHA256,
		ResultJobStatus: job.Status, ResultStepStatus: &stepStatus, ResultJob: job,
	}); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := insertCodingPlanOperationResultTx(ctx, tx, descriptor, plan); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CodingPlanMutationResult{}, err
	}
	return CodingPlanMutationResult{Plan: plan, Job: job, Applied: changed}, nil
}

func (r *Repository) FreezeCodingPlan(
	ctx context.Context,
	command FreezeCodingPlanCommand,
) (CodingPlanMutationResult, error) {
	command, err := normalizeFreezeCodingPlanCommand(command)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	descriptor, err := describeLifecycleOperation(
		command.OperationID, LifecycleCodingPlanFreeze, command,
	)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return CodingPlanMutationResult{}, err
	}
	job, err := lockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := requireCodingPlanWorkspaceAuthority(
		job, command.WorkspaceRoot, command.WorkspaceIdentity,
	); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if existing, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, command.JobID); err != nil {
		return CodingPlanMutationResult{}, err
	} else if found {
		plan, err := readCodingPlanOperationResultTx(ctx, tx, existing)
		if err != nil {
			return CodingPlanMutationResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CodingPlanMutationResult{}, err
		}
		return CodingPlanMutationResult{Plan: plan, Job: existing.ResultJob}, nil
	}
	if job.CurrentGeneration != command.Generation || job.Status != model.JobStatusWaiting {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: job %d is not waiting on generation %d plan review",
			ErrCodingPlanState, job.ID, command.Generation,
		)
	}
	planStepID, revision, err := lockReviewCodingPlanTx(ctx, tx, command.JobID, command.Generation)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if revision != command.Revision {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: coding plan revision is %d, received %d",
			ErrCodingPlanRevision, revision, command.Revision,
		)
	}
	var pending, approved int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE decision=$3),
		       COUNT(*) FILTER (WHERE decision=$4)
		FROM coding_plan_leaves
		WHERE job_id=$1 AND generation=$2
	`, command.JobID, command.Generation,
		model.CodingPlanDecisionPending, model.CodingPlanDecisionApproved).Scan(&pending, &approved); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if pending != 0 {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: coding plan has %d undecided leaves", ErrCodingPlanState, pending,
		)
	}
	if approved == 0 {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: coding plan requires at least one approved leaf", ErrCodingPlanState,
		)
	}
	result, err := tx.Exec(ctx, `
		UPDATE coding_plans
		SET revision=revision+1,state=$4,updated_at=clock_timestamp(),frozen_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND revision=$3 AND state=$5
	`, command.JobID, command.Generation, command.Revision,
		model.CodingPlanStateFrozen, model.CodingPlanStateReview)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if result.RowsAffected() != 1 {
		return CodingPlanMutationResult{}, fmt.Errorf("%w: coding plan changed during freeze", ErrCodingPlanRevision)
	}
	result, err = tx.Exec(ctx, `
		UPDATE job_steps
		SET status=$4,output=$5,finished_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1 AND job_id=$2 AND generation=$3 AND status=$6
		  AND superseded_at_generation IS NULL AND action='v3_coding_plan'
	`, planStepID, command.JobID, command.Generation, model.StepStatusCompleted,
		"coding plan approved and frozen", model.StepStatusWaiting)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if result.RowsAffected() != 1 {
		return CodingPlanMutationResult{}, fmt.Errorf("%w: coding plan review step changed during freeze", ErrCodingPlanState)
	}
	result, err = tx.Exec(ctx, `
		UPDATE jobs
		SET status=$3,result=NULL,error=NULL,completed_at=NULL,updated_at=clock_timestamp()
		WHERE id=$1 AND current_generation=$2 AND status=$4
	`, command.JobID, command.Generation, model.JobStatusRunning, model.JobStatusWaiting)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if result.RowsAffected() != 1 {
		return CodingPlanMutationResult{}, fmt.Errorf("%w: coding plan job changed during freeze", ErrCodingPlanState)
	}
	plan, err := readCodingPlan(ctx, tx, command.JobID, command.Generation)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	job, err = scanLockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	stepStatus := model.StepStatusCompleted
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: job.ID,
		ObservedGeneration: command.Generation, ResultGeneration: command.Generation,
		StepID: &planStepID, Kind: descriptor.Kind, CommandSHA256: descriptor.SHA256,
		ResultJobStatus: job.Status, ResultStepStatus: &stepStatus, ResultJob: job,
	}); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := insertCodingPlanOperationResultTx(ctx, tx, descriptor, plan); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CodingPlanMutationResult{}, err
	}
	return CodingPlanMutationResult{Plan: plan, Job: job, Applied: true}, nil
}

func lockReviewCodingPlanTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
) (int64, int64, error) {
	var stepID, revision int64
	var state model.CodingPlanState
	err := tx.QueryRow(ctx, `
		SELECT plan_step_id,revision,state
		FROM coding_plans
		WHERE job_id=$1 AND generation=$2
		FOR UPDATE
	`, jobID, generation).Scan(&stepID, &revision, &state)
	if err != nil {
		return 0, 0, err
	}
	if state != model.CodingPlanStateReview {
		return 0, 0, fmt.Errorf("%w: coding plan state is %q", ErrCodingPlanState, state)
	}
	var action, status string
	if err := tx.QueryRow(ctx, `
		SELECT action,status FROM job_steps
		WHERE id=$1 AND job_id=$2 AND generation=$3 AND superseded_at_generation IS NULL
		FOR UPDATE
	`, stepID, jobID, generation).Scan(&action, &status); err != nil {
		return 0, 0, err
	}
	if action != "v3_coding_plan" || status != model.StepStatusWaiting {
		return 0, 0, fmt.Errorf(
			"%w: coding plan step is %s/%s, expected v3_coding_plan/waiting_input",
			ErrCodingPlanState, action, status,
		)
	}
	return stepID, revision, nil
}
