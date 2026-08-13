package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/jackc/pgx/v5"
)

func repositoryMutationEvidence(
	command RepositoryMutationCommand,
	operationID string,
) (evidence.Record, error) {
	if err := validateRepositoryMutationCommand(command); err != nil {
		return evidence.Record{}, err
	}
	operation, err := repositoryMutationOperation(command)
	if err != nil {
		return evidence.Record{}, err
	}
	if operation.ID != operationID {
		return evidence.Record{}, fmt.Errorf("repository mutation evidence operation identity is invalid")
	}
	paths := make([]string, len(command.ChangedFiles))
	fileIDs := make([]string, len(command.ChangedFiles))
	postState := make([]RepositoryMutationFile, len(command.ChangedFiles))
	copy(postState, command.ChangedFiles)
	createdCount, deletedCount, modifiedCount := 0, 0, 0
	for index, file := range command.ChangedFiles {
		paths[index] = file.Path
		fileIDs[index] = file.FileID
		switch {
		case !file.SourcePresent && file.ExpectedPresent:
			createdCount++
		case file.SourcePresent && !file.ExpectedPresent:
			deletedCount++
		case file.SourcePresent && file.ExpectedPresent:
			modifiedCount++
		}
	}
	record := evidence.Record{
		JobID: command.JobID, StepID: command.StepID,
		Kind: evidence.KindGeneratedDiff, SourceType: "repository", SourceRef: command.StageID,
		Hash: command.PatchSHA256, FilePaths: paths, Excerpt: command.Patch,
		Summary: fmt.Sprintf(
			"Applied one verified repository patch for %d exact target files.", len(paths),
		),
		Confidence: 1,
		Metadata: map[string]any{
			"mutation": true, "side_effect": true, "succeeded": true,
			"repository_change_contract_id":    command.ContractID,
			"repository_change_stage_id":       command.StageID,
			"repository_mutation_operation_id": operationID,
			"source_snapshot_id":               command.SourceSnapshotID,
			"patch_sha256":                     command.PatchSHA256,
			"changed_file_ids":                 fileIDs,
			"changed_files":                    postState,
			"created_file_count":               createdCount,
			"deleted_file_count":               deletedCount,
			"modified_file_count":              modifiedCount,
		},
	}
	return record, record.Validate()
}

func insertRepositoryMutationEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	command RepositoryMutationCommand,
	operationID string,
) (int64, error) {
	record, err := repositoryMutationEvidence(command, operationID)
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
		return 0, fmt.Errorf("check repository mutation evidence identity: %w", err)
	}
	if exists {
		return 0, fmt.Errorf("repository mutation stage %q already has successful evidence", command.StageID)
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return 0, fmt.Errorf("encode repository mutation evidence: %w", err)
	}
	var evidenceID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO evidence (job_id, step_id, kind, source_type, source_ref, payload_json)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
		RETURNING id
	`, command.JobID, command.StepID, record.Kind, record.SourceType,
		record.SourceRef, string(payload)).Scan(&evidenceID)
	if err != nil {
		return 0, fmt.Errorf("insert repository mutation evidence: %w", err)
	}
	if evidenceID <= 0 {
		return 0, fmt.Errorf("insert repository mutation evidence returned invalid identity %d", evidenceID)
	}
	return evidenceID, nil
}
