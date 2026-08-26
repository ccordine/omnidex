package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/jackc/pgx/v5"
)

const generatedDeploymentEvidenceSource = "docker_compose_deployment"

type generatedDeploymentEvidenceMetadata struct {
	OperationID             string    `json:"deployment_operation_id"`
	ComposeProject          string    `json:"compose_project"`
	ConfigSHA256            string    `json:"config_sha256"`
	EndpointScheme          string    `json:"endpoint_scheme"`
	EndpointHost            string    `json:"endpoint_host"`
	EndpointPort            uint16    `json:"endpoint_port"`
	EndpointPath            string    `json:"endpoint_path"`
	AppliedAt               time.Time `json:"applied_at"`
	ObservedAt              time.Time `json:"observed_at"`
	WorkspaceVerificationID string    `json:"workspace_verification_receipt_id"`
	ExecutionEvidenceIDs    []int64   `json:"execution_evidence_ids"`
	ObservationEvidenceIDs  []int64   `json:"observation_evidence_ids"`
	Succeeded               bool      `json:"succeeded"`
}

type generatedDeploymentEvidencePayload struct {
	JobID      int64                               `json:"job_id"`
	StepID     int64                               `json:"step_id"`
	Kind       string                              `json:"kind"`
	SourceType string                              `json:"source_type"`
	SourceRef  string                              `json:"source_ref"`
	Excerpt    string                              `json:"excerpt"`
	Summary    string                              `json:"summary"`
	Hash       string                              `json:"hash"`
	Confidence float64                             `json:"confidence"`
	Metadata   generatedDeploymentEvidenceMetadata `json:"metadata"`
}

func generatedWorkloadDeploymentEvidence(
	command GeneratedWorkloadDeploymentCommand,
	receipt GeneratedWorkloadDeploymentReceipt,
	receiptJSON, receiptSHA string,
) (generatedDeploymentEvidencePayload, error) {
	record := generatedDeploymentEvidencePayload{
		JobID: command.Authority.JobID, StepID: command.Authority.StepID,
		Kind: evidence.KindDeploymentReceipt, SourceType: generatedDeploymentEvidenceSource,
		SourceRef: receipt.OperationID, Hash: receiptSHA, Excerpt: receiptJSON,
		Summary:    "Applied one generated-workload deployment with a healthy observed service set.",
		Confidence: 1,
		Metadata: generatedDeploymentEvidenceMetadata{
			OperationID: receipt.OperationID, ComposeProject: receipt.ComposeProject,
			ConfigSHA256: receipt.ConfigSHA256, EndpointScheme: receipt.EndpointScheme,
			EndpointHost: receipt.EndpointHost, EndpointPort: receipt.EndpointPort,
			EndpointPath: receipt.EndpointPath, AppliedAt: receipt.AppliedAt,
			ObservedAt:              receipt.ObservedAt,
			WorkspaceVerificationID: receipt.WorkspaceVerificationReceiptID,
			ExecutionEvidenceIDs:    append([]int64(nil), receipt.ExecutionEvidenceIDs...),
			ObservationEvidenceIDs:  append([]int64(nil), receipt.ObservationEvidenceIDs...),
			Succeeded:               true,
		},
	}
	return record, (evidence.Record{Kind: record.Kind}).Validate()
}

func insertGeneratedDeploymentEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	command GeneratedWorkloadDeploymentCommand,
	receipt GeneratedWorkloadDeploymentReceipt,
	receiptJSON, receiptSHA string,
) (int64, error) {
	record, err := generatedWorkloadDeploymentEvidence(command, receipt, receiptJSON, receiptSHA)
	if err != nil {
		return 0, err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return 0, fmt.Errorf("encode generated deployment evidence: %w", err)
	}
	var evidenceID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO evidence (job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb) RETURNING id
	`, record.JobID, record.StepID, record.Kind, record.SourceType,
		record.SourceRef, string(payload)).Scan(&evidenceID)
	if err != nil {
		return 0, fmt.Errorf("insert generated deployment receipt evidence: %w", err)
	}
	if evidenceID <= 0 {
		return 0, fmt.Errorf("insert generated deployment receipt returned invalid evidence identity")
	}
	return evidenceID, nil
}

func requireGeneratedDeploymentEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	command GeneratedWorkloadDeploymentCommand,
	operationID, receiptJSON, receiptSHA string,
	evidenceID int64,
) error {
	var kind, sourceType, sourceRef, hash, excerpt string
	err := tx.QueryRow(ctx, `
		SELECT kind,COALESCE(source_type,''),COALESCE(source_ref,''),
		       COALESCE(payload_json->>'hash',''),COALESCE(payload_json->>'excerpt','')
		FROM evidence WHERE id=$1 AND job_id=$2 AND step_id=$3
	`, evidenceID, command.Authority.JobID, command.Authority.StepID).Scan(
		&kind, &sourceType, &sourceRef, &hash, &excerpt,
	)
	if err != nil {
		return fmt.Errorf("load generated deployment evidence %d: %w", evidenceID, err)
	}
	if kind != evidence.KindDeploymentReceipt || sourceType != generatedDeploymentEvidenceSource ||
		sourceRef != operationID || hash != receiptSHA || excerpt != receiptJSON {
		return fmt.Errorf("generated deployment evidence %d disagrees with operation %s", evidenceID, operationID)
	}
	return nil
}
