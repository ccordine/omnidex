package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repositoryPlanAcceptance struct {
	scope, contractID, sourceSnapshotID, verificationSnapshotID string
	stageID, patchSHA256, planID, expectedPostID                string
}

func assertRepositoryMutationWorkflowRecords(
	t *testing.T,
	pool *pgxpool.Pool,
	jobID int64,
	wantCommands int,
	targetFileID string,
	refreshed repositoryindex.Result,
) {
	t.Helper()
	var status, stageID, patchSHA, expectedSHA, mutationPath string
	var sourceSnapshotID, verifiedSnapshotID string
	var attempts, generatedDiffs, baselineProofs, baselineAcceptances, stagedProofs, authoritativeProofs int
	var evidenceID *int64
	err := pool.QueryRow(t.Context(), `
		SELECT operation.status,operation.apply_attempt_count,operation.mutation_evidence_id,
		       operation.stage_id,operation.patch_sha256,file.expected_sha256,
		       operation.source_repository_snapshot_id,operation.verified_repository_snapshot_id,
		       file.path
		FROM workspace_mutation_operations AS operation
		JOIN workspace_mutation_files AS file ON file.operation_id=operation.id
		WHERE operation.job_id=$1
	`, jobID).Scan(
		&status, &attempts, &evidenceID, &stageID, &patchSHA, &expectedSHA,
		&sourceSnapshotID, &verifiedSnapshotID, &mutationPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT
			COUNT(*) FILTER (WHERE kind=$2 AND source_type='workspace'),
			COUNT(*) FILTER (WHERE kind=$3 AND payload_json->'metadata'->>'repository_verification_scope'='baseline'
			  AND NOT COALESCE((payload_json->'metadata'->>'repository_verification_baseline_accepted')::boolean,false)),
			COUNT(*) FILTER (WHERE kind=$3 AND COALESCE((payload_json->'metadata'->>'repository_verification_baseline_accepted')::boolean,false)),
			COUNT(*) FILTER (WHERE kind=$3 AND payload_json->'metadata'->>'repository_verification_scope'='staged'
			  AND NOT COALESCE((payload_json->'metadata'->>'repository_verification_plan_accepted')::boolean,false)),
			COUNT(*) FILTER (WHERE kind=$3 AND payload_json->'metadata'->>'repository_verification_scope'='authoritative'
			  AND NOT COALESCE((payload_json->'metadata'->>'repository_verification_plan_accepted')::boolean,false))
		FROM evidence WHERE job_id=$1
	`, jobID, evidence.KindGeneratedDiff, evidence.KindTestResult).Scan(
		&generatedDiffs, &baselineProofs, &baselineAcceptances, &stagedProofs, &authoritativeProofs,
	); err != nil {
		t.Fatal(err)
	}
	file := exactRepositorySnapshotFile(t, refreshed.Snapshot, targetFileID)
	if status != "verified" || attempts != 1 || evidenceID == nil || generatedDiffs != 1 ||
		baselineProofs != wantCommands || baselineAcceptances != 1 ||
		stagedProofs != wantCommands || authoritativeProofs != wantCommands || expectedSHA != file.SHA256 ||
		mutationPath != file.Path || sourceSnapshotID == verifiedSnapshotID ||
		verifiedSnapshotID != refreshed.Snapshot.ID {
		t.Fatalf(
			"journal=%s/%d/%v diff=%d proof=%d/%d/%d baseline_acceptance=%d expected=%s actual=%s path=%s/%s snapshots=%s/%s/%s",
			status, attempts, evidenceID, generatedDiffs, baselineProofs, stagedProofs,
			authoritativeProofs, baselineAcceptances, expectedSHA, file.SHA256,
			mutationPath, file.Path,
			sourceSnapshotID, verifiedSnapshotID, refreshed.Snapshot.ID,
		)
	}
	acceptances := loadRepositoryPlanAcceptances(t, pool, jobID)
	if len(acceptances) != 1 || acceptances[0].scope != "staged" {
		t.Fatalf("plan acceptances=%+v", acceptances)
	}
	for _, accepted := range acceptances {
		if !validRepositoryVerificationOpaqueID(accepted.contractID, "change_contract_") ||
			!validRepositoryVerificationOpaqueID(accepted.sourceSnapshotID, "snapshot_") ||
			!validRepositoryVerificationOpaqueID(accepted.stageID, "repository_change_stage_") ||
			accepted.patchSHA256 != patchSHA || !validRepositoryVerificationSHA256(accepted.planID) ||
			!validRepositoryVerificationSHA256(accepted.expectedPostID) ||
			!validRepositoryVerificationOpaqueID(stageID, "workspace_stage_") {
			t.Fatalf("unbound plan acceptance=%+v journal=%s/%s", accepted, stageID, patchSHA)
		}
	}
	if acceptances[0].verificationSnapshotID != "" ||
		acceptances[0].sourceSnapshotID != sourceSnapshotID {
		t.Fatalf("staged plan acceptance has invalid snapshot authority: %+v", acceptances)
	}
	for _, canary := range repositoryMutationSecretCanaries {
		var leaked int
		if err := pool.QueryRow(t.Context(), `
			SELECT COUNT(*) FROM evidence
			WHERE job_id=$1 AND payload_json::text LIKE '%' || $2 || '%'
		`, jobID, canary).Scan(&leaked); err != nil {
			t.Fatal(err)
		}
		if leaked != 0 {
			t.Fatalf("excluded repository canary %q entered exact command evidence", canary)
		}
	}
	var correctionCalls int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM llm_call_evidence WHERE job_id=$1
	`, jobID).Scan(&correctionCalls); err != nil {
		t.Fatal(err)
	}
	if correctionCalls != 0 {
		t.Fatalf("snapshot-only proof unexpectedly invoked %d model corrections", correctionCalls)
	}
}

func loadRepositoryPlanAcceptances(
	t *testing.T,
	pool *pgxpool.Pool,
	jobID int64,
) []repositoryPlanAcceptance {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		SELECT payload_json->'metadata'->>'repository_verification_scope',
		       payload_json->'metadata'->>'repository_change_contract_id',
		       payload_json->'metadata'->>'repository_source_snapshot_id',
		       payload_json->'metadata'->>'repository_change_stage_id',
		       payload_json->'metadata'->>'repository_change_patch_sha256',
		       payload_json->'metadata'->>'repository_verification_plan_id',
		       payload_json->'metadata'->>'repository_expected_post_id',
		       COALESCE(payload_json->'metadata'->>'repository_verification_snapshot_id','')
		FROM evidence
		WHERE job_id=$1 AND kind=$2
		  AND COALESCE((payload_json->'metadata'->>'repository_verification_plan_accepted')::boolean,false)
		ORDER BY payload_json->'metadata'->>'repository_verification_scope'
	`, jobID, evidence.KindTestResult)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	accepted := make([]repositoryPlanAcceptance, 0, 2)
	for rows.Next() {
		var item repositoryPlanAcceptance
		if err := rows.Scan(
			&item.scope, &item.contractID, &item.sourceSnapshotID, &item.stageID,
			&item.patchSHA256, &item.planID, &item.expectedPostID,
			&item.verificationSnapshotID,
		); err != nil {
			t.Fatal(err)
		}
		accepted = append(accepted, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return accepted
}
