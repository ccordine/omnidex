package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) CurrentGeneratedWorkloadDeployment(
	ctx context.Context,
	jobID, generation int64,
) (*GeneratedWorkloadDeploymentSnapshot, error) {
	if ctx == nil {
		return nil, fmt.Errorf("load current generated deployment requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load current generated deployment: %w", err)
	}
	if jobID <= 0 || generation <= 0 {
		return nil, fmt.Errorf("load current generated deployment requires positive job and generation identities")
	}
	if r == nil || r.pool == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var commandJSON string
	var receiptJSON *string
	record, err := scanGeneratedWorkloadDeployment(r.pool.QueryRow(ctx, `
		SELECT deployment.id,deployment.command_sha256,deployment.status,
		       deployment.attempt_count,deployment.terminal_code,
		       deployment.terminal_detail_sha256,deployment.receipt_sha256,
		       deployment.evidence_id,deployment.prepared_at,deployment.updated_at,
		       deployment.applied_at,deployment.observed_at,
		       deployment.creator_step_attempt,deployment.creator_worker_id,
		       deployment.current_step_attempt,deployment.current_worker_id,
		       deployment.command_json,deployment.receipt_json
		FROM generated_workload_deployments AS deployment
		JOIN jobs ON jobs.id=deployment.job_id
		WHERE deployment.job_id=$1 AND deployment.generation=$2
		  AND jobs.current_generation=$2
	`, jobID, generation), &commandJSON, &receiptJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load current generated deployment: %w", err)
	}
	var envelope generatedWorkloadDeploymentCommandEnvelope
	if err := decodeExactGeneratedDeploymentJSON(commandJSON, &envelope); err != nil {
		return nil, fmt.Errorf("decode durable generated deployment command: %w", err)
	}
	if envelope.Schema != generatedWorkloadDeploymentCommandV1 {
		return nil, fmt.Errorf("durable generated deployment command schema is invalid")
	}
	identity, err := generatedWorkloadDeploymentOperation(envelope.Command)
	if err != nil {
		return nil, fmt.Errorf("validate durable generated deployment command: %w", err)
	}
	if err := requireGeneratedDeploymentIdentity(record, identity); err != nil {
		return nil, err
	}
	if identity.CommandJSON != commandJSON {
		return nil, fmt.Errorf("%w: durable generated deployment command is not canonical", ErrGeneratedWorkloadDeploymentConflict)
	}
	snapshot := &GeneratedWorkloadDeploymentSnapshot{Command: envelope.Command, Record: record}
	if receiptJSON == nil {
		if record.ReceiptSHA256 != "" || record.EvidenceID != 0 {
			return nil, fmt.Errorf("generated deployment without receipt has sealed receipt identity")
		}
		return snapshot, nil
	}
	var receipt GeneratedWorkloadDeploymentReceipt
	if err := decodeExactGeneratedDeploymentJSON(*receiptJSON, &receipt); err != nil {
		return nil, fmt.Errorf("decode durable generated deployment receipt: %w", err)
	}
	canonical, digest, err := canonicalGeneratedWorkloadDeploymentReceipt(
		envelope.Command, receipt, identity,
	)
	if err != nil {
		return nil, fmt.Errorf("validate durable generated deployment receipt: %w", err)
	}
	if canonical != *receiptJSON || digest != record.ReceiptSHA256 || record.EvidenceID <= 0 {
		return nil, fmt.Errorf("%w: durable generated deployment receipt differs", ErrGeneratedWorkloadDeploymentConflict)
	}
	snapshot.Receipt = &receipt
	return snapshot, nil
}

func decodeExactGeneratedDeploymentJSON(raw string, target any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}
