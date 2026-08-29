package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) prepareWorkspaceMutation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	identity workspaceMutationOperationIdentity,
) (workspaceMutationOperationRecord, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return workspaceMutationOperationRecord{}, fmt.Errorf("begin workspace mutation preparation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockWorkspaceMutationProjectAuthorityTx(ctx, tx, command); err != nil {
		return workspaceMutationOperationRecord{}, err
	}
	record, found, err := loadWorkspaceMutationByStageTx(ctx, tx, command.JobID, command.Plan.ID)
	if err != nil {
		return workspaceMutationOperationRecord{}, err
	}
	if found {
		if err := requireWorkspaceMutationIdentity(record, identity); err != nil {
			return workspaceMutationOperationRecord{}, err
		}
		if err := lockWorkspaceMutationAuthorityTx(ctx, tx, authority, command, false); err != nil {
			return workspaceMutationOperationRecord{}, err
		}
		if record.Status != workspaceMutationApplying &&
			record.Status != workspaceMutationVerifying &&
			record.Status != workspaceMutationVerified &&
			record.Status != workspaceMutationVerificationFailed {
			if _, err := tx.Exec(ctx, `
				UPDATE workspace_mutation_operations
				SET current_step_attempt=$2,current_worker_id=$3,updated_at=clock_timestamp()
				WHERE id=$1
			`, identity.ID, authority.Attempt, authority.WorkerID); err != nil {
				return workspaceMutationOperationRecord{}, fmt.Errorf("refresh workspace mutation recovery authority: %w", err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return workspaceMutationOperationRecord{}, fmt.Errorf("commit workspace mutation replay lock: %w", err)
		}
		return record, nil
	}
	if err := lockWorkspaceMutationAuthorityTx(ctx, tx, authority, command, true); err != nil {
		return workspaceMutationOperationRecord{}, err
	}
	var gitSource any
	if command.Plan.GitSourceSnapshotID != "" {
		gitSource = command.Plan.GitSourceSnapshotID
	}
	inserted, err := tx.Exec(ctx, `
		WITH project_authority AS MATERIALIZED (
			SELECT project.location AS project_location
			FROM jobs AS job
			JOIN projects AS project ON project.id=job.project_id
			WHERE job.id=$3 AND job.project_id=$8 AND project.location=$21
			FOR SHARE OF project
		)
		INSERT INTO workspace_mutation_operations (
			id,command_sha256,job_id,generation,step_id,
			creator_step_attempt,creator_worker_id,current_step_attempt,current_worker_id,
			project_id,project_location,owner_id,stage_id,workspace_id,workspace_root,
			source_state_id,expected_state_id,source_repository_snapshot_id,
			patch,patch_sha256,verification_plan_json,verification_plan_sha256,status
		)
		SELECT $1,$2,$3,$4,$5,$6,$7,$6,$7,$8,project_location,
		       $9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20
		FROM project_authority
	`, identity.ID, identity.CommandSHA256, command.JobID, command.Generation,
		command.StepID, command.CreatorAttempt, command.CreatorWorkerID, command.ProjectID,
		command.Plan.OwnerID, command.Plan.ID, command.Plan.WorkspaceID, command.Plan.WorkspaceRoot,
		command.Plan.SourceStateID, command.Plan.ExpectedStateID, gitSource,
		command.Plan.Patch, command.Plan.PatchSHA256, identity.PlanJSON, identity.PlanSHA256,
		workspaceMutationPrepared, command.ProjectLocation)
	if err != nil {
		return workspaceMutationOperationRecord{}, fmt.Errorf("insert workspace mutation preparation: %w", err)
	}
	if inserted.RowsAffected() != 1 {
		return workspaceMutationOperationRecord{}, fmt.Errorf(
			"%w: workspace mutation project authority differs from its current job",
			ErrWorkspaceMutationConflict,
		)
	}
	for index, file := range command.Plan.Files {
		sourceKind, sourceSHA, sourceSize, sourceMode := workspaceMutationSQLState(
			file.Source.Present, file.Source.SHA256, file.Source.Size, file.Source.Mode,
		)
		expectedKind, expectedSHA, expectedSize, expectedMode := workspaceMutationSQLState(
			file.Expected.Present, file.Expected.SHA256, file.Expected.Size, file.Expected.Mode,
		)
		if _, err := tx.Exec(ctx, `
			INSERT INTO workspace_mutation_files (
				operation_id,ordinal,file_id,path,
				source_present,source_kind,source_sha256,source_size,source_mode,source_link_target,
				expected_present,expected_kind,expected_sha256,expected_size,expected_mode,expected_link_target
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULL,$10,$11,$12,$13,$14,NULL)
		`, identity.ID, index, file.FileID, file.Path,
			file.Source.Present, sourceKind, sourceSHA, sourceSize, sourceMode,
			file.Expected.Present, expectedKind, expectedSHA, expectedSize, expectedMode); err != nil {
			return workspaceMutationOperationRecord{}, fmt.Errorf("insert workspace mutation file %d: %w", index, err)
		}
	}
	sealed, err := tx.Exec(ctx, `
		UPDATE workspace_mutation_operations
		SET sealed_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1 AND status=$2 AND sealed_at IS NULL
	`, identity.ID, workspaceMutationPrepared)
	if err != nil {
		return workspaceMutationOperationRecord{}, fmt.Errorf("seal workspace mutation file authority: %w", err)
	}
	if sealed.RowsAffected() != 1 {
		return workspaceMutationOperationRecord{}, fmt.Errorf("workspace mutation %s lost file sealing authority", identity.ID)
	}
	if err := recordTelemetryJobEvent(ctx, tx, command.JobID, "workspace_mutation_prepared", map[string]any{
		"operation_id": identity.ID, "workspace_id": command.Plan.WorkspaceID,
		"stage_id": command.Plan.ID, "changed_file_count": len(command.Plan.Files),
	}); err != nil {
		return workspaceMutationOperationRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return workspaceMutationOperationRecord{}, fmt.Errorf("commit workspace mutation preparation: %w", err)
	}
	return workspaceMutationOperationRecord{
		ID: identity.ID, CommandSHA256: identity.CommandSHA256,
		ProjectLocation: command.ProjectLocation, Status: workspaceMutationPrepared,
	}, nil
}

func loadWorkspaceMutationByStageTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	stageID string,
) (workspaceMutationOperationRecord, bool, error) {
	record, err := scanWorkspaceMutationOperation(tx.QueryRow(ctx, `
		SELECT id,command_sha256,project_location,status,indeterminate_phase,mutation_evidence_id,
		       verification_succeeded,verification_receipt_json,verification_evidence_id,
		       verified_repository_snapshot_id
		FROM workspace_mutation_operations
		WHERE job_id=$1 AND stage_id=$2
		FOR UPDATE
	`, jobID, stageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return workspaceMutationOperationRecord{}, false, nil
	}
	if err != nil {
		return workspaceMutationOperationRecord{}, false, fmt.Errorf("load workspace mutation stage: %w", err)
	}
	return record, true, nil
}

func lockWorkspaceMutationOperationTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID string,
) (workspaceMutationOperationRecord, error) {
	record, err := scanWorkspaceMutationOperation(tx.QueryRow(ctx, `
		SELECT id,command_sha256,project_location,status,indeterminate_phase,mutation_evidence_id,
		       verification_succeeded,verification_receipt_json,verification_evidence_id,
		       verified_repository_snapshot_id
		FROM workspace_mutation_operations WHERE id=$1 FOR UPDATE
	`, operationID))
	if err != nil {
		return workspaceMutationOperationRecord{}, fmt.Errorf("lock workspace mutation operation %s: %w", operationID, err)
	}
	return record, nil
}

func scanWorkspaceMutationOperation(row pgx.Row) (workspaceMutationOperationRecord, error) {
	var record workspaceMutationOperationRecord
	err := row.Scan(
		&record.ID, &record.CommandSHA256, &record.ProjectLocation,
		&record.Status, &record.IndeterminatePhase,
		&record.MutationEvidenceID, &record.VerificationSucceeded,
		&record.VerificationReceipt, &record.VerificationEvidenceID,
		&record.VerifiedRepositorySnapshotID,
	)
	return record, err
}

func requireWorkspaceMutationIdentity(
	record workspaceMutationOperationRecord,
	identity workspaceMutationOperationIdentity,
) error {
	if record.ID != identity.ID || record.CommandSHA256 != identity.CommandSHA256 ||
		record.ProjectLocation != identity.ProjectLocation {
		return fmt.Errorf("%w: persisted workspace mutation command differs", ErrWorkspaceMutationConflict)
	}
	return nil
}
