package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type codingPlanQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (r *Repository) CurrentCodingPlan(ctx context.Context, jobID int64) (model.CodingPlan, error) {
	if r == nil || r.pool == nil {
		return model.CodingPlan{}, fmt.Errorf("coding plan repository is unavailable")
	}
	if jobID <= 0 {
		return model.CodingPlan{}, fmt.Errorf("coding plan requires a positive job ID")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return model.CodingPlan{}, fmt.Errorf("begin coding plan snapshot: %w", err)
	}
	defer tx.Rollback(ctx)
	var generation int64
	if err := tx.QueryRow(ctx, `
		SELECT current_generation FROM jobs WHERE id=$1
	`, jobID).Scan(&generation); err != nil {
		return model.CodingPlan{}, err
	}
	plan, err := readCodingPlan(ctx, tx, jobID, generation)
	if err != nil {
		return model.CodingPlan{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.CodingPlan{}, fmt.Errorf("commit coding plan snapshot: %w", err)
	}
	return plan, nil
}

func (r *Repository) CurrentCodingPlanForWorkspace(
	ctx context.Context,
	jobID int64,
	workspaceRoot string,
	workspaceIdentity string,
) (model.CodingPlan, error) {
	if r == nil || r.pool == nil {
		return model.CodingPlan{}, fmt.Errorf("coding plan repository is unavailable")
	}
	if jobID <= 0 {
		return model.CodingPlan{}, fmt.Errorf("coding plan requires a positive job ID")
	}
	if err := validateRequiredLifecycleWorkspaceBinding(workspaceRoot, workspaceIdentity); err != nil {
		return model.CodingPlan{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return model.CodingPlan{}, fmt.Errorf("begin coding plan workspace snapshot: %w", err)
	}
	defer tx.Rollback(ctx)
	job, err := scanLockedJobTx(ctx, tx, jobID)
	if err != nil {
		return model.CodingPlan{}, err
	}
	if err := requireCodingPlanWorkspaceAuthority(job, workspaceRoot, workspaceIdentity); err != nil {
		return model.CodingPlan{}, err
	}
	plan, err := readCodingPlan(ctx, tx, jobID, job.CurrentGeneration)
	if err != nil {
		return model.CodingPlan{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.CodingPlan{}, fmt.Errorf("commit coding plan workspace snapshot: %w", err)
	}
	return plan, nil
}

func readCodingPlan(
	ctx context.Context,
	query codingPlanQuerier,
	jobID, generation int64,
) (model.CodingPlan, error) {
	var plan model.CodingPlan
	var rawLeaves []byte
	err := query.QueryRow(ctx, `
		SELECT plan.job_id,plan.generation,plan.revision,plan.state,
		       plan.scope_mode,plan.request_sha256,plan.created_at,
		       plan.updated_at,plan.frozen_at,
		       COALESCE(
		         jsonb_agg(
		           jsonb_build_object(
		             'id',leaf.leaf_id,
		             'statement',leaf.statement,
		             'annotation',leaf.annotation,
		             'decision',leaf.decision
		           ) ORDER BY leaf.sort_index
		         ) FILTER (WHERE leaf.leaf_id IS NOT NULL),
		         '[]'::jsonb
		       )
		FROM coding_plans AS plan
		LEFT JOIN coding_plan_leaves AS leaf
		  ON leaf.job_id=plan.job_id AND leaf.generation=plan.generation
		WHERE plan.job_id=$1 AND plan.generation=$2
		GROUP BY plan.job_id,plan.generation,plan.revision,plan.state,
		         plan.scope_mode,plan.request_sha256,plan.created_at,
		         plan.updated_at,plan.frozen_at
	`, jobID, generation).Scan(
		&plan.JobID, &plan.Generation, &plan.Revision, &plan.State, &plan.ScopeMode,
		&plan.RequestSHA256, &plan.CreatedAt, &plan.UpdatedAt, &plan.FrozenAt,
		&rawLeaves,
	)
	if err != nil {
		return model.CodingPlan{}, err
	}
	if err := json.Unmarshal(rawLeaves, &plan.Leaves); err != nil {
		return model.CodingPlan{}, fmt.Errorf("decode persisted coding plan leaves: %w", err)
	}
	if plan.Leaves == nil {
		return model.CodingPlan{}, fmt.Errorf("persisted coding plan leaves decoded to null")
	}
	if err := plan.Validate(); err != nil {
		return model.CodingPlan{}, fmt.Errorf("persisted coding plan: %w", err)
	}
	return plan, nil
}

func readCodingPlanOperationResultTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
) (model.CodingPlan, error) {
	var raw []byte
	var jobID, generation, revision int64
	var kind LifecycleOperationKind
	err := tx.QueryRow(ctx, `
		SELECT job_id,generation,kind,result_revision,result_plan
		FROM coding_plan_operation_results
		WHERE operation_id=$1
	`, record.ID).Scan(&jobID, &generation, &kind, &revision, &raw)
	if err != nil {
		return model.CodingPlan{}, fmt.Errorf("read coding plan operation %q result: %w", record.ID, err)
	}
	if jobID != record.JobID || generation != record.ResultGeneration || kind != record.Kind {
		return model.CodingPlan{}, lifecycleReplayStateError(record.ID, "coding plan operation result")
	}
	var plan model.CodingPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return model.CodingPlan{}, fmt.Errorf("decode coding plan operation %q result: %w", record.ID, err)
	}
	if plan.JobID != jobID || plan.Generation != generation || plan.Revision != revision {
		return model.CodingPlan{}, lifecycleReplayStateError(record.ID, "coding plan result projection")
	}
	if err := plan.Validate(); err != nil {
		return model.CodingPlan{}, lifecycleReplayStateError(record.ID, "invalid coding plan result projection")
	}
	return plan, nil
}

func insertCodingPlanOperationResultTx(
	ctx context.Context,
	tx pgx.Tx,
	descriptor lifecycleOperationDescriptor,
	plan model.CodingPlan,
) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("encode coding plan operation result: %w", err)
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO coding_plan_operation_results (
			operation_id,job_id,generation,kind,result_revision,result_plan
		) VALUES ($1,$2,$3,$4,$5,$6::jsonb)
		ON CONFLICT (operation_id) DO NOTHING
	`, descriptor.ID, plan.JobID, plan.Generation, descriptor.Kind, plan.Revision, string(raw))
	if err != nil {
		return fmt.Errorf("record coding plan operation %q result: %w", descriptor.ID, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("coding plan operation %q result already exists", descriptor.ID)
	}
	return nil
}

type PriorCodingPlanDecision struct {
	Decision         model.CodingPlanDecision
	OriginGeneration int64
}

func (r *Repository) PriorCodingPlanDecisions(
	ctx context.Context,
	jobID, beforeGeneration int64,
) (map[model.CodingPlanLeafID]PriorCodingPlanDecision, error) {
	if r == nil || r.pool == nil || jobID <= 0 || beforeGeneration <= 1 {
		return map[model.CodingPlanLeafID]PriorCodingPlanDecision{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (leaf.leaf_id)
		       leaf.leaf_id,leaf.decision,leaf.decision_origin_generation
		FROM coding_plan_leaves AS leaf
		JOIN coding_plans AS plan
		  ON plan.job_id=leaf.job_id AND plan.generation=leaf.generation
		WHERE leaf.job_id=$1 AND leaf.generation<$2
		  AND leaf.decision IN ($3,$4)
		ORDER BY leaf.leaf_id,leaf.generation DESC
	`, jobID, beforeGeneration,
		model.CodingPlanDecisionApproved, model.CodingPlanDecisionRejected)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[model.CodingPlanLeafID]PriorCodingPlanDecision)
	for rows.Next() {
		var id model.CodingPlanLeafID
		var decision PriorCodingPlanDecision
		if err := rows.Scan(&id, &decision.Decision, &decision.OriginGeneration); err != nil {
			return nil, err
		}
		if _, err := model.ParseCodingPlanLeafID(string(id)); err != nil {
			return nil, err
		}
		if err := decision.Decision.Validate(); err != nil {
			return nil, err
		}
		result[id] = decision
	}
	return result, rows.Err()
}
