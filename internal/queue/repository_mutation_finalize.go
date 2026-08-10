package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) finalizeRepositoryMutation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command RepositoryMutationCommand,
	identity repositoryMutationOperationIdentity,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin repository mutation finalization: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockRepositoryMutationAuthorityTx(ctx, tx, authority, command, false); err != nil {
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
		if record.EvidenceID == nil || *record.EvidenceID <= 0 {
			return fmt.Errorf("applied repository mutation %s has no evidence identity", identity.ID)
		}
		if err := requireRepositoryMutationEvidenceTx(
			ctx, tx, command, identity.ID, *record.EvidenceID,
		); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	evidenceID, err := insertRepositoryMutationEvidenceTx(ctx, tx, command, identity.ID)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		UPDATE repository_mutation_operations
		SET status=$2, evidence_id=$3, last_error=NULL,
		    applied_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND status IN ($4,$5,$6)
	`, identity.ID, repositoryMutationApplied, evidenceID,
		repositoryMutationPrepared, repositoryMutationApplying,
		repositoryMutationIndeterminate)
	if err != nil {
		return fmt.Errorf("finalize repository mutation journal: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("repository mutation %s lost finalization authority", identity.ID)
	}
	if err := recordTelemetryJobEvent(ctx, tx, command.JobID, "repository_mutation_applied", map[string]any{
		"job_id": command.JobID, "step_id": command.StepID,
		"generation": command.Generation, "operation_id": identity.ID,
		"stage_id": command.StageID, "evidence_id": evidenceID,
		"changed_file_count": len(command.ChangedFiles),
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit repository mutation finalization: %w", err)
	}
	return nil
}

func requireRepositoryMutationEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	command RepositoryMutationCommand,
	operationID string,
	evidenceID int64,
) error {
	var kind, sourceType, sourceRef, patchHash, persistedOperation string
	err := tx.QueryRow(ctx, `
		SELECT kind, COALESCE(source_type, ''), COALESCE(source_ref, ''),
		       COALESCE(payload_json->>'hash', ''),
		       COALESCE(payload_json->'metadata'->>'repository_mutation_operation_id', '')
		FROM evidence
		WHERE id=$1 AND job_id=$2 AND step_id=$3
	`, evidenceID, command.JobID, command.StepID).Scan(
		&kind, &sourceType, &sourceRef, &patchHash, &persistedOperation,
	)
	if err != nil {
		return fmt.Errorf("load repository mutation evidence %d: %w", evidenceID, err)
	}
	if kind != evidence.KindGeneratedDiff || sourceType != "repository" ||
		sourceRef != command.StageID || patchHash != command.PatchSHA256 ||
		persistedOperation != operationID {
		return fmt.Errorf("repository mutation evidence %d disagrees with operation %s", evidenceID, operationID)
	}
	return nil
}
