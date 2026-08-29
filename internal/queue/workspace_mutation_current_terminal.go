package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/jackc/pgx/v5"
)

func validateCurrentWorkspaceMutationTx(
	ctx context.Context,
	tx pgx.Tx,
	command WorkspaceMutationCommand,
	record currentWorkspaceMutationRecord,
) (*WorkspaceMutationSnapshot, error) {
	snapshot := &WorkspaceMutationSnapshot{
		OperationID: record.operationID,
		Status:      record.status,
		Command:     command,
	}
	terminal := record.status == workspaceMutationVerified ||
		record.status == workspaceMutationVerificationFailed
	if !terminal {
		if !validNonterminalWorkspaceMutationStatus(record.status) {
			return nil, fmt.Errorf("current workspace mutation %s has invalid status %q", record.operationID, record.status)
		}
		if record.verificationSucceeded != nil || record.verificationReceipt != nil ||
			record.verificationReceiptSHA != nil || record.verificationEvidenceID != nil ||
			record.verifiedRepositorySnapshotID != nil {
			return nil, fmt.Errorf("nonterminal workspace mutation %s contains terminal authority", record.operationID)
		}
		return snapshot, nil
	}
	if record.mutationEvidenceID == nil || *record.mutationEvidenceID <= 0 ||
		record.verificationSucceeded == nil || record.verificationReceipt == nil ||
		record.verificationReceiptSHA == nil || record.verificationEvidenceID == nil ||
		*record.verificationEvidenceID <= 0 {
		return nil, fmt.Errorf("terminal workspace mutation %s is incomplete", record.operationID)
	}
	if record.status == workspaceMutationVerified && (!*record.verificationSucceeded || record.lastError != nil) {
		return nil, fmt.Errorf("verified workspace mutation %s has contradictory terminal authority", record.operationID)
	}
	if record.status == workspaceMutationVerificationFailed &&
		(*record.verificationSucceeded || record.lastError == nil || *record.lastError == "") {
		return nil, fmt.Errorf("failed workspace mutation %s has contradictory terminal authority", record.operationID)
	}
	if digestWorkspaceMutationText(*record.verificationReceipt) != *record.verificationReceiptSHA {
		return nil, fmt.Errorf("terminal workspace mutation %s receipt digest differs", record.operationID)
	}
	verifiedRepositorySnapshotID, err := workspaceMutationVerifiedSnapshotID(
		command, record.operationID, record.verifiedRepositorySnapshotID,
	)
	if err != nil {
		return nil, err
	}

	receipt, err := decodeCurrentWorkspaceMutationReceipt(
		*record.verificationReceipt, command, record,
	)
	if err != nil {
		return nil, err
	}
	if err := requireWorkspaceMutationEvidenceTx(
		ctx, tx, command, record.operationID, *record.mutationEvidenceID,
	); err != nil {
		return nil, err
	}
	if err := validateCurrentWorkspaceMutationCommandEvidenceTx(
		ctx, tx, command, record, receipt.CommandEvidenceIDs,
	); err != nil {
		return nil, err
	}
	if err := validateCurrentWorkspaceMutationReceiptEvidenceTx(
		ctx, tx, command, record,
	); err != nil {
		return nil, err
	}

	failure := ""
	if record.lastError != nil {
		failure = *record.lastError
	}
	result := WorkspaceMutationResult{
		OperationID:                  record.operationID,
		Status:                       record.status,
		MutationEvidenceID:           *record.mutationEvidenceID,
		CommandEvidenceIDs:           append([]int64(nil), receipt.CommandEvidenceIDs...),
		VerificationEvidenceID:       *record.verificationEvidenceID,
		VerificationSucceeded:        *record.verificationSucceeded,
		VerifiedRepositorySnapshotID: verifiedRepositorySnapshotID,
	}
	snapshot.Terminal = &WorkspaceMutationTerminal{
		Result: result, Failure: failure,
		ReceiptJSON: *record.verificationReceipt, ReceiptSHA256: *record.verificationReceiptSHA,
	}
	return snapshot, nil
}

func workspaceMutationVerifiedSnapshotID(
	command WorkspaceMutationCommand,
	operationID string,
	persisted *string,
) (string, error) {
	if command.Plan.GitSourceSnapshotID == "" {
		if persisted != nil {
			return "", fmt.Errorf(
				"terminal workspace mutation %s contains unexpected verified repository snapshot authority",
				operationID,
			)
		}
		return "", nil
	}
	if persisted == nil || !validSHA256ID(*persisted, "snapshot_") {
		return "", fmt.Errorf(
			"terminal workspace mutation %s lacks exact verified repository snapshot authority",
			operationID,
		)
	}
	return *persisted, nil
}

func validNonterminalWorkspaceMutationStatus(status string) bool {
	switch status {
	case workspaceMutationPrepared, workspaceMutationApplying, workspaceMutationApplied,
		workspaceMutationVerifying, workspaceMutationIndeterminateState:
		return true
	default:
		return false
	}
}

func decodeCurrentWorkspaceMutationReceipt(
	raw string,
	command WorkspaceMutationCommand,
	record currentWorkspaceMutationRecord,
) (workspaceMutationVerificationReceipt, error) {
	var receipt workspaceMutationVerificationReceipt
	if err := json.Unmarshal([]byte(raw), &receipt); err != nil {
		return workspaceMutationVerificationReceipt{},
			fmt.Errorf("decode terminal workspace mutation receipt: %w", err)
	}
	if receipt.Schema != workspaceMutationReceiptSchema ||
		receipt.OperationID != record.operationID ||
		receipt.SourceStateID != command.Plan.SourceStateID ||
		receipt.ExpectedStateID != command.Plan.ExpectedStateID ||
		receipt.ObservedStateID != command.Plan.ExpectedStateID ||
		receipt.Succeeded != *record.verificationSucceeded ||
		len(receipt.CommandEvidenceIDs) != len(command.Verification.Commands) {
		return workspaceMutationVerificationReceipt{}, fmt.Errorf(
			"terminal workspace mutation %s receipt disagrees with command authority",
			record.operationID,
		)
	}
	previous := int64(0)
	for _, evidenceID := range receipt.CommandEvidenceIDs {
		if evidenceID <= previous {
			return workspaceMutationVerificationReceipt{}, fmt.Errorf(
				"terminal workspace mutation %s receipt evidence set is invalid",
				record.operationID,
			)
		}
		previous = evidenceID
	}
	return receipt, nil
}

func validateCurrentWorkspaceMutationCommandEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	command WorkspaceMutationCommand,
	record currentWorkspaceMutationRecord,
	evidenceIDs []int64,
) error {
	rows, err := tx.Query(ctx, `
		SELECT evidence.id,evidence.kind,COALESCE(evidence.source_type,''),
		       COALESCE(evidence.source_ref,''),COALESCE(evidence.payload_json->>'command',''),
		       COALESCE(evidence.payload_json->'metadata'->>'succeeded','')
		FROM unnest($1::bigint[]) WITH ORDINALITY AS cited(evidence_id,ordinal)
		JOIN evidence ON evidence.id=cited.evidence_id AND
		     evidence.job_id=$2 AND evidence.step_id=$3
		ORDER BY cited.ordinal
	`, evidenceIDs, command.JobID, command.StepID)
	if err != nil {
		return fmt.Errorf("load terminal workspace mutation command evidence: %w", err)
	}
	defer rows.Close()
	index := 0
	failed := false
	for rows.Next() {
		var evidenceID int64
		var kind, sourceType, sourceRef, exactCommand, succeededText string
		if err := rows.Scan(
			&evidenceID, &kind, &sourceType, &sourceRef, &exactCommand, &succeededText,
		); err != nil {
			return fmt.Errorf("scan terminal workspace mutation command evidence: %w", err)
		}
		if index >= len(command.Verification.Commands) || evidenceID != evidenceIDs[index] {
			return fmt.Errorf("terminal workspace mutation %s command evidence set differs", record.operationID)
		}
		planned := command.Verification.Commands[index]
		if kind != planned.Kind || sourceType != "workspace_verification" ||
			sourceRef != record.operationID || exactCommand != planned.Command ||
			digestWorkspaceMutationText(exactCommand) != planned.CommandSHA256 ||
			(succeededText != "true" && succeededText != "false") {
			return fmt.Errorf("terminal workspace mutation %s command evidence %d differs", record.operationID, evidenceID)
		}
		if succeededText == "false" {
			failed = true
		}
		if *record.verificationSucceeded && succeededText != "true" {
			return fmt.Errorf("verified workspace mutation %s contains failed command evidence", record.operationID)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate terminal workspace mutation command evidence: %w", err)
	}
	if index != len(evidenceIDs) || !*record.verificationSucceeded && !failed {
		return fmt.Errorf("terminal workspace mutation %s command evidence outcome differs", record.operationID)
	}
	return nil
}

func validateCurrentWorkspaceMutationReceiptEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	command WorkspaceMutationCommand,
	record currentWorkspaceMutationRecord,
) error {
	var kind, sourceType, sourceRef, hash, excerpt string
	var operationID, observedStateID, succeeded string
	err := tx.QueryRow(ctx, `
		SELECT kind,COALESCE(source_type,''),COALESCE(source_ref,''),
		       COALESCE(payload_json->>'hash',''),COALESCE(payload_json->>'excerpt',''),
		       COALESCE(payload_json->'metadata'->>'workspace_mutation_operation_id',''),
		       COALESCE(payload_json->'metadata'->>'observed_state_id',''),
		       COALESCE(payload_json->'metadata'->>'succeeded','')
		FROM evidence WHERE id=$1 AND job_id=$2 AND step_id=$3
	`, *record.verificationEvidenceID, command.JobID, command.StepID).Scan(
		&kind, &sourceType, &sourceRef, &hash, &excerpt,
		&operationID, &observedStateID, &succeeded,
	)
	if err != nil {
		return fmt.Errorf("load terminal workspace mutation receipt evidence: %w", err)
	}
	if kind != evidence.KindWorkspaceVerification || sourceType != "workspace_mutation" ||
		sourceRef != record.operationID || hash != *record.verificationReceiptSHA ||
		excerpt != *record.verificationReceipt || operationID != record.operationID ||
		observedStateID != command.Plan.ExpectedStateID ||
		succeeded != fmt.Sprintf("%t", *record.verificationSucceeded) {
		return fmt.Errorf("terminal workspace mutation %s receipt evidence differs", record.operationID)
	}
	return nil
}
