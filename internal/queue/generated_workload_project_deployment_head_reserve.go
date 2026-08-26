package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReserveGeneratedWorkloadProjectDeploymentCandidate(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	secretGeneration int64,
	deploymentKeyFingerprintSHA256 string,
	expectation GeneratedWorkloadProjectDeploymentHeadExpectation,
) (GeneratedWorkloadProjectDeploymentReservation, error) {
	if ctx == nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, fmt.Errorf(
			"reserve project deployment candidate requires a context",
		)
	}
	if err := ctx.Err(); err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, fmt.Errorf(
			"reserve project deployment candidate: %w", err,
		)
	}
	if r == nil || r.pool == nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, ErrRepositoryNotConfigured
	}
	if err := validateGeneratedWorkloadProjectDeploymentExpectation(expectation); err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, err
	}
	if secretGeneration <= 0 || !repositoryMutationHexDigest(deploymentKeyFingerprintSHA256) {
		return GeneratedWorkloadProjectDeploymentReservation{}, fmt.Errorf(
			"reserve project deployment candidate requires exact secret-generation and deployment-key authority",
		)
	}
	if err := validateGeneratedDeploymentExecutionAuthority(authority, command); err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, err
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, fmt.Errorf(
			"begin project deployment reservation: %w", err,
		)
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, err
	}
	record, err := lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, err
	}
	if err := requireGeneratedDeploymentIdentity(record, identity); err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, err
	}
	if record.State != GeneratedWorkloadDeploymentPrepared &&
		record.State != GeneratedWorkloadDeploymentApplying &&
		record.State != GeneratedWorkloadDeploymentIndeterminate {
		return GeneratedWorkloadProjectDeploymentReservation{}, fmt.Errorf(
			"%w: deployment candidate %s is %s",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
			identity.OperationID, record.State,
		)
	}
	head, found, err := lockGeneratedWorkloadProjectDeploymentHeadTx(
		ctx, tx, command.Authority.ProjectID,
	)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, err
	}
	if err := takeOverGeneratedWorkloadDeploymentExecutorTx(
		ctx, tx, authority, identity.OperationID, record, head, found,
	); err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, err
	}
	if !found {
		head, err = insertInitialGeneratedWorkloadProjectDeploymentHeadTx(
			ctx, tx, authority, command, identity.OperationID, secretGeneration,
			deploymentKeyFingerprintSHA256, expectation,
		)
	} else {
		head, err = reserveExistingGeneratedWorkloadProjectDeploymentHeadTx(
			ctx, tx, authority, command, identity.OperationID, secretGeneration,
			deploymentKeyFingerprintSHA256, expectation, head,
		)
	}
	if err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, err
	}
	reservation, err := generatedWorkloadProjectDeploymentReservation(head)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, fmt.Errorf(
			"commit project deployment reservation: %w", err,
		)
	}
	return reservation, nil
}

func insertInitialGeneratedWorkloadProjectDeploymentHeadTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	secretGeneration int64,
	keyFingerprint string,
	expectation GeneratedWorkloadProjectDeploymentHeadExpectation,
) (GeneratedWorkloadProjectDeploymentHead, error) {
	if expectation.Revision != 0 || expectation.Fence != 0 || command.PriorDeploymentID != "" {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"%w: first project deployment requires empty head and predecessor authority",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO generated_workload_project_deployment_heads(
		 project_id,compose_project,secret_generation,deployment_key_fingerprint_sha256,
		 revision,fence,candidate_deployment_id,candidate_job_id,candidate_generation,
		 candidate_step_id,candidate_step_attempt,candidate_worker_id)
		VALUES($1,$2,$3,$4,0,1,$5,$6,$7,$8,$9,$10)
		ON CONFLICT DO NOTHING
	`, command.Authority.ProjectID, command.ComposeProject, secretGeneration, keyFingerprint,
		operationID, command.Authority.JobID, command.Authority.Generation,
		command.Authority.StepID, authority.Attempt, authority.WorkerID)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"reserve first project deployment candidate: %w", err,
		)
	}
	if tag.RowsAffected() != 1 {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"%w: project deployment head was concurrently reserved",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	return reloadGeneratedWorkloadProjectDeploymentHeadTx(ctx, tx, command.Authority.ProjectID)
}

func reserveExistingGeneratedWorkloadProjectDeploymentHeadTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	secretGeneration int64,
	keyFingerprint string,
	expectation GeneratedWorkloadProjectDeploymentHeadExpectation,
	head GeneratedWorkloadProjectDeploymentHead,
) (GeneratedWorkloadProjectDeploymentHead, error) {
	if head.Revision != expectation.Revision || head.Fence != expectation.Fence ||
		head.ComposeProject != command.ComposeProject ||
		head.SecretGeneration != secretGeneration ||
		head.DeploymentKeyFingerprintSHA256 != keyFingerprint ||
		head.ActiveDeploymentID != command.PriorDeploymentID {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"%w: project deployment expectation or stable authority differs",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	if head.Candidate != nil {
		if head.Candidate.DeploymentID != operationID {
			return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
				"%w: project already has candidate %s",
				ErrGeneratedWorkloadProjectDeploymentHeadConflict, head.Candidate.DeploymentID,
			)
		}
		if head.Candidate.Authority.JobID == authority.JobID &&
			head.Candidate.Authority.Generation == authority.Generation &&
			head.Candidate.Authority.StepID == authority.StepID &&
			head.Candidate.Executor.StepAttempt == authority.Attempt &&
			head.Candidate.Executor.WorkerID == authority.WorkerID {
			return head, nil
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE generated_workload_project_deployment_heads
		SET fence=fence+1,candidate_deployment_id=$4,candidate_job_id=$5,
		    candidate_generation=$6,candidate_step_id=$7,candidate_step_attempt=$8,
		    candidate_worker_id=$9,
		    updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
		WHERE project_id=$1 AND revision=$2 AND fence=$3
	`, head.ProjectID, expectation.Revision, expectation.Fence, operationID,
		command.Authority.JobID, command.Authority.Generation, command.Authority.StepID,
		authority.Attempt, authority.WorkerID)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"reserve project deployment candidate: %w", err,
		)
	}
	if tag.RowsAffected() != 1 {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"%w: project deployment head changed during reservation",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	return reloadGeneratedWorkloadProjectDeploymentHeadTx(ctx, tx, head.ProjectID)
}

func reloadGeneratedWorkloadProjectDeploymentHeadTx(
	ctx context.Context,
	tx pgx.Tx,
	projectID int64,
) (GeneratedWorkloadProjectDeploymentHead, error) {
	head, found, err := lockGeneratedWorkloadProjectDeploymentHeadTx(ctx, tx, projectID)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, err
	}
	if !found {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"%w: project deployment head disappeared",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	return head, nil
}
