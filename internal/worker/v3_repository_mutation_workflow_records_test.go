package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/queue"
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
	var status, stageID, patchSHA, expectedSHA string
	var attempts, generatedDiffs, baselineProofs, baselineAcceptances, stagedProofs, authoritativeProofs int
	var evidenceID *int64
	err := pool.QueryRow(t.Context(), `
		SELECT operation.status,operation.attempt_count,operation.evidence_id,
		       operation.stage_id,operation.patch_sha256,file.expected_sha256
		FROM repository_mutation_operations AS operation
		JOIN repository_mutation_files AS file ON file.operation_id=operation.id
		WHERE operation.job_id=$1 AND file.file_id=$2
	`, jobID, targetFileID).Scan(
		&status, &attempts, &evidenceID, &stageID, &patchSHA, &expectedSHA,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT
			COUNT(*) FILTER (WHERE kind=$2 AND source_type='repository'),
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
	if status != "applied" || attempts != 1 || evidenceID == nil || generatedDiffs != 1 ||
		baselineProofs != wantCommands || baselineAcceptances != 1 ||
		stagedProofs != wantCommands || authoritativeProofs != wantCommands || expectedSHA != file.SHA256 {
		t.Fatalf(
			"journal=%s/%d/%v diff=%d proof=%d/%d/%d baseline_acceptance=%d expected=%s actual=%s",
			status, attempts, evidenceID, generatedDiffs, baselineProofs, stagedProofs,
			authoritativeProofs, baselineAcceptances, expectedSHA, file.SHA256,
		)
	}
	acceptances := loadRepositoryPlanAcceptances(t, pool, jobID)
	if len(acceptances) != 2 || acceptances[0].scope != "authoritative" ||
		acceptances[1].scope != "staged" {
		t.Fatalf("plan acceptances=%+v", acceptances)
	}
	for _, accepted := range acceptances {
		if accepted.contractID == "" || accepted.sourceSnapshotID == "" ||
			accepted.stageID != stageID || accepted.patchSHA256 != patchSHA ||
			accepted.planID == "" || accepted.expectedPostID == "" {
			t.Fatalf("unbound plan acceptance=%+v journal=%s/%s", accepted, stageID, patchSHA)
		}
	}
	if acceptances[0].verificationSnapshotID == "" ||
		acceptances[1].verificationSnapshotID != "" {
		t.Fatalf("verification projection identity is not authoritative-only: %+v", acceptances)
	}
	if acceptances[0].contractID != acceptances[1].contractID ||
		acceptances[0].sourceSnapshotID != acceptances[1].sourceSnapshotID ||
		acceptances[0].planID != acceptances[1].planID ||
		acceptances[0].expectedPostID != acceptances[1].expectedPostID {
		t.Fatalf("staged and authoritative acceptance identities differ: %+v", acceptances)
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

func loadRepositoryMutationWorkflowCommand(
	t *testing.T,
	pool *pgxpool.Pool,
	jobID int64,
) queue.RepositoryMutationCommand {
	t.Helper()
	var command queue.RepositoryMutationCommand
	err := pool.QueryRow(t.Context(), `
		SELECT job_id,step_id,generation,step_attempt,worker_id,contract_id,stage_id,
		       source_snapshot_id,patch,patch_sha256
		FROM repository_mutation_operations WHERE job_id=$1
	`, jobID).Scan(
		&command.JobID, &command.StepID, &command.Generation, &command.Attempt, &command.WorkerID,
		&command.ContractID, &command.StageID, &command.SourceSnapshotID,
		&command.Patch, &command.PatchSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(t.Context(), `
		SELECT file_id,path,source_sha256,source_size,expected_sha256,expected_size
		FROM repository_mutation_files
		WHERE operation_id=(SELECT id FROM repository_mutation_operations WHERE job_id=$1)
		ORDER BY ordinal
	`, jobID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var file queue.RepositoryMutationFile
		if err := rows.Scan(
			&file.FileID, &file.Path, &file.SourceSHA256, &file.SourceSize,
			&file.ExpectedSHA256, &file.ExpectedSize,
		); err != nil {
			t.Fatal(err)
		}
		command.ChangedFiles = append(command.ChangedFiles, file)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return command
}
