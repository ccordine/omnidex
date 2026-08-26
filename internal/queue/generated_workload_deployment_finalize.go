package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) SealGeneratedWorkloadDeploymentApplied(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	receipt GeneratedWorkloadDeploymentReceipt,
) (GeneratedWorkloadDeploymentRecord, error) {
	if ctx == nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("seal generated deployment requires a context")
	}
	if err := ctx.Err(); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("seal generated deployment: %w", err)
	}
	if r == nil || r.pool == nil {
		return GeneratedWorkloadDeploymentRecord{}, ErrRepositoryNotConfigured
	}
	if err := validateGeneratedDeploymentExecutionAuthority(authority, command); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("begin generated deployment seal: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	record, err := lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if err := requireGeneratedDeploymentIdentity(record, identity); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if err := lockAndValidateGeneratedDeploymentEvidenceRailTx(
		ctx, tx, command, identity.OperationID, record.Current, receipt,
	); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	receiptJSON, receiptSHA, err := canonicalGeneratedWorkloadDeploymentReceipt(command, receipt, identity)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if record.State == GeneratedWorkloadDeploymentApplied {
		if record.ReceiptSHA256 != receiptSHA || record.EvidenceID <= 0 {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf(
				"%w: applied generated deployment receipt differs", ErrGeneratedWorkloadDeploymentConflict,
			)
		}
		if err := requireGeneratedDeploymentEvidenceTx(
			ctx, tx, command, identity.OperationID, receiptJSON, receiptSHA, record.EvidenceID,
		); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, err
		}
		if _, err := sealGeneratedWorkloadProjectDeploymentHeadTx(
			ctx, tx, authority, command, receipt,
		); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("commit generated deployment seal replay: %w", err)
		}
		return record, nil
	}
	if record.State != GeneratedWorkloadDeploymentApplying {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf(
			"%w: cannot seal healthy receipt from %s", ErrGeneratedWorkloadDeploymentState, record.State,
		)
	}
	evidenceID, err := insertGeneratedDeploymentEvidenceTx(
		ctx, tx, command, receipt, receiptJSON, receiptSHA,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE generated_workload_deployments
		SET status='applied',terminal_code=NULL,terminal_detail_sha256=NULL,
		    receipt_json=$2,receipt_sha256=$3,evidence_id=$4,
		    healthy_endpoint_port=$5,applied_at=$6,observed_at=$7,
		    current_step_attempt=$8,current_worker_id=$9,
		    terminal_at=NULL,updated_at=clock_timestamp()
		WHERE id=$1
	`, identity.OperationID, receiptJSON, receiptSHA, evidenceID,
		receipt.EndpointPort, receipt.AppliedAt, receipt.ObservedAt,
		authority.Attempt, authority.WorkerID)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("seal generated deployment receipt: %w", err)
	}
	record, err = lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if _, err := sealGeneratedWorkloadProjectDeploymentHeadTx(
		ctx, tx, authority, command, receipt,
	); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("commit generated deployment seal: %w", err)
	}
	return record, nil
}
