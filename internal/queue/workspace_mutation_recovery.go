package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
	"github.com/jackc/pgx/v5"
)

// UnresolvedWorkspaceMutation returns the one exact nonterminal command for a
// generation. It transfers no authority; ExecuteWorkspaceMutation must still
// run under the current exact step attempt.
func (r *Repository) UnresolvedWorkspaceMutation(
	ctx context.Context,
	jobID, generation int64,
) (*WorkspaceMutationCommand, error) {
	if ctx == nil {
		return nil, fmt.Errorf("load unresolved workspace mutation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load unresolved workspace mutation: %w", err)
	}
	if jobID <= 0 || generation <= 0 {
		return nil, fmt.Errorf("load unresolved workspace mutation requires positive job and generation identities")
	}
	if r == nil || r.pool == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var command WorkspaceMutationCommand
	var operationID, commandSHA, status, verificationJSON, verificationSHA string
	var sourceGit *string
	var gitRepositoryID, gitHeadCommit string
	err := r.pool.QueryRow(ctx, `
		SELECT operation.id,operation.command_sha256,operation.status,
		       operation.job_id,operation.step_id,operation.generation,
		       operation.creator_step_attempt,operation.creator_worker_id,operation.project_id,
		       operation.owner_id,operation.stage_id,operation.workspace_id,operation.workspace_root,
		       operation.source_state_id,operation.expected_state_id,
		       operation.source_repository_snapshot_id,operation.patch,operation.patch_sha256,
		       operation.verification_plan_json,operation.verification_plan_sha256,
		       COALESCE(snapshot.repository_id,''),COALESCE(snapshot.head_commit,'')
		FROM workspace_mutation_operations AS operation
		JOIN jobs ON jobs.id=operation.job_id
		LEFT JOIN repository_snapshots AS snapshot
		  ON snapshot.project_id=operation.project_id AND
		     snapshot.id=operation.source_repository_snapshot_id
		WHERE operation.job_id=$1 AND operation.generation=$2 AND
		      jobs.current_generation=$2 AND
		      operation.status NOT IN ($3,$4)
	`, jobID, generation, workspaceMutationVerified,
		workspaceMutationVerificationFailed).Scan(
		&operationID, &commandSHA, &status,
		&command.JobID, &command.StepID, &command.Generation,
		&command.CreatorAttempt, &command.CreatorWorkerID, &command.ProjectID,
		&command.Plan.OwnerID, &command.Plan.ID, &command.Plan.WorkspaceID,
		&command.Plan.WorkspaceRoot, &command.Plan.SourceStateID,
		&command.Plan.ExpectedStateID, &sourceGit, &command.Plan.Patch,
		&command.Plan.PatchSHA256, &verificationJSON, &verificationSHA,
		&gitRepositoryID, &gitHeadCommit,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load unresolved workspace mutation command: %w", err)
	}
	command.Plan.Schema = workspacefacts.MutationPlanSchemaV1
	if sourceGit != nil {
		command.Plan.GitSourceSnapshotID = *sourceGit
		command.Plan.GitRepositoryID = gitRepositoryID
		command.Plan.GitHeadCommit = gitHeadCommit
	}
	if digestWorkspaceMutationText(verificationJSON) != verificationSHA {
		return nil, fmt.Errorf("durable workspace mutation verification plan digest differs")
	}
	if err := json.Unmarshal([]byte(verificationJSON), &command.Verification); err != nil {
		return nil, fmt.Errorf("decode unresolved workspace mutation verification plan: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT file_id,path,
		       source_present,source_kind,source_sha256,source_size,source_mode,source_link_target,
		       expected_present,expected_kind,expected_sha256,expected_size,expected_mode,expected_link_target
		FROM workspace_mutation_files WHERE operation_id=$1 ORDER BY ordinal
	`, operationID)
	if err != nil {
		return nil, fmt.Errorf("load unresolved workspace mutation files: %w", err)
	}
	defer rows.Close()
	command.Plan.Files = make([]workspacefacts.MutationFileTransition, 0, workspacefacts.MaxMutationFiles)
	for rows.Next() {
		file, err := scanWorkspaceMutationFile(rows)
		if err != nil {
			return nil, err
		}
		command.Plan.Files = append(command.Plan.Files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unresolved workspace mutation files: %w", err)
	}
	identity, err := workspaceMutationOperation(command)
	if err != nil {
		return nil, fmt.Errorf("validate unresolved workspace mutation command: %w", err)
	}
	if identity.ID != operationID || identity.CommandSHA256 != commandSHA ||
		identity.PlanJSON != verificationJSON || identity.PlanSHA256 != verificationSHA {
		return nil, fmt.Errorf("%w: durable workspace mutation %s differs from its command", ErrWorkspaceMutationConflict, operationID)
	}
	return &command, nil
}

func scanWorkspaceMutationFile(rows pgx.Rows) (workspacefacts.MutationFileTransition, error) {
	var file workspacefacts.MutationFileTransition
	var sourceKind, sourceSHA, sourceLink *string
	var sourceSize *int64
	var sourceMode *int32
	var expectedKind, expectedSHA, expectedLink *string
	var expectedSize *int64
	var expectedMode *int32
	if err := rows.Scan(
		&file.FileID, &file.Path,
		&file.Source.Present, &sourceKind, &sourceSHA, &sourceSize, &sourceMode, &sourceLink,
		&file.Expected.Present, &expectedKind, &expectedSHA, &expectedSize, &expectedMode, &expectedLink,
	); err != nil {
		return workspacefacts.MutationFileTransition{}, fmt.Errorf("scan unresolved workspace mutation file: %w", err)
	}
	if sourceLink != nil || expectedLink != nil {
		return workspacefacts.MutationFileTransition{}, fmt.Errorf("durable workspace mutation %q contains unsupported symlink transition", file.Path)
	}
	var err error
	file.Source.SHA256, file.Source.Size, file.Source.Mode, err = assignWorkspaceMutationSQLState(
		file.FileID, "source", file.Source.Present, sourceKind, sourceSHA, sourceSize, sourceMode,
	)
	if err != nil {
		return workspacefacts.MutationFileTransition{}, err
	}
	file.Expected.SHA256, file.Expected.Size, file.Expected.Mode, err = assignWorkspaceMutationSQLState(
		file.FileID, "expected", file.Expected.Present,
		expectedKind, expectedSHA, expectedSize, expectedMode,
	)
	if err != nil {
		return workspacefacts.MutationFileTransition{}, err
	}
	return file, nil
}
