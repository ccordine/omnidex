package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
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
	Workload      *codingagent.Workload      `json:"workload"`
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

func recordCodingAgentPlanProposalTx(ctx context.Context, tx pgx.Tx, command StoreCodingPlanReviewCommand, configuration codingagent.Configuration, plan model.CodingPlan) error {
	receipt := codingAgentPlanReceipt{IntakeContext: command.IntakeContext, Workload: command.AgentWorkload, Generation: plan.Generation, Phase: "plan_review_ready", Authority: command.Authority, Delegation: configuration, Inputs: codingAgentPlanInputs(command), Snapshot: codingAgentPlanSnapshot{JobID: plan.JobID, Generation: plan.Generation, Revision: plan.Revision, State: plan.State, Leaves: plan.Leaves}}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,payload_json) VALUES($1,$2,'coding_agent',$3::jsonb)`, plan.JobID, command.Authority.StepID, string(encoded))
	return err
}

func requireStoredCodingAgentPlanCommandTx(ctx context.Context, tx pgx.Tx, plan model.CodingPlan, command StoreCodingPlanReviewCommand, configuration codingagent.Configuration) error {
	if plan.State != model.CodingPlanStateReview {
		return fmt.Errorf("delegated agent plan replay lacks the reviewable result")
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
	workload, err := json.Marshal(command.AgentWorkload)
	if err != nil {
		return err
	}
	var exact bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evidence WHERE job_id=$1 AND step_id=$2 AND kind='coding_agent' AND payload_json->>'phase'='plan_review_ready' AND payload_json->>'generation'=$3 AND payload_json->'input_leaves'=$4::jsonb AND payload_json->'authority'=$5::jsonb AND payload_json->'delegation'=$6::jsonb AND COALESCE(payload_json->'intake_context','null'::jsonb)=$7::jsonb AND payload_json->'workload'=$8::jsonb)`, plan.JobID, command.Authority.StepID, fmt.Sprint(plan.Generation), string(inputs), string(authority), string(delegation), string(intake), string(workload)).Scan(&exact); err != nil {
		return err
	}
	if !exact {
		return fmt.Errorf("delegated agent plan replay differs from exact planning command")
	}
	return requireStoredCodingPlanCommandTx(ctx, tx, plan, command)
}

func requireCodingAgentPlanProjection(command StoreCodingPlanReviewCommand) error {
	if len(command.Leaves) != len(command.AgentWorkload.Tasks) {
		return fmt.Errorf("agent plan review leaves do not cover its exact tasks")
	}
	for index, task := range command.AgentWorkload.Tasks {
		receipt := command.Leaves[index].ResultRelation
		if receipt.Schema != assemblyline.ApplicationRequirementCandidateResultRelationSchemaV1 || receipt.Relation != assemblyline.ApplicationRequirementNoDerivedResult {
			return fmt.Errorf("agent plan review leaf %d lacks its structural ledger receipt", index)
		}
		statement, err := codingagent.PlanTaskStatement(task)
		if err != nil {
			return err
		}
		if command.Leaves[index].Leaf.Statement != statement {
			return fmt.Errorf("agent plan review leaf %d differs from task %s", index, task.ID)
		}
	}
	return nil
}
