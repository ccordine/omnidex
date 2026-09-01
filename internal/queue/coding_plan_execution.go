package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) LoadFrozenCodingPlan(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) (FrozenCodingPlan, error) {
	if err := validateStepAttemptAuthority(authority); err != nil {
		return FrozenCodingPlan{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return FrozenCodingPlan{}, err
	}
	defer tx.Rollback(ctx)
	locked, err := lockStepAttemptAuthorityTx(ctx, tx, authority)
	if err != nil {
		return FrozenCodingPlan{}, err
	}
	if err := requireLockedStepAttemptActiveTx(ctx, tx, authority, locked); err != nil {
		return FrozenCodingPlan{}, err
	}
	if locked.JobStatus != model.JobStatusRunning || locked.StepStatus != model.StepStatusRunning {
		return FrozenCodingPlan{}, staleStepAttemptError(
			authority,
			fmt.Sprintf("coding execution job status %q step status %q", locked.JobStatus, locked.StepStatus),
			nil,
		)
	}
	var action string
	if err := tx.QueryRow(ctx, `
		SELECT action FROM job_steps WHERE id=$1 AND job_id=$2
	`, authority.StepID, authority.JobID).Scan(&action); err != nil {
		return FrozenCodingPlan{}, err
	}
	if action != "v3_coding" {
		return FrozenCodingPlan{}, fmt.Errorf("frozen coding plan requires v3_coding execution, received %q", action)
	}
	var planStepStatus string
	if err := tx.QueryRow(ctx, `
		SELECT status FROM job_steps
		WHERE job_id=$1 AND generation=$2 AND action='v3_coding_plan'
		  AND superseded_at_generation IS NULL
	`, authority.JobID, authority.Generation).Scan(&planStepStatus); err != nil {
		return FrozenCodingPlan{}, err
	}
	if planStepStatus != model.StepStatusCompleted {
		return FrozenCodingPlan{}, fmt.Errorf("coding execution requires one completed plan review step")
	}
	plan, err := readCodingPlan(ctx, tx, authority.JobID, authority.Generation)
	if err != nil {
		return FrozenCodingPlan{}, err
	}
	if plan.State != model.CodingPlanStateFrozen {
		return FrozenCodingPlan{}, fmt.Errorf("coding execution requires a frozen plan, received %q", plan.State)
	}
	rows, err := tx.Query(ctx, `
		SELECT leaf_id,statement,annotation,decision,
		       result_schema,candidate_sha256,kind_receipt_sha256,
		       cardinality_receipt_sha256,result_relation
		FROM coding_plan_leaves
		WHERE job_id=$1 AND generation=$2 AND decision=$3
		ORDER BY sort_index
	`, authority.JobID, authority.Generation, model.CodingPlanDecisionApproved)
	if err != nil {
		return FrozenCodingPlan{}, err
	}
	defer rows.Close()
	leaves := make([]FrozenCodingPlanLeaf, 0, len(plan.Leaves))
	for rows.Next() {
		var leaf FrozenCodingPlanLeaf
		if err := rows.Scan(
			&leaf.Leaf.ID, &leaf.Leaf.Statement, &leaf.Leaf.Annotation,
			&leaf.Leaf.Decision,
			&leaf.ResultRelation.Schema, &leaf.ResultRelation.CandidateSHA256,
			&leaf.ResultRelation.KindReceiptSHA256,
			&leaf.ResultRelation.CardinalityReceiptSHA256,
			&leaf.ResultRelation.Relation,
		); err != nil {
			return FrozenCodingPlan{}, err
		}
		if err := leaf.Leaf.Validate(); err != nil {
			return FrozenCodingPlan{}, err
		}
		if err := leaf.ResultRelation.validateFor(leaf.Leaf); err != nil {
			return FrozenCodingPlan{}, err
		}
		if err := leaf.ResultRelation.assemblyline().ValidateAcceptedFor(leaf.Leaf.Statement); err != nil {
			return FrozenCodingPlan{}, err
		}
		leaves = append(leaves, leaf)
	}
	if err := rows.Err(); err != nil {
		return FrozenCodingPlan{}, err
	}
	if len(leaves) == 0 {
		return FrozenCodingPlan{}, fmt.Errorf("frozen coding plan contains no executable approved leaves")
	}
	if err := tx.Commit(ctx); err != nil {
		return FrozenCodingPlan{}, err
	}
	return FrozenCodingPlan{Plan: plan, Leaves: leaves}, nil
}
