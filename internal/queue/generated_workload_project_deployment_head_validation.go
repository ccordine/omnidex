package queue

import (
	"fmt"
	"strings"
)

func validateGeneratedWorkloadProjectDeploymentHead(
	head GeneratedWorkloadProjectDeploymentHead,
) error {
	if head.ProjectID <= 0 || !generatedDeploymentProject.MatchString(head.ComposeProject) ||
		head.SecretGeneration <= 0 ||
		!repositoryMutationHexDigest(head.DeploymentKeyFingerprintSHA256) ||
		head.Revision < 0 || head.Fence <= 0 || head.CreatedAt.IsZero() || head.UpdatedAt.IsZero() {
		return fmt.Errorf("project deployment head authority is invalid")
	}
	if (head.ActiveDeploymentID == "") != (head.Endpoint == nil) {
		return fmt.Errorf("project deployment head active endpoint authority is incomplete")
	}
	if head.ActiveDeploymentID != "" {
		if !repositoryMutationOpaqueID(head.ActiveDeploymentID, "generated_workload_deployment_") {
			return fmt.Errorf("project deployment head active deployment identity is invalid")
		}
		if err := validateGeneratedDeploymentEndpoint(
			head.Endpoint.Scheme, head.Endpoint.Host, head.Endpoint.Path,
		); err != nil {
			return fmt.Errorf("project deployment head active endpoint is invalid: %w", err)
		}
		if head.Endpoint.Port == 0 {
			return fmt.Errorf("project deployment head active endpoint port is invalid")
		}
	}
	if head.Candidate == nil {
		return nil
	}
	if !repositoryMutationOpaqueID(
		head.Candidate.DeploymentID, "generated_workload_deployment_",
	) || head.Candidate.Authority.ProjectID != head.ProjectID ||
		head.Candidate.Authority.JobID <= 0 || head.Candidate.Authority.Generation <= 0 ||
		head.Candidate.Authority.StepID <= 0 || head.Candidate.Executor.StepAttempt <= 0 ||
		head.Candidate.Executor.WorkerID == "" ||
		strings.TrimSpace(head.Candidate.Executor.WorkerID) != head.Candidate.Executor.WorkerID ||
		len(head.Candidate.Executor.WorkerID) > 256 {
		return fmt.Errorf("project deployment head candidate authority is invalid")
	}
	return nil
}

func validateGeneratedWorkloadProjectDeploymentExpectation(
	expectation GeneratedWorkloadProjectDeploymentHeadExpectation,
) error {
	if expectation.Revision < 0 || expectation.Fence < 0 ||
		(expectation.Revision > 0 && expectation.Fence == 0) {
		return fmt.Errorf("project deployment head expectation is invalid")
	}
	return nil
}

func validateGeneratedWorkloadProjectDeploymentReservation(
	reservation GeneratedWorkloadProjectDeploymentReservation,
) error {
	if reservation.ProjectID <= 0 ||
		!repositoryMutationOpaqueID(reservation.DeploymentID, "generated_workload_deployment_") ||
		reservation.Revision < 0 || reservation.Fence <= 0 ||
		reservation.Authority.ProjectID != reservation.ProjectID ||
		reservation.Authority.JobID <= 0 || reservation.Authority.Generation <= 0 ||
		reservation.Authority.StepID <= 0 || reservation.Executor.StepAttempt <= 0 ||
		reservation.Executor.WorkerID == "" ||
		strings.TrimSpace(reservation.Executor.WorkerID) != reservation.Executor.WorkerID ||
		len(reservation.Executor.WorkerID) > 256 {
		return fmt.Errorf("project deployment reservation authority is invalid")
	}
	return nil
}

func generatedWorkloadProjectDeploymentReservation(
	head GeneratedWorkloadProjectDeploymentHead,
) (GeneratedWorkloadProjectDeploymentReservation, error) {
	if err := validateGeneratedWorkloadProjectDeploymentHead(head); err != nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, err
	}
	if head.Candidate == nil {
		return GeneratedWorkloadProjectDeploymentReservation{}, fmt.Errorf(
			"project deployment head has no reserved candidate",
		)
	}
	return GeneratedWorkloadProjectDeploymentReservation{
		ProjectID: head.ProjectID, DeploymentID: head.Candidate.DeploymentID,
		Revision: head.Revision, Fence: head.Fence,
		Authority: head.Candidate.Authority, Executor: head.Candidate.Executor,
	}, nil
}
