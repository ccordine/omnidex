package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func freezeCodingAgentWorkloadTx(ctx context.Context, tx pgx.Tx, jobID, generation, planStepID int64) error {
	workload, proposal, err := proposedCodingAgentWorkloadTx(ctx, tx, jobID, generation, planStepID)
	if err != nil {
		return err
	}
	if proposal.CallID <= 0 {
		return fmt.Errorf("approved agent workload lacks its native planning call")
	}
	rows, err := tx.Query(ctx, `SELECT statement,decision FROM coding_plan_leaves WHERE job_id=$1 AND generation=$2 ORDER BY sort_index`, jobID, generation)
	if err != nil {
		return err
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		if index >= len(workload.Tasks) {
			return fmt.Errorf("approved agent workload has fewer tasks than plan leaves")
		}
		var statement string
		var decision model.CodingPlanDecision
		if err := rows.Scan(&statement, &decision); err != nil {
			return err
		}
		want, err := codingagent.PlanTaskStatement(workload.Tasks[index])
		if err != nil {
			return err
		}
		if statement != want || decision != model.CodingPlanDecisionApproved {
			return fmt.Errorf("approved agent plan leaf %d differs from its exact task", index)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if index != len(workload.Tasks) {
		return fmt.Errorf("approved agent workload task count differs from plan leaves")
	}
	proposal.ID = 0
	proposal.Phase = "plan_frozen"
	encoded, err := json.Marshal(proposal)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,payload_json) VALUES($1,$2,'coding_agent',$3::jsonb)`, jobID, planStepID, string(encoded))
	return err
}

func proposedCodingAgentWorkloadTx(ctx context.Context, tx pgx.Tx, jobID, generation, planStepID int64) (*codingagent.Workload, codingagent.Entry, error) {
	var receiptRaw, proposalRaw []byte
	if err := tx.QueryRow(ctx, `SELECT payload_json->'workload' FROM evidence WHERE job_id=$1 AND step_id=$2 AND kind='coding_agent' AND payload_json->>'generation'=$3 AND payload_json->>'phase'='plan_review_ready'`, jobID, planStepID, fmt.Sprint(generation)).Scan(&receiptRaw); err != nil {
		return nil, codingagent.Entry{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT payload_json FROM evidence WHERE job_id=$1 AND step_id=$2 AND kind='coding_agent' AND payload_json->>'generation'=$3 AND payload_json->>'phase'='plan_proposed'`, jobID, planStepID, fmt.Sprint(generation)).Scan(&proposalRaw); err != nil {
		return nil, codingagent.Entry{}, err
	}
	var workload codingagent.Workload
	if err := json.Unmarshal(receiptRaw, &workload); err != nil {
		return nil, codingagent.Entry{}, err
	}
	var proposal codingagent.Entry
	if err := json.Unmarshal(proposalRaw, &proposal); err != nil {
		return nil, codingagent.Entry{}, err
	}
	if proposal.Generation != generation || proposal.Phase != "plan_proposed" || proposal.Workload == nil || !reflect.DeepEqual(*proposal.Workload, workload) {
		return nil, codingagent.Entry{}, fmt.Errorf("agent plan review differs from its native proposal")
	}
	if err := workload.Validate(); err != nil {
		return nil, codingagent.Entry{}, err
	}
	return &workload, proposal, nil
}

func (repository *Repository) CodingAgentFrozenWorkload(ctx context.Context, authority model.StepAttemptAuthority) (*codingagent.Workload, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return nil, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return nil, staleStepAttemptError(authority, "frozen workload projection requires current execution", nil)
	}
	var action string
	if err := tx.QueryRow(ctx, `SELECT action FROM job_steps WHERE id=$1 AND job_id=$2 AND generation=$3`, authority.StepID, authority.JobID, authority.Generation).Scan(&action); err != nil {
		return nil, err
	}
	if action != "v3_coding" {
		return nil, fmt.Errorf("frozen agent workload requires the execution step")
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT frozen.payload_json->'workload' FROM coding_plans plan JOIN evidence frozen ON frozen.job_id=plan.job_id AND frozen.step_id=plan.plan_step_id AND frozen.kind='coding_agent' AND frozen.payload_json->>'phase'='plan_frozen' AND frozen.payload_json->>'generation'=plan.generation::text WHERE plan.job_id=$1 AND plan.generation=$2 AND plan.state='frozen'`, authority.JobID, authority.Generation).Scan(&raw); err != nil {
		return nil, err
	}
	var workload codingagent.Workload
	if err := json.Unmarshal(raw, &workload); err != nil {
		return nil, err
	}
	if err := workload.Validate(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &workload, nil
}
