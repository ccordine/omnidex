package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) GeneratedWorkloadVerification(
	ctx context.Context, id string,
) (*GeneratedWorkloadVerificationRecord, error) {
	if ctx == nil || !validSHA256ID(id, "generated_workload_verification_") {
		return nil, fmt.Errorf("load workspace verification requires context and exact receipt identity")
	}
	if r == nil || r.pool == nil {
		return nil, ErrRepositoryNotConfigured
	}
	record, found, err := loadGeneratedWorkloadVerificationByIDTx(ctx, r.pool, id)
	if err != nil || !found {
		return nil, err
	}
	return &record, nil
}

func (r *Repository) BoundGeneratedWorkloadVerification(
	ctx context.Context, jobID, generation int64,
) (*GeneratedWorkloadVerificationRecord, error) {
	if ctx == nil || jobID <= 0 || generation <= 0 || r == nil || r.pool == nil {
		return nil, fmt.Errorf("load bound workspace verification requires PostgreSQL and positive authority")
	}
	var id string
	err := r.pool.QueryRow(ctx, `
		SELECT binding.verification_id
		FROM generated_workload_deployments AS deployment
		JOIN generated_workload_deployment_verifications AS binding ON binding.operation_id=deployment.id
		WHERE deployment.job_id=$1 AND deployment.generation=$2
	`, jobID, generation).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.GeneratedWorkloadVerification(ctx, id)
}

type generatedWorkloadVerificationQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadGeneratedWorkloadVerificationByIDTx(
	ctx context.Context,
	querier generatedWorkloadVerificationQuerier,
	id string,
) (GeneratedWorkloadVerificationRecord, bool, error) {
	var record GeneratedWorkloadVerificationRecord
	var receiptJSON string
	err := querier.QueryRow(ctx, `
		SELECT id,receipt_sha256,receipt_json,job_id,generation,step_id,workspace_sha256,
		       command_evidence_ids,evidence_id,created_at
		FROM generated_workload_verifications WHERE id=$1
	`, id).Scan(
		&record.ID, &record.ReceiptSHA256, &receiptJSON, &record.JobID,
		&record.Generation, &record.StepID, &record.WorkspaceSHA256,
		&record.CommandEvidenceIDs, &record.EvidenceID, &record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GeneratedWorkloadVerificationRecord{}, false, nil
	}
	if err != nil {
		return GeneratedWorkloadVerificationRecord{}, false, fmt.Errorf("load workspace verification receipt: %w", err)
	}
	var receipt GeneratedWorkloadVerificationReceipt
	if err := decodeExactGeneratedDeploymentJSON(receiptJSON, &receipt); err != nil {
		return GeneratedWorkloadVerificationRecord{}, false, fmt.Errorf("decode workspace verification receipt: %w", err)
	}
	canonical, digest, canonicalID, err := canonicalGeneratedWorkloadVerification(receipt)
	ids := generatedWorkloadVerificationEvidenceIDs(receipt.Commands)
	if err != nil || canonical != receiptJSON || digest != record.ReceiptSHA256 || canonicalID != record.ID ||
		record.EvidenceID <= 0 || receipt.JobID != record.JobID || receipt.Generation != record.Generation ||
		receipt.StepID != record.StepID || receipt.WorkspaceSHA256 != record.WorkspaceSHA256 ||
		!equalGeneratedDeploymentEvidenceIDs(ids, record.CommandEvidenceIDs) {
		return GeneratedWorkloadVerificationRecord{}, false, fmt.Errorf("%w: durable workspace verification receipt differs", ErrGeneratedWorkloadDeploymentConflict)
	}
	record.Commands = append([]GeneratedWorkloadVerificationCommandProof(nil), receipt.Commands...)
	return record, true, nil
}

func generatedWorkloadVerificationEvidenceIDs(
	commands []GeneratedWorkloadVerificationCommandProof,
) []int64 {
	ids := make([]int64, len(commands))
	for index, command := range commands {
		ids[index] = command.EvidenceID
	}
	return ids
}

func equalGeneratedWorkloadVerificationCommands(
	left, right []GeneratedWorkloadVerificationCommandProof,
) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equivalentGeneratedWorkloadVerificationCommands(
	left, right []GeneratedWorkloadVerificationCommandProof,
) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Ordinal != right[index].Ordinal || left[index].Kind != right[index].Kind ||
			left[index].CommandSHA256 != right[index].CommandSHA256 {
			return false
		}
	}
	return true
}

func validateGeneratedDeploymentEvidenceIDs(ids []int64, minimum, maximum int) error {
	if len(ids) < minimum || len(ids) > maximum {
		return fmt.Errorf("evidence identity count must be %d-%d", minimum, maximum)
	}
	var previous int64
	for index, id := range ids {
		if id <= 0 || index > 0 && id <= previous {
			return fmt.Errorf("evidence identities must be positive, sorted, and unique")
		}
		previous = id
	}
	return nil
}

func equalGeneratedDeploymentEvidenceIDs(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
