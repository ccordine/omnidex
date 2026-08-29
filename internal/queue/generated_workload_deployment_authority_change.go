package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// GeneratedWorkloadDeploymentBlocker identifies durable deployment authority
// that does not belong to the current job generation and must be reconciled
// before ordinary workspace work can continue.
type GeneratedWorkloadDeploymentBlocker struct {
	OperationID string
	JobID       int64
	Generation  int64
	ProjectID   int64
	State       GeneratedWorkloadDeploymentState
	Candidate   bool
}

// UnresolvedGeneratedWorkloadDeployment returns one deterministic historical
// or foreign-project-candidate blocker. The current generation's own deployment
// is deliberately excluded because CurrentGeneratedWorkloadDeployment owns its
// typed recovery path.
func (r *Repository) UnresolvedGeneratedWorkloadDeployment(
	ctx context.Context,
	jobID, currentGeneration int64,
) (*GeneratedWorkloadDeploymentBlocker, error) {
	if ctx == nil {
		return nil, fmt.Errorf("inspect unresolved generated deployment requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("inspect unresolved generated deployment: %w", err)
	}
	if jobID <= 0 || currentGeneration <= 0 {
		return nil, fmt.Errorf("inspect unresolved generated deployment requires positive job and generation identities")
	}
	if r == nil || r.pool == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var blocker GeneratedWorkloadDeploymentBlocker
	err := r.pool.QueryRow(ctx, `
		SELECT deployment.id,deployment.job_id,deployment.generation,
		       deployment.project_id,deployment.status,
		       COALESCE(head.candidate_deployment_id=deployment.id,FALSE)
		FROM jobs AS requested
		JOIN generated_workload_deployments AS deployment
		  ON deployment.project_id=requested.project_id
		LEFT JOIN generated_workload_project_deployment_heads AS head
		  ON head.project_id=deployment.project_id
		WHERE requested.id=$1 AND (
		 (deployment.status IN ('prepared','applying','indeterminate') AND
		  (deployment.job_id<>$1 OR deployment.generation<>$2)) OR
		 (head.candidate_deployment_id=deployment.id AND
		  (head.candidate_job_id IS DISTINCT FROM $1 OR
		   head.candidate_generation IS DISTINCT FROM $2))
		)
		ORDER BY COALESCE(head.candidate_deployment_id=deployment.id,FALSE) DESC,
		         deployment.generation, deployment.id
		LIMIT 1
	`, jobID, currentGeneration).Scan(
		&blocker.OperationID, &blocker.JobID, &blocker.Generation,
		&blocker.ProjectID, &blocker.State, &blocker.Candidate,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect unresolved generated deployment for job %d: %w", jobID, err)
	}
	return &blocker, nil
}

// rejectUnresolvedGeneratedWorkloadDeploymentsTx must run while the owning job
// row is locked and before a transaction changes job, step, or attempt
// authority. Leaving the attempt active allows lease-expiry takeover to recover
// the durable deployment instead of orphaning it.
func rejectUnresolvedGeneratedWorkloadDeploymentsTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
) error {
	var operationID, state string
	var generation int64
	err := tx.QueryRow(ctx, `
		SELECT id,generation,status
		FROM generated_workload_deployments
		WHERE job_id=$1 AND status IN ('prepared','applying','indeterminate')
		ORDER BY generation,id
		LIMIT 1
		FOR UPDATE
	`, jobID).Scan(&operationID, &generation, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check unresolved generated deployments for job %d: %w", jobID, err)
	}
	return fmt.Errorf(
		"%w: job %d cannot change execution authority while deployment %s generation %d is %s",
		ErrGeneratedWorkloadDeploymentState, jobID, operationID, generation, state,
	)
}
