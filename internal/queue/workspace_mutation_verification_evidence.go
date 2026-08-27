package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type workspaceMutationVerificationReceipt struct {
	Schema             string  `json:"schema"`
	OperationID        string  `json:"operation_id"`
	SourceStateID      string  `json:"source_state_id"`
	ExpectedStateID    string  `json:"expected_state_id"`
	ObservedStateID    string  `json:"observed_state_id"`
	Succeeded          bool    `json:"succeeded"`
	CommandEvidenceIDs []int64 `json:"command_evidence_ids"`
}

func (r *Repository) finalizeWorkspaceMutationVerification(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	identity workspaceMutationOperationIdentity,
	verification WorkspaceMutationVerificationResult,
) (WorkspaceMutationResult, error) {
	if err := validateWorkspaceMutationVerificationResult(command, identity.ID, verification); err != nil {
		return WorkspaceMutationResult{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WorkspaceMutationResult{}, fmt.Errorf("begin workspace verification finalization: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockWorkspaceMutationAuthorityTx(ctx, tx, authority, command, false); err != nil {
		return WorkspaceMutationResult{}, err
	}
	record, err := lockWorkspaceMutationOperationTx(ctx, tx, identity.ID)
	if err != nil {
		return WorkspaceMutationResult{}, err
	}
	if err := requireWorkspaceMutationIdentity(record, identity); err != nil {
		return WorkspaceMutationResult{}, err
	}
	if record.Status == workspaceMutationVerified || record.Status == workspaceMutationVerificationFailed {
		return workspaceMutationTerminalResult(ctx, tx, record)
	}
	if record.Status != workspaceMutationVerifying || record.MutationEvidenceID == nil {
		return WorkspaceMutationResult{}, fmt.Errorf("workspace mutation %s is not ready for terminal verification", identity.ID)
	}
	commandEvidenceIDs := make([]int64, len(verification.CommandEvidence))
	for index, raw := range verification.CommandEvidence {
		record := raw
		record.SourceType = "workspace_verification"
		record.SourceRef = identity.ID
		record.Metadata = cloneWorkspaceMutationMetadata(raw.Metadata)
		payload, err := json.Marshal(record)
		if err != nil {
			return WorkspaceMutationResult{}, fmt.Errorf("encode workspace verification evidence %d: %w", index+1, err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
			VALUES($1,$2,$3,$4,$5,$6::jsonb) RETURNING id
		`, command.JobID, command.StepID, record.Kind, record.SourceType,
			record.SourceRef, string(payload)).Scan(&commandEvidenceIDs[index]); err != nil {
			return WorkspaceMutationResult{}, fmt.Errorf("insert workspace verification evidence %d: %w", index+1, err)
		}
		if commandEvidenceIDs[index] <= 0 || index > 0 && commandEvidenceIDs[index] <= commandEvidenceIDs[index-1] {
			return WorkspaceMutationResult{}, fmt.Errorf("workspace verification evidence identities are not strictly increasing")
		}
	}
	receipt := workspaceMutationVerificationReceipt{
		Schema: workspaceMutationReceiptSchema, OperationID: identity.ID,
		SourceStateID:   command.Plan.SourceStateID,
		ExpectedStateID: command.Plan.ExpectedStateID,
		ObservedStateID: command.Plan.ExpectedStateID,
		Succeeded:       verification.Succeeded, CommandEvidenceIDs: commandEvidenceIDs,
	}
	receiptRaw, err := json.Marshal(receipt)
	if err != nil {
		return WorkspaceMutationResult{}, fmt.Errorf("encode workspace verification receipt: %w", err)
	}
	receiptJSON := string(receiptRaw)
	receiptSHA := digestWorkspaceMutationText(receiptJSON)
	receiptRecord := evidence.Record{
		JobID: command.JobID, StepID: command.StepID,
		Kind:       evidence.KindWorkspaceVerification,
		SourceType: "workspace_mutation", SourceRef: identity.ID,
		Hash: receiptSHA, Excerpt: receiptJSON,
		Summary:    fmt.Sprintf("Workspace mutation verification succeeded=%t.", verification.Succeeded),
		Confidence: 1,
		Metadata: map[string]any{
			"workspace_mutation_operation_id": identity.ID,
			"observed_state_id":               command.Plan.ExpectedStateID,
			"succeeded":                       verification.Succeeded,
		},
	}
	payload, err := json.Marshal(receiptRecord)
	if err != nil {
		return WorkspaceMutationResult{}, fmt.Errorf("encode workspace verification receipt evidence: %w", err)
	}
	var receiptEvidenceID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,$3,$4,$5,$6::jsonb) RETURNING id
	`, command.JobID, command.StepID, receiptRecord.Kind, receiptRecord.SourceType,
		receiptRecord.SourceRef, string(payload)).Scan(&receiptEvidenceID); err != nil {
		return WorkspaceMutationResult{}, fmt.Errorf("insert workspace verification receipt evidence: %w", err)
	}
	status := workspaceMutationVerificationFailed
	var lastError any = verification.Failure
	if verification.Succeeded {
		status = workspaceMutationVerified
		lastError = nil
	}
	var verifiedGit any
	if verification.VerifiedRepositorySnapshotID != "" {
		verifiedGit = verification.VerifiedRepositorySnapshotID
	}
	result, err := tx.Exec(ctx, `
		UPDATE workspace_mutation_operations
		SET status=$2,verification_succeeded=$3,verification_receipt_json=$4,
		    verification_receipt_sha256=$5,verification_evidence_id=$6,
		    verified_repository_snapshot_id=$7,last_error=$8,terminal_at=clock_timestamp(),
		    current_step_attempt=$9,current_worker_id=$10,updated_at=clock_timestamp()
		WHERE id=$1 AND status=$11
	`, identity.ID, status, verification.Succeeded, receiptJSON, receiptSHA,
		receiptEvidenceID, verifiedGit, lastError, authority.Attempt, authority.WorkerID,
		workspaceMutationVerifying)
	if err != nil {
		return WorkspaceMutationResult{}, fmt.Errorf("seal workspace mutation verification: %w", err)
	}
	if result.RowsAffected() != 1 {
		return WorkspaceMutationResult{}, fmt.Errorf("workspace mutation %s lost verification finalization authority", identity.ID)
	}
	if err := recordTelemetryJobEvent(ctx, tx, command.JobID, "workspace_mutation_verified", map[string]any{
		"operation_id": identity.ID, "succeeded": verification.Succeeded,
		"command_evidence_count": len(commandEvidenceIDs),
	}); err != nil {
		return WorkspaceMutationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return WorkspaceMutationResult{}, fmt.Errorf("commit workspace mutation verification: %w", err)
	}
	return WorkspaceMutationResult{
		OperationID: identity.ID, Status: status,
		MutationEvidenceID:     *record.MutationEvidenceID,
		CommandEvidenceIDs:     append([]int64(nil), commandEvidenceIDs...),
		VerificationEvidenceID: receiptEvidenceID,
		VerificationSucceeded:  verification.Succeeded,
	}, nil
}

func cloneWorkspaceMutationMetadata(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func workspaceMutationTerminalResult(
	ctx context.Context,
	tx pgx.Tx,
	record workspaceMutationOperationRecord,
) (WorkspaceMutationResult, error) {
	if record.MutationEvidenceID == nil || record.VerificationSucceeded == nil ||
		record.VerificationReceipt == nil || record.VerificationEvidenceID == nil {
		return WorkspaceMutationResult{}, fmt.Errorf("terminal workspace mutation %s is incomplete", record.ID)
	}
	var receipt workspaceMutationVerificationReceipt
	if err := json.Unmarshal([]byte(*record.VerificationReceipt), &receipt); err != nil {
		return WorkspaceMutationResult{}, fmt.Errorf("decode terminal workspace mutation receipt: %w", err)
	}
	if receipt.OperationID != record.ID || receipt.Succeeded != *record.VerificationSucceeded {
		return WorkspaceMutationResult{}, fmt.Errorf("terminal workspace mutation %s receipt disagrees with state", record.ID)
	}
	if err := tx.Commit(ctx); err != nil {
		return WorkspaceMutationResult{}, fmt.Errorf("commit terminal workspace mutation replay: %w", err)
	}
	return WorkspaceMutationResult{
		OperationID: record.ID, Status: record.Status,
		MutationEvidenceID:     *record.MutationEvidenceID,
		CommandEvidenceIDs:     append([]int64(nil), receipt.CommandEvidenceIDs...),
		VerificationEvidenceID: *record.VerificationEvidenceID,
		VerificationSucceeded:  *record.VerificationSucceeded,
	}, nil
}
