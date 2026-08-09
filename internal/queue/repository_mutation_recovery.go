package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// UnresolvedRepositoryMutation returns the one exact durable command that must
// be reconciled before any new repository indexing or semantic work for the
// current generation. The schema forbids multiple unresolved operations. This
// read does not claim or transfer a running step; process-restart execution
// remains unavailable until queue writes carry monotonic step-attempt leases.
func (r *Repository) UnresolvedRepositoryMutation(
	ctx context.Context,
	jobID, generation int64,
) (*RepositoryMutationCommand, error) {
	if ctx == nil {
		return nil, fmt.Errorf("load unresolved repository mutation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load unresolved repository mutation: %w", err)
	}
	if jobID <= 0 || generation <= 0 {
		return nil, fmt.Errorf("load unresolved repository mutation requires positive job and generation identities")
	}
	if r == nil || r.pool == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var command RepositoryMutationCommand
	var operationID, commandSHA, status string
	err := r.pool.QueryRow(ctx, `
		SELECT operation.id, operation.command_sha256, operation.status,
		       operation.job_id, operation.step_id, operation.generation,
		       operation.worker_id, operation.contract_id, operation.stage_id,
		       operation.source_snapshot_id, operation.patch, operation.patch_sha256
		FROM repository_mutation_operations AS operation
		JOIN jobs ON jobs.id=operation.job_id
		WHERE operation.job_id=$1 AND operation.generation=$2
		  AND jobs.current_generation=$2
		  AND operation.status IN ($3,$4,$5)
	`, jobID, generation, repositoryMutationPrepared, repositoryMutationApplying,
		repositoryMutationIndeterminate).Scan(
		&operationID, &commandSHA, &status,
		&command.JobID, &command.StepID, &command.Generation,
		&command.WorkerID, &command.ContractID, &command.StageID,
		&command.SourceSnapshotID, &command.Patch, &command.PatchSHA256,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load unresolved repository mutation command: %w", err)
	}
	if status != repositoryMutationPrepared && status != repositoryMutationApplying &&
		status != repositoryMutationIndeterminate {
		return nil, fmt.Errorf("unresolved repository mutation %s has invalid status %q", operationID, status)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT file_id, path, source_sha256, source_size,
		       expected_sha256, expected_size
		FROM repository_mutation_files
		WHERE operation_id=$1
		ORDER BY ordinal
	`, operationID)
	if err != nil {
		return nil, fmt.Errorf("load unresolved repository mutation files: %w", err)
	}
	defer rows.Close()
	command.ChangedFiles = make([]RepositoryMutationFile, 0, maxRepositoryMutationFiles)
	for rows.Next() {
		var file RepositoryMutationFile
		if err := rows.Scan(
			&file.FileID, &file.Path, &file.SourceSHA256, &file.SourceSize,
			&file.ExpectedSHA256, &file.ExpectedSize,
		); err != nil {
			return nil, fmt.Errorf("scan unresolved repository mutation file: %w", err)
		}
		command.ChangedFiles = append(command.ChangedFiles, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unresolved repository mutation files: %w", err)
	}
	identity, err := repositoryMutationOperation(command)
	if err != nil {
		return nil, fmt.Errorf("validate unresolved repository mutation command: %w", err)
	}
	if identity.ID != operationID || identity.CommandSHA256 != commandSHA {
		return nil, fmt.Errorf(
			"%w: durable repository mutation %s does not match its command",
			ErrRepositoryMutationConflict, operationID,
		)
	}
	return &command, nil
}
