package queue

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type desiredVerificationRow struct {
	sourceType, sourceRef, command, scope, owner, graphID, sourceID     string
	stageID, patchSHA, planID, expectedPostID, verificationSnapshotID   string
	proofValid, succeeded, baselineAccepted, planAccepted, commandCount string
}

func loadDesiredVerificationEvidence(
	ctx context.Context, tx pgx.Tx, authority model.StepAttemptAuthority,
	graphID, beforeID, afterID, stageID, patchSHA string,
	proof *DesiredRepositoryExecutionEvidence,
) error {
	rows, err := tx.Query(ctx, `
		SELECT COALESCE(source_type,''),COALESCE(source_ref,''),COALESCE(payload_json->>'command',''),
		       COALESCE(payload_json->'metadata'->>'repository_verification_scope',''),
		       COALESCE(payload_json->'metadata'->>'repository_mutation_owner_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_desired_artifact_graph_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_source_snapshot_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_change_stage_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_change_patch_sha256',''),
		       COALESCE(payload_json->'metadata'->>'repository_verification_plan_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_expected_post_id',''),
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
			&row.graphID, &row.sourceID, &row.stageID, &row.patchSHA, &row.planID,
			&row.expectedPostID, &row.verificationSnapshotID, &row.proofValid,
			&row.succeeded, &row.baselineAccepted, &row.planAccepted, &row.commandCount,
		); err != nil {
			return fmt.Errorf("scan desired repository verification evidence: %w", err)
		}
		counter, registered := counts[row.scope]
		if !registered || row.owner != graphID || row.graphID != graphID ||
			row.sourceID != beforeID || !repositoryMutationHexDigest(row.planID) ||
			(planID != "" && row.planID != planID) {
			return fmt.Errorf("desired repository verification evidence has mismatched common authority")
		}
		planID = row.planID
		if row.scope == "baseline" {
			if row.stageID != "" || row.patchSHA != "" || row.verificationSnapshotID != "" {
				return fmt.Errorf("desired repository baseline evidence contains post-state authority")
			}
		} else if row.stageID != stageID || row.patchSHA != patchSHA ||
			!repositoryMutationHexDigest(row.expectedPostID) ||
			(row.scope == "authoritative" && row.verificationSnapshotID != afterID) ||
			(row.scope == "staged" && row.verificationSnapshotID != "") {
			return fmt.Errorf("desired repository verification evidence has mismatched stage authority")
		}
		if row.command != "" {
			if row.sourceType != "command" || row.sourceRef != "go" || row.proofValid != "true" ||
				row.succeeded != "true" || row.baselineAccepted != "" || row.planAccepted != "" ||
				row.commandCount != "" {
				return fmt.Errorf("desired repository command evidence is not one successful structured proof")
			}
			*counter++
			commands[row.scope] = append(commands[row.scope], row.command)
			continue
		}
		wantAcceptance := row.scope == "baseline" && row.baselineAccepted == "true" &&
			row.planAccepted == "" && row.sourceType == "command-baseline"
		wantAcceptance = wantAcceptance || row.scope != "baseline" && row.planAccepted == "true" &&
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
	for _, scope := range []string{"baseline", "staged", "authoritative"} {
		if acceptances[scope] != 1 || expectedCounts[scope] != *counts[scope] || *counts[scope] < 1 {
			return fmt.Errorf("desired repository verification scope %q lacks one exact accepted command set", scope)
		}
		sort.Strings(commands[scope])
	}
	if !equalDesiredCommandSet(commands["baseline"], commands["staged"]) ||
		!equalDesiredCommandSet(commands["baseline"], commands["authoritative"]) {
		return fmt.Errorf("desired repository verification scopes executed different command sets")
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
