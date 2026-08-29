package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const generatedWorkloadVerificationEvidenceSource = "workspace_verification"

func (r *Repository) RecordGeneratedWorkloadVerification(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	workspaceSHA256 string,
	evidenceIDs []int64,
) (GeneratedWorkloadVerificationRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return GeneratedWorkloadVerificationRecord{}, fmt.Errorf("record workspace verification requires PostgreSQL and context")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadVerificationRecord{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedVerificationAuthorityTx(ctx, tx, authority); err != nil {
		return GeneratedWorkloadVerificationRecord{}, err
	}
	proofs, err := lockGeneratedVerificationEvidenceTx(
		ctx, tx, authority.JobID, authority.StepID, evidenceIDs,
	)
	if err != nil {
		return GeneratedWorkloadVerificationRecord{}, err
	}
	receipt := GeneratedWorkloadVerificationReceipt{
		Schema: GeneratedWorkloadVerificationReceiptV1,
		JobID:  authority.JobID, Generation: authority.Generation, StepID: authority.StepID,
		WorkspaceSHA256: workspaceSHA256, Commands: proofs,
	}
	receiptJSON, receiptSHA, id, err := canonicalGeneratedWorkloadVerification(receipt)
	if err != nil {
		return GeneratedWorkloadVerificationRecord{}, err
	}
	existing, found, err := loadGeneratedWorkloadVerificationByIDTx(ctx, tx, id)
	if err != nil {
		return GeneratedWorkloadVerificationRecord{}, err
	}
	if found {
		if existing.ReceiptSHA256 != receiptSHA || existing.WorkspaceSHA256 != workspaceSHA256 ||
			!equalGeneratedWorkloadVerificationCommands(existing.Commands, proofs) {
			return GeneratedWorkloadVerificationRecord{}, fmt.Errorf("%w: workspace verification receipt differs", ErrGeneratedWorkloadDeploymentConflict)
		}
		if err := tx.Commit(ctx); err != nil {
			return GeneratedWorkloadVerificationRecord{}, err
		}
		return existing, nil
	}
	payload, err := generatedWorkloadVerificationEvidencePayload(receipt, id, receiptJSON, receiptSHA)
	if err != nil {
		return GeneratedWorkloadVerificationRecord{}, err
	}
	var aggregateEvidenceID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,$3,$4,$5,$6::jsonb) RETURNING id
	`, authority.JobID, authority.StepID, evidence.KindWorkspaceVerification,
		generatedWorkloadVerificationEvidenceSource, id, payload).Scan(&aggregateEvidenceID); err != nil {
		return GeneratedWorkloadVerificationRecord{}, fmt.Errorf("insert workspace verification evidence: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO generated_workload_verifications(
		 id,receipt_sha256,receipt_json,job_id,generation,step_id,workspace_sha256,
		 command_evidence_ids,evidence_id,creator_step_attempt,creator_worker_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, id, receiptSHA, receiptJSON, authority.JobID, authority.Generation,
		authority.StepID, workspaceSHA256, evidenceIDs, aggregateEvidenceID,
		authority.Attempt, authority.WorkerID)
	if err != nil {
		return GeneratedWorkloadVerificationRecord{}, fmt.Errorf("insert workspace verification receipt: %w", err)
	}
	record, found, err := loadGeneratedWorkloadVerificationByIDTx(ctx, tx, id)
	if err != nil || !found {
		return GeneratedWorkloadVerificationRecord{}, fmt.Errorf("reload workspace verification receipt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadVerificationRecord{}, err
	}
	return record, nil
}

func canonicalGeneratedWorkloadVerification(
	receipt GeneratedWorkloadVerificationReceipt,
) (string, string, string, error) {
	if receipt.Schema != GeneratedWorkloadVerificationReceiptV1 || receipt.JobID <= 0 ||
		receipt.Generation <= 0 || receipt.StepID <= 0 ||
		!validSHA256Digest(receipt.WorkspaceSHA256) || len(receipt.Commands) < 1 ||
		len(receipt.Commands) > MaxGeneratedWorkloadVerificationEvidence {
		return "", "", "", fmt.Errorf("workspace verification receipt authority is invalid")
	}
	for index, command := range receipt.Commands {
		if command.Ordinal != index+1 || command.EvidenceID <= 0 ||
			!validSHA256Digest(command.CommandSHA256) ||
			(command.Kind != evidence.KindCommandOutput && command.Kind != evidence.KindTestResult) ||
			(index > 0 && command.EvidenceID <= receipt.Commands[index-1].EvidenceID) {
			return "", "", "", fmt.Errorf("workspace verification command proof %d is invalid", index)
		}
	}
	encoded, err := canonicalGeneratedDeploymentJSON(receipt)
	if err != nil || len(encoded) > 32768 {
		return "", "", "", fmt.Errorf("workspace verification receipt exceeds canonical bound: %w", err)
	}
	digest := generatedDeploymentSHA(encoded)
	return encoded, digest, "generated_workload_verification_" + digest, nil
}

func generatedWorkloadVerificationEvidencePayload(
	receipt GeneratedWorkloadVerificationReceipt,
	id, receiptJSON, receiptSHA string,
) (string, error) {
	payload := evidence.Record{
		JobID: receipt.JobID, StepID: receipt.StepID, Kind: evidence.KindWorkspaceVerification,
		SourceType: generatedWorkloadVerificationEvidenceSource, SourceRef: id,
		Excerpt: receiptJSON, Summary: "Verified one exact generated workspace with code-owned commands.",
		Hash: receiptSHA, Confidence: 1,
		Metadata: map[string]any{
			"workspace_sha256": receipt.WorkspaceSHA256,
			"commands":         receipt.Commands, "succeeded": true,
		},
	}
	encoded, err := json.Marshal(payload)
	return string(encoded), err
}

func lockGeneratedVerificationAuthorityTx(
	ctx context.Context, tx pgx.Tx, authority model.StepAttemptAuthority,
) error {
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return fmt.Errorf("%w: workspace verification attempt is not running", ErrStaleStepAttempt)
	}
	return nil
}

func lockGeneratedVerificationEvidenceTx(
	ctx context.Context, tx pgx.Tx, jobID, stepID int64, evidenceIDs []int64,
) ([]GeneratedWorkloadVerificationCommandProof, error) {
	if err := validateGeneratedDeploymentEvidenceIDs(
		evidenceIDs, 1, MaxGeneratedWorkloadVerificationEvidence,
	); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id,kind,COALESCE(payload_json->>'command',''),
		       COALESCE(payload_json->'metadata'->>'succeeded','')
		FROM evidence WHERE id=ANY($1::bigint[]) AND job_id=$2 AND step_id=$3
		ORDER BY id FOR KEY SHARE
	`, evidenceIDs, jobID, stepID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	proofs := make([]GeneratedWorkloadVerificationCommandProof, 0, len(evidenceIDs))
	for rows.Next() {
		var id int64
		var kind, command, succeeded string
		if err := rows.Scan(&id, &kind, &command, &succeeded); err != nil {
			return nil, err
		}
		index := len(proofs)
		if index >= len(evidenceIDs) || id != evidenceIDs[index] || command == "" ||
			(kind != evidence.KindCommandOutput && kind != evidence.KindTestResult) || succeeded != "true" {
			return nil, fmt.Errorf("workspace verification evidence %d is not exact successful command evidence", id)
		}
		proofs = append(proofs, GeneratedWorkloadVerificationCommandProof{
			Ordinal: index + 1, EvidenceID: id, Kind: kind,
			CommandSHA256: generatedDeploymentSHA(command),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(proofs) != len(evidenceIDs) {
		return nil, fmt.Errorf("workspace verification evidence set is incomplete")
	}
	return proofs, nil
}
