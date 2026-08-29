package queue

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
)

// prepareWorkspaceMutationBeforeProjectLocation installs one exact operation
// through the historical schema used by migration-prefix tests. Production
// mutation preparation has no pre-179 compatibility path.
func prepareWorkspaceMutationBeforeProjectLocation(
	t *testing.T,
	fixture workspaceMutationPipelineActionFixture,
) (workspaceMutationOperationRecord, error) {
	t.Helper()
	tx, err := fixture.commandDatabase().BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		return workspaceMutationOperationRecord{}, err
	}
	defer tx.Rollback(t.Context())
	command := fixture.command
	identity := fixture.identity
	var gitSource any
	if command.Plan.GitSourceSnapshotID != "" {
		gitSource = command.Plan.GitSourceSnapshotID
	}
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO workspace_mutation_operations (
			id,command_sha256,job_id,generation,step_id,
			creator_step_attempt,creator_worker_id,current_step_attempt,current_worker_id,
			project_id,owner_id,stage_id,workspace_id,workspace_root,
			source_state_id,expected_state_id,source_repository_snapshot_id,
			patch,patch_sha256,verification_plan_json,verification_plan_sha256,status
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
			$16,$17,$18,$19,$20
		)
	`, identity.ID, identity.CommandSHA256, command.JobID, command.Generation,
		command.StepID, command.CreatorAttempt, command.CreatorWorkerID, command.ProjectID,
		command.Plan.OwnerID, command.Plan.ID, command.Plan.WorkspaceID,
		command.Plan.WorkspaceRoot, command.Plan.SourceStateID, command.Plan.ExpectedStateID,
		gitSource, command.Plan.Patch, command.Plan.PatchSHA256, identity.PlanJSON,
		identity.PlanSHA256, workspaceMutationPrepared); err != nil {
		return workspaceMutationOperationRecord{}, err
	}
	for index, file := range command.Plan.Files {
		sourceKind, sourceSHA, sourceSize, sourceMode := workspaceMutationSQLState(
			file.Source.Present, file.Source.SHA256, file.Source.Size, file.Source.Mode,
		)
		expectedKind, expectedSHA, expectedSize, expectedMode := workspaceMutationSQLState(
			file.Expected.Present, file.Expected.SHA256, file.Expected.Size, file.Expected.Mode,
		)
		if _, err := tx.Exec(t.Context(), `
			INSERT INTO workspace_mutation_files (
				operation_id,ordinal,file_id,path,
				source_present,source_kind,source_sha256,source_size,source_mode,source_link_target,
				expected_present,expected_kind,expected_sha256,expected_size,expected_mode,expected_link_target
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULL,$10,$11,$12,$13,$14,NULL)
		`, identity.ID, index, file.FileID, file.Path,
			file.Source.Present, sourceKind, sourceSHA, sourceSize, sourceMode,
			file.Expected.Present, expectedKind, expectedSHA, expectedSize, expectedMode); err != nil {
			return workspaceMutationOperationRecord{}, err
		}
	}
	if _, err := tx.Exec(t.Context(), `
		UPDATE workspace_mutation_operations
		SET sealed_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1 AND status=$2 AND sealed_at IS NULL
	`, identity.ID, workspaceMutationPrepared); err != nil {
		return workspaceMutationOperationRecord{}, err
	}
	if err := tx.Commit(t.Context()); err != nil {
		return workspaceMutationOperationRecord{}, err
	}
	return workspaceMutationOperationRecord{
		ID: identity.ID, CommandSHA256: identity.CommandSHA256,
		ProjectLocation: command.ProjectLocation, Status: workspaceMutationPrepared,
	}, nil
}

// markWorkspaceMutationApplyingBeforeProjectLocation drives the historical
// transition schema exactly. Production transition loading requires the
// project_location column introduced by migration 179.
func markWorkspaceMutationApplyingBeforeProjectLocation(
	t *testing.T,
	fixture workspaceMutationPipelineActionFixture,
) error {
	t.Helper()
	tx, err := fixture.commandDatabase().BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin historical workspace mutation applying transition: %w", err)
	}
	defer tx.Rollback(t.Context())
	command := fixture.command
	identity := fixture.identity
	if err := lockWorkspaceMutationAuthorityTx(
		t.Context(), tx, fixture.claim.Authority, command, false,
	); err != nil {
		return err
	}
	record, err := lockWorkspaceMutationOperationBeforeProjectLocation(
		t, tx, identity.ID, command.ProjectLocation,
	)
	if err != nil {
		return err
	}
	if err := requireWorkspaceMutationIdentity(record, identity); err != nil {
		return err
	}
	var attemptCount int
	err = tx.QueryRow(t.Context(), `
		UPDATE workspace_mutation_operations
		SET status=$2,indeterminate_phase=NULL,last_error=NULL,
		    apply_attempt_count=apply_attempt_count+1,applying_at=clock_timestamp(),
		    current_step_attempt=$3,current_worker_id=$4,updated_at=clock_timestamp()
		WHERE id=$1 AND status IN ($5,$6,$7)
		RETURNING apply_attempt_count
	`, identity.ID, workspaceMutationApplying, fixture.claim.Authority.Attempt,
		fixture.claim.Authority.WorkerID, workspaceMutationPrepared,
		workspaceMutationApplying, workspaceMutationIndeterminateState).Scan(&attemptCount)
	if err != nil {
		return fmt.Errorf("mark historical workspace mutation applying: %w", err)
	}
	if attemptCount <= 0 {
		return fmt.Errorf("historical workspace mutation %s returned invalid apply attempt count", identity.ID)
	}
	if err := recordTelemetryJobEvent(
		t.Context(), tx, command.JobID, "workspace_mutation_applying", map[string]any{
			"operation_id": identity.ID, "workspace_id": command.Plan.WorkspaceID,
			"attempt_count": attemptCount,
		},
	); err != nil {
		return err
	}
	if err := tx.Commit(t.Context()); err != nil {
		return fmt.Errorf("commit historical workspace mutation applying transition: %w", err)
	}
	return nil
}

func lockWorkspaceMutationOperationBeforeProjectLocation(
	t *testing.T,
	tx pgx.Tx,
	operationID string,
	projectLocation string,
) (workspaceMutationOperationRecord, error) {
	t.Helper()
	var record workspaceMutationOperationRecord
	err := tx.QueryRow(t.Context(), `
		SELECT id,command_sha256,status,indeterminate_phase,mutation_evidence_id,
		       verification_succeeded,verification_receipt_json,verification_evidence_id,
		       verified_repository_snapshot_id
		FROM workspace_mutation_operations WHERE id=$1 FOR UPDATE
	`, operationID).Scan(
		&record.ID, &record.CommandSHA256, &record.Status, &record.IndeterminatePhase,
		&record.MutationEvidenceID, &record.VerificationSucceeded,
		&record.VerificationReceipt, &record.VerificationEvidenceID,
		&record.VerifiedRepositorySnapshotID,
	)
	record.ProjectLocation = projectLocation
	if err != nil {
		return workspaceMutationOperationRecord{}, fmt.Errorf(
			"lock historical workspace mutation operation %s: %w", operationID, err,
		)
	}
	return record, nil
}
