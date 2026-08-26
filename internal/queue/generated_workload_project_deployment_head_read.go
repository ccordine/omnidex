package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const generatedWorkloadProjectDeploymentHeadColumns = `
	project_id,compose_project,secret_generation,deployment_key_fingerprint_sha256,
	active_deployment_id,active_endpoint_scheme,active_endpoint_host,
	active_endpoint_port,active_endpoint_path,revision,fence,
	candidate_deployment_id,candidate_job_id,candidate_generation,candidate_step_id,
	candidate_step_attempt,candidate_worker_id,created_at,updated_at
`

func (r *Repository) CurrentGeneratedWorkloadProjectDeploymentHead(
	ctx context.Context,
	projectID int64,
) (*GeneratedWorkloadProjectDeploymentHead, error) {
	if ctx == nil {
		return nil, fmt.Errorf("load project deployment head requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load project deployment head: %w", err)
	}
	if r == nil || r.pool == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if projectID <= 0 {
		return nil, fmt.Errorf("load project deployment head requires a positive project identity")
	}
	head, err := scanGeneratedWorkloadProjectDeploymentHead(r.pool.QueryRow(ctx, `
		SELECT `+generatedWorkloadProjectDeploymentHeadColumns+`
		FROM generated_workload_project_deployment_heads WHERE project_id=$1
	`, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load project deployment head: %w", err)
	}
	if err := validateGeneratedWorkloadProjectDeploymentHead(head); err != nil {
		return nil, fmt.Errorf("validate durable project deployment head: %w", err)
	}
	return &head, nil
}

func lockGeneratedWorkloadProjectDeploymentHeadTx(
	ctx context.Context,
	tx pgx.Tx,
	projectID int64,
) (GeneratedWorkloadProjectDeploymentHead, bool, error) {
	head, err := scanGeneratedWorkloadProjectDeploymentHead(tx.QueryRow(ctx, `
		SELECT `+generatedWorkloadProjectDeploymentHeadColumns+`
		FROM generated_workload_project_deployment_heads
		WHERE project_id=$1 FOR UPDATE
	`, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return GeneratedWorkloadProjectDeploymentHead{}, false, nil
	}
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, false, fmt.Errorf(
			"lock project deployment head: %w", err,
		)
	}
	if err := validateGeneratedWorkloadProjectDeploymentHead(head); err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, false, err
	}
	return head, true, nil
}

func scanGeneratedWorkloadProjectDeploymentHead(row pgx.Row) (
	GeneratedWorkloadProjectDeploymentHead,
	error,
) {
	var head GeneratedWorkloadProjectDeploymentHead
	var activeID, scheme, host, endpointPath, candidateID, candidateWorker *string
	var endpointPort *int32
	var candidateJobID, candidateGeneration, candidateStepID, candidateAttempt *int64
	err := row.Scan(
		&head.ProjectID, &head.ComposeProject, &head.SecretGeneration,
		&head.DeploymentKeyFingerprintSHA256, &activeID, &scheme, &host, &endpointPort,
		&endpointPath, &head.Revision, &head.Fence, &candidateID, &candidateJobID,
		&candidateGeneration, &candidateStepID, &candidateAttempt, &candidateWorker,
		&head.CreatedAt, &head.UpdatedAt,
	)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, err
	}
	if activeID != nil {
		if scheme == nil || host == nil || endpointPort == nil || endpointPath == nil {
			return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
				"project deployment head has incomplete durable endpoint authority",
			)
		}
		head.ActiveDeploymentID = *activeID
		head.Endpoint = &GeneratedWorkloadProjectDeploymentEndpoint{
			Scheme: *scheme, Host: *host, Port: uint16(*endpointPort), Path: *endpointPath,
		}
	}
	if candidateID != nil {
		if candidateJobID == nil || candidateGeneration == nil || candidateStepID == nil ||
			candidateAttempt == nil || candidateWorker == nil {
			return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
				"project deployment head has incomplete durable candidate authority",
			)
		}
		head.Candidate = &GeneratedWorkloadProjectDeploymentCandidate{
			DeploymentID: *candidateID,
			Authority: GeneratedWorkloadDeploymentAuthority{
				JobID: *candidateJobID, Generation: *candidateGeneration,
				StepID: *candidateStepID, ProjectID: head.ProjectID,
			},
			Executor: GeneratedWorkloadDeploymentExecutor{
				StepAttempt: *candidateAttempt, WorkerID: *candidateWorker,
			},
		}
	}
	return head, nil
}
