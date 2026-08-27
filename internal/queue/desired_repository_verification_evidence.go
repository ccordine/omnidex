package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type desiredVerificationRow struct {
	sourceType, sourceRef, command, scope, owner, graphID string
	repositorySourceID, workspaceSourceID                 string
	repositoryStageID, workspaceStageID                   string
	repositoryPatchSHA, workspacePatchSHA                 string
	planID, expectedPostID, workspaceExpectedStateID      string
	verificationSnapshotID                                string
	proofValid, succeeded, baselineAccepted               string
	planAccepted, commandCount                            string
}

func loadDesiredVerificationEvidence(
	ctx context.Context, tx pgx.Tx, authority model.StepAttemptAuthority,
	graphID, beforeID, afterID, operationID, stageID, patchSHA string,
	sourceStateID, expectedStateID string,
	verificationEvidenceID int64,
	plannedVerificationCommands int,
	proof *DesiredRepositoryExecutionEvidence,
) error {
	rows, err := tx.Query(ctx, `
		SELECT COALESCE(source_type,''),COALESCE(source_ref,''),COALESCE(payload_json->>'command',''),
		       COALESCE(payload_json->'metadata'->>'repository_verification_scope',''),
		       COALESCE(payload_json->'metadata'->>'repository_mutation_owner_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_desired_artifact_graph_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_source_snapshot_id',''),
		       COALESCE(payload_json->'metadata'->>'workspace_source_snapshot_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_change_stage_id',''),
		       COALESCE(payload_json->'metadata'->>'workspace_mutation_stage_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_change_patch_sha256',''),
		       COALESCE(payload_json->'metadata'->>'workspace_mutation_patch_sha256',''),
		       COALESCE(payload_json->'metadata'->>'repository_verification_plan_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_expected_post_id',''),
		       COALESCE(payload_json->'metadata'->>'workspace_expected_state_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_verification_snapshot_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_structured_proof_valid',''),
		       COALESCE(payload_json->'metadata'->>'succeeded',''),
		       COALESCE(payload_json->'metadata'->>'repository_verification_baseline_accepted',''),
		       COALESCE(payload_json->'metadata'->>'repository_verification_plan_accepted',''),
		       COALESCE(payload_json->'metadata'->>'repository_verification_command_count','')
		FROM evidence
		WHERE job_id=$1 AND step_id=$2 AND kind=$3
		  AND (payload_json->'metadata'->>'repository_mutation_owner_id'=$4
		       OR payload_json->'metadata'->>'repository_desired_artifact_graph_id'=$4)
		ORDER BY id
	`, authority.JobID, authority.StepID, evidence.KindTestResult, graphID)
	if err != nil {
		return fmt.Errorf("load desired repository verification evidence: %w", err)
	}
	defer rows.Close()
	counts := map[string]*int{
		"baseline":      &proof.VerificationCommands.Baseline,
		"staged":        &proof.VerificationCommands.Staged,
		"authoritative": &proof.VerificationCommands.Authoritative,
	}
	acceptances, expectedCounts := map[string]int{}, map[string]int{}
	commands := map[string][]string{}
	planID := ""
	for rows.Next() {
		var row desiredVerificationRow
		if err := rows.Scan(
			&row.sourceType, &row.sourceRef, &row.command, &row.scope, &row.owner,
			&row.graphID, &row.repositorySourceID, &row.workspaceSourceID,
			&row.repositoryStageID, &row.workspaceStageID,
			&row.repositoryPatchSHA, &row.workspacePatchSHA, &row.planID,
			&row.expectedPostID, &row.workspaceExpectedStateID,
			&row.verificationSnapshotID, &row.proofValid,
			&row.succeeded, &row.baselineAccepted, &row.planAccepted, &row.commandCount,
		); err != nil {
			return fmt.Errorf("scan desired repository verification evidence: %w", err)
		}
		counter, registered := counts[row.scope]
		if !registered || row.owner != graphID || row.graphID != graphID ||
			row.repositorySourceID != beforeID || !validSHA256Digest(row.planID) ||
			(planID != "" && row.planID != planID) {
			return fmt.Errorf("desired repository verification evidence has mismatched common authority")
		}
		planID = row.planID
		switch row.scope {
		case "baseline":
			if row.repositoryStageID != "" || row.workspaceStageID != "" ||
				row.repositoryPatchSHA != "" || row.workspacePatchSHA != "" ||
				row.expectedPostID != "" || row.workspaceExpectedStateID != "" ||
				row.workspaceSourceID != "" || row.verificationSnapshotID != "" {
				return fmt.Errorf("desired repository baseline evidence contains post-state authority")
			}
		case "staged":
			if !validSHA256ID(row.repositoryStageID, "repository_change_stage_") ||
				!validSHA256Digest(row.repositoryPatchSHA) ||
				!validSHA256Digest(row.expectedPostID) ||
				row.workspaceSourceID != "" || row.workspaceStageID != "" ||
				row.workspacePatchSHA != "" || row.workspaceExpectedStateID != "" ||
				row.verificationSnapshotID != "" {
				return fmt.Errorf("desired repository staged verification has malformed stage authority")
			}
		case "authoritative":
			if row.workspaceSourceID != beforeID || row.workspaceStageID != stageID ||
				row.workspacePatchSHA != patchSHA || row.workspaceExpectedStateID != expectedStateID ||
				row.repositoryStageID != "" || row.repositoryPatchSHA != "" ||
				row.expectedPostID != "" || row.verificationSnapshotID != afterID {
				return fmt.Errorf("desired repository authoritative verification has mismatched workspace authority")
			}
		}
		if row.command != "" {
			validSource := row.scope != "authoritative" &&
				row.sourceType == "command" && row.sourceRef == "go"
			if row.scope == "authoritative" {
				validSource = row.sourceType == "workspace_verification" &&
					row.sourceRef == operationID
			}
			if !validSource || row.proofValid != "true" ||
				row.succeeded != "true" || row.baselineAccepted != "" || row.planAccepted != "" ||
				row.commandCount != "" {
				return fmt.Errorf("desired repository command evidence is not one successful structured proof")
			}
			*counter++
			if row.scope != "authoritative" {
				commands[row.scope] = append(commands[row.scope], row.command)
			}
			continue
		}
		if row.scope == "authoritative" {
			return fmt.Errorf("desired repository authoritative verification contains non-command evidence")
		}
		wantAcceptance := row.scope == "baseline" && row.baselineAccepted == "true" &&
			row.planAccepted == "" && row.sourceType == "command-baseline"
		wantAcceptance = wantAcceptance || row.scope == "staged" && row.planAccepted == "true" &&
			row.baselineAccepted == "" && row.sourceType == "command-plan"
		commandCount, countErr := strconv.Atoi(row.commandCount)
		if !wantAcceptance || row.sourceRef != "go" || row.proofValid != "" || row.succeeded != "" ||
			countErr != nil || commandCount < 1 || commandCount > 64 {
			return fmt.Errorf("desired repository verification acceptance evidence is malformed")
		}
		acceptances[row.scope]++
		expectedCounts[row.scope] = commandCount
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate desired repository verification evidence: %w", err)
	}
	for _, scope := range []string{"baseline", "staged"} {
		if acceptances[scope] != 1 || expectedCounts[scope] != *counts[scope] || *counts[scope] < 1 {
			return fmt.Errorf("desired repository verification scope %q lacks one exact accepted command set", scope)
		}
		sort.Strings(commands[scope])
	}
	if acceptances["authoritative"] != 0 ||
		proof.VerificationCommands.Authoritative != plannedVerificationCommands ||
		plannedVerificationCommands < 1 {
		return fmt.Errorf("desired repository authoritative verification lacks its exact journal command set")
	}
	if !equalDesiredCommandSet(commands["baseline"], commands["staged"]) {
		return fmt.Errorf("desired repository baseline and staged verification executed different command sets")
	}
	if err := validateDesiredWorkspaceVerificationReceipt(
		ctx, tx, authority, verificationEvidenceID, operationID,
		sourceStateID, expectedStateID, plannedVerificationCommands,
	); err != nil {
		return err
	}
	return nil
}

func equalDesiredCommandSet(left, right []string) bool {
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

func validateDesiredWorkspaceVerificationReceipt(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	evidenceID int64,
	operationID, sourceStateID, expectedStateID string,
	plannedCommands int,
) error {
	var kind, sourceType, sourceRef, payload string
	if err := tx.QueryRow(ctx, `
		SELECT kind,COALESCE(source_type,''),COALESCE(source_ref,''),payload_json::text
		FROM evidence WHERE id=$1 AND job_id=$2 AND step_id=$3
	`, evidenceID, authority.JobID, authority.StepID).Scan(
		&kind, &sourceType, &sourceRef, &payload,
	); err != nil {
		return fmt.Errorf("load desired repository workspace verification receipt: %w", err)
	}
	var record evidence.Record
	if err := json.Unmarshal([]byte(payload), &record); err != nil {
		return fmt.Errorf("decode desired repository workspace verification evidence: %w", err)
	}
	var receipt workspaceMutationVerificationReceipt
	if err := json.Unmarshal([]byte(record.Excerpt), &receipt); err != nil {
		return fmt.Errorf("decode desired repository workspace verification receipt: %w", err)
	}
	succeeded, succeededOK := record.Metadata["succeeded"].(bool)
	observedState, observedOK := record.Metadata["observed_state_id"].(string)
	if kind != evidence.KindWorkspaceVerification || record.Kind != kind ||
		sourceType != "workspace_mutation" || sourceRef != operationID ||
		record.SourceType != sourceType || record.SourceRef != sourceRef ||
		record.Hash != digestWorkspaceMutationText(record.Excerpt) ||
		!succeededOK || !succeeded || !observedOK || observedState != expectedStateID ||
		receipt.OperationID != operationID || receipt.SourceStateID != sourceStateID ||
		receipt.ExpectedStateID != expectedStateID || receipt.ObservedStateID != expectedStateID ||
		!receipt.Succeeded || len(receipt.CommandEvidenceIDs) != plannedCommands {
		return fmt.Errorf("desired repository workspace verification receipt disagrees with its terminal operation")
	}
	for index, id := range receipt.CommandEvidenceIDs {
		if id <= 0 || index > 0 && id <= receipt.CommandEvidenceIDs[index-1] {
			return fmt.Errorf("desired repository workspace verification receipt has invalid command evidence identities")
		}
	}
	return nil
}

func loadDesiredPostIndexEvidence(
	ctx context.Context, tx pgx.Tx, authority model.StepAttemptAuthority,
	afterID, afterGitSHA string, afterFiles int, proof *DesiredRepositoryExecutionEvidence,
) error {
	var sourceType, sourceRef, hash, snapshotID, fileCount string
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*),COALESCE(MIN(source_type),''),COALESCE(MIN(source_ref),''),
		       COALESCE(MIN(payload_json->>'hash'),''),
		       COALESCE(MIN(payload_json->'metadata'->>'snapshot_id'),''),
		       COALESCE(MIN(payload_json->'metadata'->>'file_count'),'')
		FROM evidence WHERE job_id=$1 AND step_id=$2 AND kind=$3 AND source_ref=$4
	`, authority.JobID, authority.StepID, evidence.KindRepositoryIndex, afterID).Scan(
		&proof.PostStateRepositoryReindexes, &sourceType, &sourceRef, &hash, &snapshotID, &fileCount,
	)
	if err != nil {
		return fmt.Errorf("load desired repository post-state index evidence: %w", err)
	}
	persistedFiles, parseErr := strconv.Atoi(fileCount)
	if proof.PostStateRepositoryReindexes != 1 || sourceType != "repository" || sourceRef != afterID ||
		hash != afterGitSHA || snapshotID != afterID || parseErr != nil || persistedFiles != afterFiles {
		return fmt.Errorf("desired repository execution requires one exact post-state repository reindex")
	}
	return nil
}
