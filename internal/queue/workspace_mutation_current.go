package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
	"github.com/jackc/pgx/v5"
)

type currentWorkspaceMutationRecord struct {
	operationID                  string
	commandSHA256                string
	status                       string
	verificationJSON             string
	verificationSHA256           string
	mutationEvidenceID           *int64
	verificationSucceeded        *bool
	verificationReceipt          *string
	verificationReceiptSHA       *string
	verificationEvidenceID       *int64
	verifiedRepositorySnapshotID *string
	lastError                    *string
}

// CurrentWorkspaceMutation returns the one exact workspace mutation operation
// for a job's current generation, including immutable terminal receipt
// authority when verification has completed. No operation returns nil. More
// than one operation is an authority conflict, never an implicit selection.
// A generation other than the job's current generation is stale authority.
func (r *Repository) CurrentWorkspaceMutation(
	ctx context.Context,
	jobID, generation int64,
) (*WorkspaceMutationSnapshot, error) {
	if ctx == nil {
		return nil, fmt.Errorf("load current workspace mutation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load current workspace mutation: %w", err)
	}
	if jobID <= 0 || generation <= 0 {
		return nil, fmt.Errorf("load current workspace mutation requires positive job and generation identities")
	}
	if r == nil || r.pool == nil {
		return nil, ErrRepositoryNotConfigured
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, fmt.Errorf("begin current workspace mutation read: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentGeneration int64
	err = tx.QueryRow(ctx, `
		SELECT current_generation FROM jobs WHERE id=$1 FOR SHARE
	`, jobID).Scan(&currentGeneration)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("load current workspace mutation job %d: %w", jobID, err)
	}
	if err != nil {
		return nil, fmt.Errorf("load current workspace mutation generation: %w", err)
	}
	if currentGeneration != generation {
		return nil, fmt.Errorf(
			"%w: workspace mutation observed job %d generation %d, current generation is %d",
			ErrStaleStepAttempt, jobID, generation, currentGeneration,
		)
	}

	operationIDs, err := currentWorkspaceMutationOperationIDs(
		ctx, tx, jobID, generation,
	)
	if err != nil {
		return nil, err
	}
	if len(operationIDs) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit empty current workspace mutation read: %w", err)
		}
		return nil, nil
	}
	if len(operationIDs) != 1 {
		return nil, fmt.Errorf(
			"%w: job %d generation %d has multiple workspace mutation operations",
			ErrWorkspaceMutationConflict, jobID, generation,
		)
	}

	command, record, err := loadCurrentWorkspaceMutationTx(
		ctx, tx, jobID, generation, operationIDs[0],
	)
	if err != nil {
		return nil, err
	}
	snapshot, err := validateCurrentWorkspaceMutationTx(ctx, tx, command, record)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit current workspace mutation read: %w", err)
	}
	return snapshot, nil
}

func currentWorkspaceMutationOperationIDs(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT id FROM workspace_mutation_operations
		WHERE job_id=$1 AND generation=$2
		ORDER BY id
		LIMIT 2
		FOR SHARE
	`, jobID, generation)
	if err != nil {
		return nil, fmt.Errorf("load current workspace mutation identities: %w", err)
	}
	defer rows.Close()
	operationIDs := make([]string, 0, 2)
	for rows.Next() {
		var operationID string
		if err := rows.Scan(&operationID); err != nil {
			return nil, fmt.Errorf("scan current workspace mutation identity: %w", err)
		}
		operationIDs = append(operationIDs, operationID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate current workspace mutation identities: %w", err)
	}
	return operationIDs, nil
}

func loadCurrentWorkspaceMutationTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
	operationID string,
) (WorkspaceMutationCommand, currentWorkspaceMutationRecord, error) {
	var command WorkspaceMutationCommand
	var record currentWorkspaceMutationRecord
	var sourceGit *string
	var gitRepositoryID, gitHeadCommit string
	err := tx.QueryRow(ctx, `
		SELECT operation.id,operation.command_sha256,operation.status,
		       operation.job_id,operation.step_id,operation.generation,
		       operation.creator_step_attempt,operation.creator_worker_id,operation.project_id,
		       operation.project_location,
		       operation.owner_id,operation.stage_id,operation.workspace_id,operation.workspace_root,
		       operation.source_state_id,operation.expected_state_id,
		       operation.source_repository_snapshot_id,operation.patch,operation.patch_sha256,
		       operation.verification_plan_json,operation.verification_plan_sha256,
		       COALESCE(snapshot.repository_id,''),COALESCE(snapshot.head_commit,''),
		       operation.mutation_evidence_id,operation.verification_succeeded,
		       operation.verification_receipt_json,operation.verification_receipt_sha256,
		       operation.verification_evidence_id,operation.verified_repository_snapshot_id,
		       operation.last_error
		FROM workspace_mutation_operations AS operation
		LEFT JOIN repository_snapshots AS snapshot
		  ON snapshot.project_id=operation.project_id AND
		     snapshot.id=operation.source_repository_snapshot_id
		WHERE operation.id=$1 AND operation.job_id=$2 AND operation.generation=$3
	`, operationID, jobID, generation).Scan(
		&record.operationID, &record.commandSHA256, &record.status,
		&command.JobID, &command.StepID, &command.Generation,
		&command.CreatorAttempt, &command.CreatorWorkerID, &command.ProjectID,
		&command.ProjectLocation,
		&command.Plan.OwnerID, &command.Plan.ID, &command.Plan.WorkspaceID,
		&command.Plan.WorkspaceRoot, &command.Plan.SourceStateID,
		&command.Plan.ExpectedStateID, &sourceGit, &command.Plan.Patch,
		&command.Plan.PatchSHA256, &record.verificationJSON,
		&record.verificationSHA256, &gitRepositoryID, &gitHeadCommit,
		&record.mutationEvidenceID, &record.verificationSucceeded,
		&record.verificationReceipt, &record.verificationReceiptSHA,
		&record.verificationEvidenceID, &record.verifiedRepositorySnapshotID,
		&record.lastError,
	)
	if err != nil {
		return WorkspaceMutationCommand{}, currentWorkspaceMutationRecord{},
			fmt.Errorf("load current workspace mutation command: %w", err)
	}
	command.Plan.Schema = workspacefacts.MutationPlanSchemaV1
	if sourceGit != nil {
		command.Plan.GitSourceSnapshotID = *sourceGit
		command.Plan.GitRepositoryID = gitRepositoryID
		command.Plan.GitHeadCommit = gitHeadCommit
	}
	if digestWorkspaceMutationText(record.verificationJSON) != record.verificationSHA256 {
		return WorkspaceMutationCommand{}, currentWorkspaceMutationRecord{},
			fmt.Errorf("durable workspace mutation verification plan digest differs")
	}
	if err := json.Unmarshal([]byte(record.verificationJSON), &command.Verification); err != nil {
		return WorkspaceMutationCommand{}, currentWorkspaceMutationRecord{},
			fmt.Errorf("decode current workspace mutation verification plan: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT file_id,path,
		       source_present,source_kind,source_sha256,source_size,source_mode,source_link_target,
		       expected_present,expected_kind,expected_sha256,expected_size,expected_mode,expected_link_target
		FROM workspace_mutation_files WHERE operation_id=$1 ORDER BY ordinal
	`, operationID)
	if err != nil {
		return WorkspaceMutationCommand{}, currentWorkspaceMutationRecord{},
			fmt.Errorf("load current workspace mutation files: %w", err)
	}
	command.Plan.Files = make([]workspacefacts.MutationFileTransition, 0, workspacefacts.MaxMutationFiles)
	for rows.Next() {
		file, scanErr := scanWorkspaceMutationFile(rows)
		if scanErr != nil {
			rows.Close()
			return WorkspaceMutationCommand{}, currentWorkspaceMutationRecord{}, scanErr
		}
		command.Plan.Files = append(command.Plan.Files, file)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return WorkspaceMutationCommand{}, currentWorkspaceMutationRecord{},
			fmt.Errorf("iterate current workspace mutation files: %w", err)
	}
	rows.Close()

	identity, err := workspaceMutationOperation(command)
	if err != nil {
		return WorkspaceMutationCommand{}, currentWorkspaceMutationRecord{},
			fmt.Errorf("validate current workspace mutation command: %w", err)
	}
	if identity.ID != record.operationID || identity.CommandSHA256 != record.commandSHA256 ||
		identity.PlanJSON != record.verificationJSON ||
		identity.PlanSHA256 != record.verificationSHA256 {
		return WorkspaceMutationCommand{}, currentWorkspaceMutationRecord{}, fmt.Errorf(
			"%w: durable workspace mutation %s differs from its command",
			ErrWorkspaceMutationConflict, record.operationID,
		)
	}
	return command, record, nil
}
