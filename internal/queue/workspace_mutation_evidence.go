package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/jackc/pgx/v5"
)

func workspaceMutationEvidence(
	command WorkspaceMutationCommand,
	operationID string,
) (evidence.Record, error) {
	identity, err := workspaceMutationOperation(command)
	if err != nil {
		return evidence.Record{}, err
	}
	if identity.ID != operationID {
		return evidence.Record{}, fmt.Errorf("workspace mutation evidence operation identity is invalid")
	}
	paths := make([]string, len(command.Plan.Files))
	created, deleted, modified := 0, 0, 0
	for index, file := range command.Plan.Files {
		paths[index] = file.Path
		switch {
		case !file.Source.Present:
			created++
		case !file.Expected.Present:
			deleted++
		default:
			modified++
		}
	}
	record := evidence.Record{
		JobID: command.JobID, StepID: command.StepID,
		Kind: evidence.KindGeneratedDiff, SourceType: "workspace", SourceRef: command.Plan.ID,
		Hash: command.Plan.PatchSHA256, FilePaths: paths,
		Excerpt:    fmt.Sprintf("workspace delta %s", command.Plan.PatchSHA256),
		Summary:    fmt.Sprintf("Applied one workspace delta for %d exact file transitions.", len(paths)),
		Confidence: 1,
		Metadata: map[string]any{
			"mutation": true, "side_effect": true, "succeeded": true,
			"workspace_mutation_operation_id": operationID,
			"workspace_id":                    command.Plan.WorkspaceID,
			"source_state_id":                 command.Plan.SourceStateID,
			"expected_state_id":               command.Plan.ExpectedStateID,
			"patch_sha256":                    command.Plan.PatchSHA256,
			"created_file_count":              created,
			"deleted_file_count":              deleted,
			"modified_file_count":             modified,
		},
	}
	return record, record.Validate()
}

func insertWorkspaceMutationEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	command WorkspaceMutationCommand,
	operationID string,
) (int64, error) {
	record, err := workspaceMutationEvidence(command, operationID)
	if err != nil {
		return 0, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM evidence
			WHERE job_id=$1 AND kind=$2 AND source_type=$3 AND source_ref=$4
		)
	`, command.JobID, record.Kind, record.SourceType, record.SourceRef).Scan(&exists); err != nil {
		return 0, fmt.Errorf("check workspace mutation evidence identity: %w", err)
	}
	if exists {
		return 0, fmt.Errorf("workspace mutation stage %q already has successful evidence", command.Plan.ID)
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return 0, fmt.Errorf("encode workspace mutation evidence: %w", err)
	}
	var evidenceID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,$3,$4,$5,$6::jsonb) RETURNING id
	`, command.JobID, command.StepID, record.Kind, record.SourceType,
		record.SourceRef, string(payload)).Scan(&evidenceID); err != nil {
		return 0, fmt.Errorf("insert workspace mutation evidence: %w", err)
	}
	if evidenceID <= 0 {
		return 0, fmt.Errorf("insert workspace mutation evidence returned invalid identity %d", evidenceID)
	}
	return evidenceID, nil
}

func requireWorkspaceMutationEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	command WorkspaceMutationCommand,
	operationID string,
	evidenceID int64,
) error {
	var kind, sourceType, sourceRef, patchHash, persistedOperation string
	err := tx.QueryRow(ctx, `
		SELECT kind,COALESCE(source_type,''),COALESCE(source_ref,''),
		       COALESCE(payload_json->>'hash',''),
		       COALESCE(payload_json->'metadata'->>'workspace_mutation_operation_id','')
		FROM evidence WHERE id=$1 AND job_id=$2 AND step_id=$3
	`, evidenceID, command.JobID, command.StepID).Scan(
		&kind, &sourceType, &sourceRef, &patchHash, &persistedOperation,
	)
	if err != nil {
		return fmt.Errorf("load workspace mutation evidence %d: %w", evidenceID, err)
	}
	if kind != evidence.KindGeneratedDiff || sourceType != "workspace" ||
		sourceRef != command.Plan.ID || patchHash != command.Plan.PatchSHA256 ||
		persistedOperation != operationID {
		return fmt.Errorf("workspace mutation evidence %d disagrees with operation %s", evidenceID, operationID)
	}
	return nil
}
