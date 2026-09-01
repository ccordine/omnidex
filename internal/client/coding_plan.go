package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type CodingPlanDecisionChange struct {
	LeafID   model.CodingPlanLeafID   `json:"leaf_id"`
	Decision model.CodingPlanDecision `json:"decision"`
}

type CodingPlanFreezeReceipt struct {
	Plan      model.CodingPlan `json:"plan"`
	JobStatus string           `json:"job_status"`
}

func (client *Client) CodingPlan(
	ctx context.Context,
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
) (model.CodingPlan, error) {
	if err := validateCodingPlanAuthority(channel, workspaceIdentity, jobID); err != nil {
		return model.CodingPlan{}, err
	}
	var plan model.CodingPlan
	query := url.Values{}
	query.Set("workspace_root", channel.WorkspaceRoot)
	query.Set("workspace_identity", workspaceIdentity)
	path := fmt.Sprintf("/v1/jobs/%d/plan?%s", jobID, query.Encode())
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &plan, http.StatusOK); err != nil {
		return model.CodingPlan{}, err
	}
	if err := validateCodingPlanResponse(plan, jobID); err != nil {
		return model.CodingPlan{}, fmt.Errorf("invalid coding plan response: %w", err)
	}
	return plan, nil
}

func (client *Client) DecideCodingPlan(
	ctx context.Context,
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
	operationID queue.LifecycleOperationID,
	generation int64,
	revision int64,
	decisions []CodingPlanDecisionChange,
) (model.CodingPlan, error) {
	if err := validateCodingPlanMutation(
		channel, workspaceIdentity, jobID, operationID, generation, revision,
	); err != nil {
		return model.CodingPlan{}, err
	}
	if err := validateCodingPlanDecisionChanges(decisions); err != nil {
		return model.CodingPlan{}, err
	}
	payload := struct {
		OperationID       queue.LifecycleOperationID `json:"operation_id"`
		Generation        int64                      `json:"generation"`
		Revision          int64                      `json:"revision"`
		WorkspaceRoot     string                     `json:"workspace_root"`
		WorkspaceIdentity string                     `json:"workspace_identity"`
		Decisions         []CodingPlanDecisionChange `json:"decisions"`
	}{
		OperationID: operationID, Generation: generation, Revision: revision,
		WorkspaceRoot: channel.WorkspaceRoot, WorkspaceIdentity: workspaceIdentity,
		Decisions: append([]CodingPlanDecisionChange(nil), decisions...),
	}
	var plan model.CodingPlan
	path := fmt.Sprintf("/v1/jobs/%d/plan/decisions", jobID)
	if err := client.doJSON(ctx, http.MethodPost, path, payload, &plan, http.StatusOK); err != nil {
		return model.CodingPlan{}, err
	}
	if err := validateCodingPlanResponse(plan, jobID); err != nil {
		return model.CodingPlan{}, fmt.Errorf("invalid coding plan decision response: %w", err)
	}
	return plan, nil
}

func (client *Client) FreezeCodingPlan(
	ctx context.Context,
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
	operationID queue.LifecycleOperationID,
	generation int64,
	revision int64,
) (CodingPlanFreezeReceipt, error) {
	if err := validateCodingPlanMutation(
		channel, workspaceIdentity, jobID, operationID, generation, revision,
	); err != nil {
		return CodingPlanFreezeReceipt{}, err
	}
	payload := struct {
		OperationID       queue.LifecycleOperationID `json:"operation_id"`
		Generation        int64                      `json:"generation"`
		Revision          int64                      `json:"revision"`
		WorkspaceRoot     string                     `json:"workspace_root"`
		WorkspaceIdentity string                     `json:"workspace_identity"`
	}{
		OperationID: operationID, Generation: generation, Revision: revision,
		WorkspaceRoot: channel.WorkspaceRoot, WorkspaceIdentity: workspaceIdentity,
	}
	var receipt CodingPlanFreezeReceipt
	path := fmt.Sprintf("/v1/jobs/%d/plan/freeze", jobID)
	if err := client.doJSON(ctx, http.MethodPost, path, payload, &receipt, http.StatusOK); err != nil {
		return CodingPlanFreezeReceipt{}, err
	}
	if err := validateCodingPlanResponse(receipt.Plan, jobID); err != nil {
		return CodingPlanFreezeReceipt{}, fmt.Errorf("invalid coding plan freeze response: %w", err)
	}
	if receipt.Plan.State != model.CodingPlanStateFrozen {
		return CodingPlanFreezeReceipt{}, fmt.Errorf(
			"coding plan freeze response has state %q", receipt.Plan.State,
		)
	}
	if receipt.JobStatus != model.JobStatusRunning {
		return CodingPlanFreezeReceipt{}, fmt.Errorf(
			"coding plan freeze response has job status %q", receipt.JobStatus,
		)
	}
	return receipt, nil
}

func validateCodingPlanAuthority(
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
) error {
	if err := validateLifecycleWorkspace(channel, workspaceIdentity); err != nil {
		return err
	}
	if jobID < 1 {
		return fmt.Errorf("coding plan requires a positive job ID")
	}
	return nil
}

func validateCodingPlanMutation(
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
	operationID queue.LifecycleOperationID,
	generation int64,
	revision int64,
) error {
	if err := validateCodingPlanAuthority(channel, workspaceIdentity, jobID); err != nil {
		return err
	}
	if _, err := queue.ParseLifecycleOperationID(string(operationID)); err != nil {
		return err
	}
	if generation < 1 || revision < 1 {
		return fmt.Errorf("coding plan mutation requires positive generation and revision authority")
	}
	return nil
}

func validateCodingPlanDecisionChanges(changes []CodingPlanDecisionChange) error {
	if len(changes) == 0 || len(changes) > model.MaxCodingPlanLeaves {
		return fmt.Errorf(
			"coding plan decision mutation requires between 1 and %d changes",
			model.MaxCodingPlanLeaves,
		)
	}
	seen := make(map[model.CodingPlanLeafID]struct{}, len(changes))
	for index, change := range changes {
		if _, err := model.ParseCodingPlanLeafID(string(change.LeafID)); err != nil {
			return fmt.Errorf("coding plan decision %d: %w", index, err)
		}
		if _, duplicate := seen[change.LeafID]; duplicate {
			return fmt.Errorf("coding plan decision leaf %q is duplicated", change.LeafID)
		}
		seen[change.LeafID] = struct{}{}
		switch change.Decision {
		case model.CodingPlanDecisionApproved, model.CodingPlanDecisionRejected:
		default:
			return fmt.Errorf("coding plan decision %d has unsupported value %q", index, change.Decision)
		}
	}
	return nil
}

func validateCodingPlanResponse(plan model.CodingPlan, expectedJobID int64) error {
	if plan.JobID != expectedJobID {
		return fmt.Errorf("coding plan job %d differs from requested job %d", plan.JobID, expectedJobID)
	}
	return plan.Validate()
}
