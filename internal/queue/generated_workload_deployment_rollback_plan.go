package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func validateGeneratedWorkloadDeploymentRollbackPlan(
	command GeneratedWorkloadDeploymentCommand,
	plan GeneratedWorkloadDeploymentRollbackPlan,
) error {
	if command.PriorDeploymentID != "" {
		return fmt.Errorf("first-deployment rollback plan cannot target a successor deployment")
	}
	if plan.Policy != GeneratedWorkloadDeploymentRollbackDestroyFirstV1 ||
		plan.MaxAttempts != MaxGeneratedWorkloadDeploymentRollbackAttempts ||
		plan.Execution.Slot != GeneratedDeploymentSlotRollback ||
		plan.Execution.WorkspaceSHA256 != command.WorkspaceSHA256 ||
		!repositoryMutationHexDigest(plan.Execution.CommandSHA256) ||
		plan.ComposeProject != command.ComposeProject ||
		plan.ResourceObservation != GeneratedWorkloadDeploymentRollbackResourcesV1 ||
		!plan.RequireContainerAbsence || !plan.RequireNetworkAbsence ||
		!plan.RequireVolumeAbsence ||
		plan.StateMarkerSHA256 != "" && !repositoryMutationHexDigest(plan.StateMarkerSHA256) {
		return fmt.Errorf("generated deployment rollback plan authority is invalid")
	}
	expectedJSON, expectedSHA, err := CanonicalGeneratedWorkloadDeploymentRollbackPostcondition(plan)
	if err != nil || plan.PostconditionJSON != expectedJSON || plan.PostconditionSHA256 != expectedSHA {
		return fmt.Errorf("generated deployment rollback postcondition authority is invalid: %w", err)
	}
	return nil
}

func CanonicalGeneratedWorkloadDeploymentRollbackPostcondition(
	plan GeneratedWorkloadDeploymentRollbackPlan,
) (string, string, error) {
	payload := struct {
		Policy                  string `json:"policy"`
		ComposeProject          string `json:"compose_project"`
		ResourceObservation     string `json:"resource_observation"`
		RequireContainerAbsence bool   `json:"require_container_absence"`
		RequireNetworkAbsence   bool   `json:"require_network_absence"`
		RequireVolumeAbsence    bool   `json:"require_volume_absence"`
		StateMarkerSHA256       string `json:"state_marker_sha256"`
	}{
		plan.Policy, plan.ComposeProject, plan.ResourceObservation,
		plan.RequireContainerAbsence, plan.RequireNetworkAbsence,
		plan.RequireVolumeAbsence, plan.StateMarkerSHA256,
	}
	encoded, err := canonicalGeneratedDeploymentJSON(payload)
	if err != nil {
		return "", "", err
	}
	return encoded, generatedDeploymentSHA(encoded), nil
}

func loadGeneratedDeploymentRollbackPlanTx(
	ctx context.Context,
	querier generatedDeploymentExecutionQuerier,
	operationID string,
	lock bool,
) (GeneratedWorkloadDeploymentRollbackPlan, bool, error) {
	query := `
		SELECT policy,max_attempts,slot_name,slot_ordinal,command_sha256,workspace_sha256,
		       compose_project,resource_observation,require_container_absence,
		       require_network_absence,require_volume_absence,
		       COALESCE(state_marker_sha256,''),postcondition_json,postcondition_sha256
		FROM generated_workload_deployment_rollback_plans WHERE operation_id=$1`
	if lock {
		query += ` FOR KEY SHARE`
	}
	var plan GeneratedWorkloadDeploymentRollbackPlan
	err := querier.QueryRow(ctx, query, operationID).Scan(
		&plan.Policy, &plan.MaxAttempts, &plan.Execution.Slot.Name,
		&plan.Execution.Slot.Ordinal, &plan.Execution.CommandSHA256,
		&plan.Execution.WorkspaceSHA256, &plan.ComposeProject,
		&plan.ResourceObservation, &plan.RequireContainerAbsence,
		&plan.RequireNetworkAbsence, &plan.RequireVolumeAbsence,
		&plan.StateMarkerSHA256, &plan.PostconditionJSON, &plan.PostconditionSHA256,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GeneratedWorkloadDeploymentRollbackPlan{}, false, nil
	}
	if err != nil {
		return GeneratedWorkloadDeploymentRollbackPlan{}, false, fmt.Errorf(
			"load generated deployment rollback plan: %w", err,
		)
	}
	return plan, true, nil
}

func equalGeneratedWorkloadDeploymentRollbackPlans(
	left, right GeneratedWorkloadDeploymentRollbackPlan,
) bool {
	return left == right
}
