package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) PrepareGeneratedWorkloadDeployment(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	verificationID string,
	manifest GeneratedWorkloadDeploymentLifecycleManifest,
	rollback GeneratedWorkloadDeploymentRollbackPlan,
) (GeneratedWorkloadDeploymentRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("prepare generated deployment requires PostgreSQL and context")
	}
	if err := validateGeneratedDeploymentExecutionAuthority(authority, command); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	manifestJSON, manifestSHA, err := canonicalGeneratedDeploymentLifecycleManifest(command, manifest)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if err := validateGeneratedWorkloadDeploymentRollbackPlan(command, rollback); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("begin generated deployment preparation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	existing, found, err := loadGeneratedDeploymentByGenerationTx(
		ctx, tx, command.Authority.JobID, command.Authority.Generation,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	var binding GeneratedWorkloadDeploymentVerificationBinding
	var boundRollback GeneratedWorkloadDeploymentRollbackPlan
	if found {
		if err := requireGeneratedDeploymentIdentity(existing, identity); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, err
		}
		var bound bool
		binding, bound, err = loadGeneratedDeploymentVerificationBindingTx(
			ctx, tx, identity.OperationID, true,
		)
		if err != nil || !bound {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("%w: deployment verification binding is unavailable", ErrGeneratedWorkloadDeploymentConflict)
		}
		boundRollback, bound, err = loadGeneratedDeploymentRollbackPlanTx(
			ctx, tx, identity.OperationID, true,
		)
		if err != nil || !bound {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf(
				"%w: deployment rollback plan is unavailable", ErrGeneratedWorkloadDeploymentConflict,
			)
		}
	}
	verificationIDs := []string{verificationID}
	if found {
		verificationIDs = append(verificationIDs, binding.VerificationID)
	}
	if err := lockGeneratedDeploymentVerificationIdentitiesTx(ctx, tx, verificationIDs...); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	verification, verified, err := loadGeneratedWorkloadVerificationByIDTx(ctx, tx, verificationID)
	if err != nil || !verified {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("load deployment workspace verification: %w", err)
	}
	var boundVerification GeneratedWorkloadVerificationRecord
	lockEvidenceIDs := append([]int64{verification.EvidenceID}, verification.CommandEvidenceIDs...)
	if found {
		var loaded bool
		boundVerification, loaded, err = loadGeneratedWorkloadVerificationByIDTx(
			ctx, tx, binding.VerificationID,
		)
		if err != nil || !loaded {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("load bound deployment workspace verification: %w", err)
		}
		lockEvidenceIDs = append(lockEvidenceIDs, boundVerification.EvidenceID)
		lockEvidenceIDs = append(lockEvidenceIDs, boundVerification.CommandEvidenceIDs...)
	}
	if err := lockGeneratedDeploymentEvidenceIDsTx(ctx, tx, lockEvidenceIDs); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if err := requireGeneratedDeploymentVerification(command, verification); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if err := requireGeneratedDeploymentResolvedConfigTx(ctx, tx, command, verification); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if found {
		if binding.WorkspaceSHA256 != command.WorkspaceSHA256 ||
			binding.LifecycleManifestSHA256 != manifestSHA ||
			!equalGeneratedWorkloadDeploymentRollbackPlans(boundRollback, rollback) {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("%w: deployment verification binding differs", ErrGeneratedWorkloadDeploymentConflict)
		}
		if !equivalentGeneratedWorkloadVerificationCommands(boundVerification.Commands, verification.Commands) {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("%w: resumed workspace verification proof differs", ErrGeneratedWorkloadDeploymentConflict)
		}
		if err := requireGeneratedDeploymentResolvedConfigTx(ctx, tx, command, boundVerification); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("commit generated deployment replay: %w", err)
		}
		return existing, nil
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO generated_workload_deployments (
		 id,command_sha256,command_json,job_id,generation,step_id,
		 creator_step_attempt,creator_worker_id,current_step_attempt,current_worker_id,
		 project_id,compose_project,bind_host,endpoint_port_authority,
		 requested_endpoint_port,prior_deployment_id,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,''),$15)
	`, identity.OperationID, identity.CommandSHA256, identity.CommandJSON,
		command.Authority.JobID, command.Authority.Generation, command.Authority.StepID,
		authority.Attempt, authority.WorkerID, command.Authority.ProjectID,
		command.ComposeProject, command.BindHost, command.EndpointPortAuthority, command.EndpointPort,
		command.PriorDeploymentID, GeneratedWorkloadDeploymentPrepared)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("insert generated deployment preparation: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO generated_workload_deployment_rollback_plans(
		 operation_id,policy,max_attempts,slot_name,slot_ordinal,command_sha256,workspace_sha256,
		 compose_project,resource_observation,require_container_absence,
		 require_network_absence,require_volume_absence,state_marker_sha256,
		 postcondition_json,postcondition_sha256)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,''),$14,$15)
	`, identity.OperationID, rollback.Policy, rollback.MaxAttempts,
		rollback.Execution.Slot.Name, rollback.Execution.Slot.Ordinal,
		rollback.Execution.CommandSHA256, rollback.Execution.WorkspaceSHA256,
		rollback.ComposeProject, rollback.ResourceObservation,
		rollback.RequireContainerAbsence, rollback.RequireNetworkAbsence,
		rollback.RequireVolumeAbsence, rollback.StateMarkerSHA256,
		rollback.PostconditionJSON, rollback.PostconditionSHA256)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("bind deployment rollback plan: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO generated_workload_deployment_verifications(
		 operation_id,verification_id,workspace_sha256,lifecycle_manifest_json,lifecycle_manifest_sha256)
		VALUES($1,$2,$3,$4,$5)
	`, identity.OperationID, verification.ID, verification.WorkspaceSHA256, manifestJSON, manifestSHA)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("bind deployment workspace verification: %w", err)
	}
	record, err := lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("commit generated deployment preparation: %w", err)
	}
	return record, nil
}

func requireGeneratedDeploymentResolvedConfigTx(
	ctx context.Context,
	tx pgx.Tx,
	command GeneratedWorkloadDeploymentCommand,
	verification GeneratedWorkloadVerificationRecord,
) error {
	if len(verification.CommandEvidenceIDs) == 0 {
		return fmt.Errorf("deployment workspace verification has no resolved config proof")
	}
	configEvidenceID := verification.CommandEvidenceIDs[len(verification.CommandEvidenceIDs)-1]
	var valid bool
	var serviceHashesJSON, environmentNamesJSON, namespacePreflightJSON []byte
	if err := tx.QueryRow(ctx, `
		SELECT source_type=$2 AND payload_json->'metadata'->>'resolved_config_sha256'=$3 AND
		       payload_json->'metadata'->>'workspace_sha256'=$4 AND
		       payload_json->'metadata'->>'secret_set_sha256'=$5 AND
		       payload_json->'metadata'->>'succeeded'='true' AND
		       payload_json->'metadata'->>'implicit_env_disabled'='true',
		       payload_json->'metadata'->'service_hashes',
		       payload_json->'metadata'->'environment_names',
		       payload_json->'metadata'->($8::TEXT)
		FROM evidence WHERE id=$1 AND job_id=$6 AND step_id=$7 FOR KEY SHARE
	`, configEvidenceID, GeneratedWorkloadResolvedConfigEvidenceSource,
		command.ConfigSHA256, command.WorkspaceSHA256, command.SecretSetSHA256,
		command.Authority.JobID, command.Authority.StepID,
		GeneratedWorkloadDeploymentNamespaceMetadataKey).Scan(
		&valid, &serviceHashesJSON, &environmentNamesJSON, &namespacePreflightJSON,
	); err != nil {
		return fmt.Errorf("load deployment resolved config proof: %w", err)
	}
	if !valid {
		return fmt.Errorf("deployment resolved config proof differs from immutable command authority")
	}
	var services []struct {
		Service string `json:"service"`
		SHA256  string `json:"sha256"`
	}
	if err := json.Unmarshal(serviceHashesJSON, &services); err != nil || len(services) != len(command.Services) {
		return fmt.Errorf("deployment resolved config service proof is invalid: %w", err)
	}
	for index, service := range services {
		if service.Service != command.Services[index] || !repositoryMutationHexDigest(service.SHA256) {
			return fmt.Errorf("deployment resolved config service %d differs from command authority", index)
		}
	}
	var environmentNames []string
	if err := json.Unmarshal(environmentNamesJSON, &environmentNames); err != nil {
		return fmt.Errorf("deployment resolved config environment proof is invalid: %w", err)
	}
	expectedEnvironment := append([]string{"HOST_BIND_ADDRESS", "HOST_HTTP_PORT"}, command.RequiredSecretNames...)
	sort.Strings(expectedEnvironment)
	if len(environmentNames) != len(expectedEnvironment) {
		return fmt.Errorf("deployment resolved config environment set differs from command authority")
	}
	for index := range expectedEnvironment {
		if environmentNames[index] != expectedEnvironment[index] {
			return fmt.Errorf("deployment resolved config environment set differs from command authority")
		}
	}
	var namespacePreflight GeneratedWorkloadDeploymentNamespacePreflight
	if err := json.Unmarshal(namespacePreflightJSON, &namespacePreflight); err != nil {
		return fmt.Errorf("deployment namespace preflight proof is invalid: %w", err)
	}
	boundNamespace, _, err := BindGeneratedWorkloadDeploymentNamespacePreflight(namespacePreflight)
	if err != nil || boundNamespace.ComposeProject != command.ComposeProject ||
		!GeneratedWorkloadDeploymentNamespaceVacant(boundNamespace) {
		return fmt.Errorf(
			"deployment namespace preflight does not prove the exact vacant Compose project: %w", err,
		)
	}
	return nil
}

func requireGeneratedDeploymentVerification(
	command GeneratedWorkloadDeploymentCommand,
	verification GeneratedWorkloadVerificationRecord,
) error {
	if verification.ID == "" || verification.JobID != command.Authority.JobID ||
		verification.Generation != command.Authority.Generation ||
		verification.StepID != command.Authority.StepID ||
		verification.WorkspaceSHA256 != command.WorkspaceSHA256 {
		return fmt.Errorf("%w: deployment workspace verification differs", ErrGeneratedWorkloadDeploymentConflict)
	}
	return nil
}
