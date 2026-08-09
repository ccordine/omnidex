package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const maxRepositoryMutationErrorBytes = 64 * 1024

func (r *Repository) prepareRepositoryMutation(
	ctx context.Context,
	command RepositoryMutationCommand,
	identity repositoryMutationOperationIdentity,
) (repositoryMutationOperationRecord, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return repositoryMutationOperationRecord{}, fmt.Errorf("begin repository mutation preparation: %w", err)
	}
	defer tx.Rollback(ctx)
	existing, found, err := loadRepositoryMutationByStageTx(ctx, tx, command.JobID, command.StageID)
	if err != nil {
		return repositoryMutationOperationRecord{}, err
	}
	if found {
		if err := requireRepositoryMutationIdentity(existing, identity); err != nil {
			return repositoryMutationOperationRecord{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return repositoryMutationOperationRecord{}, fmt.Errorf("commit repository mutation replay lock: %w", err)
		}
		return existing, nil
	}
	if err := lockRepositoryMutationAuthorityTx(ctx, tx, command); err != nil {
		return repositoryMutationOperationRecord{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO repository_mutation_operations (
			id, command_sha256, job_id, step_id, generation, worker_id,
			contract_id, stage_id, source_snapshot_id, patch, patch_sha256, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, identity.ID, identity.CommandSHA256, command.JobID, command.StepID,
		command.Generation, command.WorkerID, command.ContractID, command.StageID,
		command.SourceSnapshotID, command.Patch, command.PatchSHA256,
		repositoryMutationPrepared); err != nil {
		return repositoryMutationOperationRecord{}, fmt.Errorf("insert repository mutation preparation: %w", err)
	}
	for index, file := range command.ChangedFiles {
		if _, err := tx.Exec(ctx, `
			INSERT INTO repository_mutation_files (
				operation_id, ordinal, file_id, path, source_sha256, source_size,
				expected_sha256, expected_size
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		`, identity.ID, index, file.FileID, file.Path, file.SourceSHA256,
			file.SourceSize, file.ExpectedSHA256, file.ExpectedSize); err != nil {
			return repositoryMutationOperationRecord{}, fmt.Errorf(
				"insert repository mutation file %d: %w", index, err,
			)
		}
	}
	sealed, err := tx.Exec(ctx, `
		UPDATE repository_mutation_operations
		SET sealed_at=NOW()
		WHERE id=$1 AND status=$2 AND attempt_count=0 AND sealed_at IS NULL
	`, identity.ID, repositoryMutationPrepared)
	if err != nil {
		return repositoryMutationOperationRecord{}, fmt.Errorf("seal repository mutation file authority: %w", err)
	}
	if sealed.RowsAffected() != 1 {
		return repositoryMutationOperationRecord{}, fmt.Errorf(
			"repository mutation %s lost file sealing authority", identity.ID,
		)
	}
	if err := recordTelemetryJobEvent(ctx, tx, command.JobID, "repository_mutation_prepared", map[string]any{
		"job_id": command.JobID, "step_id": command.StepID,
		"generation": command.Generation, "operation_id": identity.ID,
		"stage_id": command.StageID, "changed_file_count": len(command.ChangedFiles),
	}); err != nil {
		return repositoryMutationOperationRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repositoryMutationOperationRecord{}, fmt.Errorf("commit repository mutation preparation: %w", err)
	}
	return repositoryMutationOperationRecord{
		ID: identity.ID, CommandSHA256: identity.CommandSHA256,
		Status: repositoryMutationPrepared,
	}, nil
}

func (r *Repository) markRepositoryMutationApplying(
	ctx context.Context,
	command RepositoryMutationCommand,
	identity repositoryMutationOperationIdentity,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin repository mutation applying transition: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockRepositoryMutationAuthorityTx(ctx, tx, command); err != nil {
		return err
	}
	record, err := lockRepositoryMutationOperationTx(ctx, tx, identity.ID)
	if err != nil {
		return err
	}
	if err := requireRepositoryMutationIdentity(record, identity); err != nil {
		return err
	}
	if record.Status == repositoryMutationApplied {
		return fmt.Errorf("%w: repository mutation %s is already applied", ErrRepositoryMutationConflict, identity.ID)
	}
	var attemptCount int
	err = tx.QueryRow(ctx, `
		UPDATE repository_mutation_operations
		SET status=$2, attempt_count=attempt_count+1, last_error=NULL, updated_at=NOW()
		WHERE id=$1 AND status IN ($3,$4,$5)
		RETURNING attempt_count
	`, identity.ID, repositoryMutationApplying, repositoryMutationPrepared,
		repositoryMutationApplying, repositoryMutationIndeterminate).Scan(&attemptCount)
	if err != nil {
		return fmt.Errorf("mark repository mutation applying: %w", err)
	}
	if attemptCount <= 0 {
		return fmt.Errorf("repository mutation %s returned invalid attempt count %d", identity.ID, attemptCount)
	}
	if err := recordTelemetryJobEvent(ctx, tx, command.JobID, "repository_mutation_applying", map[string]any{
		"job_id": command.JobID, "step_id": command.StepID,
		"generation": command.Generation, "operation_id": identity.ID,
		"stage_id": command.StageID, "attempt_count": attemptCount,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit repository mutation applying transition: %w", err)
	}
	return nil
}

func (r *Repository) recordRepositoryMutationState(
	ctx context.Context,
	command RepositoryMutationCommand,
	identity repositoryMutationOperationIdentity,
	status string,
	reason error,
) error {
	message, err := exactRepositoryMutationError(reason)
	if err != nil {
		return err
	}
	if status != repositoryMutationPrepared && status != repositoryMutationIndeterminate {
		return fmt.Errorf("repository mutation state transition target %q is invalid", status)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin repository mutation state transition: %w", err)
	}
	defer tx.Rollback(ctx)
	record, err := lockRepositoryMutationOperationTx(ctx, tx, identity.ID)
	if err != nil {
		return err
	}
	if err := requireRepositoryMutationIdentity(record, identity); err != nil {
		return err
	}
	if record.Status == repositoryMutationApplied {
		return fmt.Errorf("%w: repository mutation %s is already applied", ErrRepositoryMutationConflict, identity.ID)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE repository_mutation_operations
		SET status=$2, last_error=$3, updated_at=NOW()
		WHERE id=$1
	`, identity.ID, status, message); err != nil {
		return fmt.Errorf("record repository mutation state %q: %w", status, err)
	}
	eventType := "repository_mutation_indeterminate"
	if status == repositoryMutationPrepared {
		eventType = "repository_mutation_deferred"
	}
	if err := recordTelemetryJobEvent(ctx, tx, command.JobID,
		eventType, map[string]any{
			"job_id": command.JobID, "step_id": command.StepID,
			"generation": command.Generation, "operation_id": identity.ID,
			"stage_id": command.StageID, "status": status,
			"error_sha256": repositoryMutationFailureSHA(message),
		}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit repository mutation state %q: %w", status, err)
	}
	return nil
}

func repositoryMutationFailureSHA(message string) string {
	digest := sha256.Sum256([]byte(message))
	return hex.EncodeToString(digest[:])
}

func exactRepositoryMutationError(source error) (string, error) {
	if source == nil {
		return "", fmt.Errorf("repository mutation state transition requires one exact failure")
	}
	message := source.Error()
	if message == "" || message != strings.TrimSpace(message) || !utf8.ValidString(message) ||
		strings.ContainsRune(message, '\x00') || len(message) > maxRepositoryMutationErrorBytes {
		return "", fmt.Errorf("repository mutation failure is not exact bounded PostgreSQL text: %w", source)
	}
	return message, nil
}

func loadRepositoryMutationByStageTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	stageID string,
) (repositoryMutationOperationRecord, bool, error) {
	record, err := scanRepositoryMutationOperation(tx.QueryRow(ctx, `
		SELECT id, command_sha256, status, evidence_id
		FROM repository_mutation_operations
		WHERE job_id=$1 AND stage_id=$2
		FOR UPDATE
	`, jobID, stageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return repositoryMutationOperationRecord{}, false, nil
	}
	if err != nil {
		return repositoryMutationOperationRecord{}, false, fmt.Errorf("load repository mutation stage: %w", err)
	}
	return record, true, nil
}

func lockRepositoryMutationOperationTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID string,
) (repositoryMutationOperationRecord, error) {
	record, err := scanRepositoryMutationOperation(tx.QueryRow(ctx, `
		SELECT id, command_sha256, status, evidence_id
		FROM repository_mutation_operations WHERE id=$1 FOR UPDATE
	`, operationID))
	if err != nil {
		return repositoryMutationOperationRecord{}, fmt.Errorf("lock repository mutation operation %s: %w", operationID, err)
	}
	return record, nil
}

func scanRepositoryMutationOperation(row pgx.Row) (repositoryMutationOperationRecord, error) {
	var record repositoryMutationOperationRecord
	err := row.Scan(&record.ID, &record.CommandSHA256, &record.Status, &record.EvidenceID)
	return record, err
}

func requireRepositoryMutationIdentity(
	record repositoryMutationOperationRecord,
	identity repositoryMutationOperationIdentity,
) error {
	if record.ID != identity.ID || record.CommandSHA256 != identity.CommandSHA256 {
		return fmt.Errorf("%w: persisted repository mutation command differs", ErrRepositoryMutationConflict)
	}
	return nil
}
