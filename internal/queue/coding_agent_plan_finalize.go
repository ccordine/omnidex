package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type codingAgentPlanInput struct {
	ID             model.CodingPlanLeafID   `json:"id"`
	Statement      string                   `json:"statement"`
	Decision       model.CodingPlanDecision `json:"decision"`
	Origin         int64                    `json:"decision_origin_generation"`
	ResultSchema   string                   `json:"result_schema"`
	ResultRelation string                   `json:"result_relation"`
}

type codingAgentPlanSnapshot struct {
	JobID      int64                  `json:"job_id"`
	Generation int64                  `json:"generation"`
	Revision   int64                  `json:"revision"`
	State      model.CodingPlanState  `json:"state"`
	Leaves     []model.CodingPlanLeaf `json:"leaves"`
}

type codingAgentPlanReceipt struct {
	IntakeContext *codingagent.IntakeContext `json:"intake_context,omitempty"`
	Generation    int64                      `json:"generation"`
	Phase         string                     `json:"phase"`
	Authority     model.StepAttemptAuthority `json:"authority"`
	Delegation    codingagent.Configuration  `json:"delegation"`
	Inputs        []codingAgentPlanInput     `json:"input_leaves"`
	Snapshot      codingAgentPlanSnapshot    `json:"snapshot"`
}

func codingAgentPlanInputs(command StoreCodingPlanReviewCommand) []codingAgentPlanInput {
	inputs := make([]codingAgentPlanInput, len(command.Leaves))
	for index, write := range command.Leaves {
		inputs[index] = codingAgentPlanInput{ID: write.Leaf.ID, Statement: write.Leaf.Statement, Decision: write.Leaf.Decision, Origin: write.DecisionOriginGeneration, ResultSchema: write.ResultRelation.Schema, ResultRelation: write.ResultRelation.Relation}
	}
	return inputs
}

func finalizeCodingAgentPlanTx(ctx context.Context, tx pgx.Tx, command StoreCodingPlanReviewCommand) error {
	if _, err := tx.Exec(ctx, `UPDATE coding_plan_leaves SET decision='approved',decision_origin_generation=generation,updated_at=clock_timestamp() WHERE job_id=$1 AND generation=$2 AND decision='pending'`, command.Authority.JobID, command.Authority.Generation); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE coding_plans SET state='frozen',revision=revision+1,frozen_at=clock_timestamp(),updated_at=clock_timestamp() WHERE job_id=$1 AND generation=$2 AND state='review' AND revision=1`, command.Authority.JobID, command.Authority.Generation)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("delegated agent plan lost its initial freeze authority")
	}
	return nil
}

func recordCodingAgentPlanFinalizationTx(ctx context.Context, tx pgx.Tx, command StoreCodingPlanReviewCommand, configuration codingagent.Configuration, plan model.CodingPlan) error {
	receipt := codingAgentPlanReceipt{IntakeContext: command.IntakeContext, Generation: plan.Generation, Phase: "requirements_frozen", Authority: command.Authority, Delegation: configuration, Inputs: codingAgentPlanInputs(command), Snapshot: codingAgentPlanSnapshot{JobID: plan.JobID, Generation: plan.Generation, Revision: plan.Revision, State: plan.State, Leaves: plan.Leaves}}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,payload_json) VALUES($1,$2,'coding_agent',$3::jsonb)`, plan.JobID, command.Authority.StepID, string(encoded))
	return err
}

func requireStoredCodingAgentPlanCommandTx(ctx context.Context, tx pgx.Tx, plan model.CodingPlan, command StoreCodingPlanReviewCommand, configuration codingagent.Configuration) error {
	if plan.State != model.CodingPlanStateFrozen {
		return fmt.Errorf("delegated agent plan replay lacks the frozen result")
	}
	inputs, err := json.Marshal(codingAgentPlanInputs(command))
	if err != nil {
		return err
	}
	authority, err := json.Marshal(command.Authority)
	if err != nil {
		return err
	}
	delegation, err := json.Marshal(configuration)
	if err != nil {
		return err
	}
	intake, err := json.Marshal(command.IntakeContext)
	if err != nil {
		return err
	}
	var exact bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evidence WHERE job_id=$1 AND step_id=$2 AND kind='coding_agent' AND payload_json->>'phase'='requirements_frozen' AND payload_json->>'generation'=$3 AND payload_json->'input_leaves'=$4::jsonb AND payload_json->'authority'=$5::jsonb AND payload_json->'delegation'=$6::jsonb AND COALESCE(payload_json->'intake_context','null'::jsonb)=$7::jsonb)`, plan.JobID, command.Authority.StepID, fmt.Sprint(plan.Generation), string(inputs), string(authority), string(delegation), string(intake)).Scan(&exact); err != nil {
		return err
	}
	if !exact {
		return fmt.Errorf("delegated agent plan replay differs from exact planning command")
	}
	expected := command
	expected.Leaves = append([]CodingPlanLeafWrite{}, command.Leaves...)
	for index := range expected.Leaves {
		if expected.Leaves[index].Leaf.Decision == model.CodingPlanDecisionPending {
			expected.Leaves[index].Leaf.Decision = model.CodingPlanDecisionApproved
		}
	}
	return requireStoredCodingPlanCommandTx(ctx, tx, plan, expected)
}
