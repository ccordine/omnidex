package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func canonicalGeneratedDeploymentLifecycleManifest(
	command GeneratedWorkloadDeploymentCommand,
	manifest GeneratedWorkloadDeploymentLifecycleManifest,
) (string, string, error) {
	if manifest.Schema != GeneratedWorkloadDeploymentLifecycleManifestV1 ||
		len(manifest.Commands) < 6 || len(manifest.Commands) > 9 {
		return "", "", fmt.Errorf("deployment lifecycle manifest count or schema is invalid")
	}
	bySlot := make(map[GeneratedWorkloadDeploymentLifecycleSlot]GeneratedWorkloadDeploymentExecutionRecord)
	previous := 0
	for index, execution := range manifest.Commands {
		if err := validateGeneratedDeploymentExecutionCommand(command, execution); err != nil {
			return "", "", fmt.Errorf("deployment lifecycle manifest command %d: %w", index, err)
		}
		if execution.Slot.Ordinal <= previous || execution.Slot == GeneratedDeploymentSlotConfig ||
			execution.Slot == GeneratedDeploymentSlotRollback {
			return "", "", fmt.Errorf("deployment lifecycle manifest slots must be ordered successful operations")
		}
		previous = execution.Slot.Ordinal
		bySlot[execution.Slot] = GeneratedWorkloadDeploymentExecutionRecord{}
	}
	if len(bySlot) != len(manifest.Commands) {
		return "", "", fmt.Errorf("deployment lifecycle manifest repeats a slot")
	}
	if err := validateGeneratedDeploymentSuccessfulSlots(bySlot); err != nil {
		return "", "", err
	}
	encoded, err := canonicalGeneratedDeploymentJSON(manifest)
	if err != nil || len(encoded) > 8192 {
		return "", "", fmt.Errorf("deployment lifecycle manifest exceeds canonical bound: %w", err)
	}
	return encoded, generatedDeploymentSHA(encoded), nil
}

func loadGeneratedDeploymentVerificationBindingTx(
	ctx context.Context,
	querier generatedDeploymentExecutionQuerier,
	operationID string,
	lock bool,
) (GeneratedWorkloadDeploymentVerificationBinding, bool, error) {
	query := `
		SELECT operation_id,verification_id,workspace_sha256,
		       lifecycle_manifest_json,lifecycle_manifest_sha256
		FROM generated_workload_deployment_verifications WHERE operation_id=$1`
	if lock {
		query += ` FOR KEY SHARE`
	}
	var binding GeneratedWorkloadDeploymentVerificationBinding
	var manifestJSON string
	err := querier.QueryRow(ctx, query, operationID).Scan(
		&binding.OperationID, &binding.VerificationID, &binding.WorkspaceSHA256,
		&manifestJSON, &binding.LifecycleManifestSHA256,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GeneratedWorkloadDeploymentVerificationBinding{}, false, nil
	}
	if err != nil {
		return GeneratedWorkloadDeploymentVerificationBinding{}, false, fmt.Errorf("load deployment verification binding: %w", err)
	}
	if err := decodeExactGeneratedDeploymentJSON(manifestJSON, &binding.LifecycleManifest); err != nil {
		return GeneratedWorkloadDeploymentVerificationBinding{}, false, fmt.Errorf("decode deployment lifecycle manifest: %w", err)
	}
	return binding, true, nil
}
