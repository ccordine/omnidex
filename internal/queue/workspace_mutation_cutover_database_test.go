package queue

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresWorkspaceMutationCutoverRejectsUnresolvedLegacyJournalAtomically(t *testing.T) {
	fixture := newLegacyRepositoryMutationCutoverFixture(t, "unresolved")
	err := fixture.repository.EnsureSchema(
		fixture.ctx, loadMigrationBundleThroughPrefix(t, "159"),
	)
	if err == nil || !strings.Contains(
		err.Error(), "workspace mutation cutover rejects unresolved legacy repository mutation",
	) {
		t.Fatalf("159 unresolved legacy mutation rejection=%v", err)
	}
	assertAppliedMigrationCount(t, fixture.pool, workspaceMutationCutoverMigration, 0)
	assertMigrationRelationExists(t, fixture.pool, "repository_mutation_operations", true)
	assertMigrationRelationExists(t, fixture.pool, "retired_repository_mutation_operations", false)
	assertMigrationRelationExists(t, fixture.pool, "workspace_mutation_operations", false)
	var status string
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT status FROM repository_mutation_operations WHERE id=$1
	`, fixture.operationID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "prepared" {
		t.Fatalf("rejected cutover changed legacy mutation status=%q", status)
	}
}

func TestPostgresWorkspaceMutationCutoverRetiresLegacyAndPersistsVerifiedCrashRecovery(t *testing.T) {
	fixture := newLegacyRepositoryMutationCutoverFixture(t, "verified")
	sealAppliedLegacyRepositoryMutation(t, fixture)
	if err := fixture.repository.EnsureSchema(
		fixture.ctx, loadMigrationBundleThroughPrefix(t, "159"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, fixture.pool, workspaceMutationCutoverMigration, 1)
	assertMigrationRelationExists(t, fixture.pool, "repository_mutation_operations", false)
	assertMigrationRelationExists(t, fixture.pool, "repository_mutation_files", false)
	assertMigrationRelationExists(t, fixture.pool, "retired_repository_mutation_operations", true)
	assertMigrationRelationExists(t, fixture.pool, "retired_repository_mutation_files", true)
	assertMigrationRelationExists(t, fixture.pool, "workspace_mutation_operations", true)
	assertMigrationRelationExists(t, fixture.pool, "workspace_mutation_files", true)
	var legacyStatus string
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT status FROM retired_repository_mutation_operations WHERE id=$1
	`, fixture.operationID).Scan(&legacyStatus); err != nil {
		t.Fatal(err)
	}
	if legacyStatus != "applied" {
		t.Fatalf("retired legacy mutation status=%q want applied", legacyStatus)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE retired_repository_mutation_operations SET last_error='tampered' WHERE id=$1
	`, fixture.operationID); err == nil || !strings.Contains(err.Error(), "retired repository mutation journal is immutable") {
		t.Fatalf("retired legacy mutation update=%v", err)
	}

	root := t.TempDir()
	project, err := fixture.repository.CreateProject(
		fixture.ctx, "empty plain workspace", root, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	marker := fmt.Sprintf("plain-workspace-mutation-%d", time.Now().UnixNano())
	metadata := []byte(fmt.Sprintf(`{"project_id":%d,"client_cwd":%q}`, project.ID, root))
	job, err := fixture.repository.EnqueueJob(
		fixture.ctx, marker, model.PipelineCoding, metadata,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := fixture.repository.ClaimNextStep(fixture.ctx, marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("plain workspace claim=%+v want job %d", claim, job.ID)
	}
	if _, err := fixture.repository.RenewStepAttempt(fixture.ctx, claim.Authority); err != nil {
		t.Fatal(err)
	}

	workspaceID := workspaceCutoverOpaqueID("workspace_", root)
	sourceStateID := "workspace_state_" + workspaceCutoverDigest("empty-source")
	expectedStateID := "workspace_state_" + workspaceCutoverDigest("thirty-two-files")
	ownerID := "desired_graph_" + workspaceCutoverDigest("plain-workspace-owner")
	stageID := "workspace_change_stage_" + workspaceCutoverDigest("plain-workspace-stage")
	patch := strings.Repeat("p", 1024*1024+1)
	patchSHA := workspaceCutoverDigest(patch)
	verificationCommand := "go test ./..."
	verificationCommandSHA := workspaceCutoverDigest(verificationCommand)
	verificationPlan := fmt.Sprintf(
		`{"schema":"omnidex.workspace-mutation-verification-plan.v1","commands":[{"ordinal":1,"kind":"test_result","command":"%s","command_sha256":"%s"}]}`,
		verificationCommand, verificationCommandSHA,
	)
	verificationPlanSHA := workspaceCutoverDigest(verificationPlan)
	commandSHA := workspaceCutoverDigest(strings.Join([]string{
		workspaceID, sourceStateID, expectedStateID, ownerID, stageID,
		patchSHA, verificationPlanSHA,
	}, "\x00"))
	operationID := "workspace_mutation_" + commandSHA

	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.ctx)
	if _, err := tx.Exec(fixture.ctx, `
		INSERT INTO workspace_mutation_operations (
			id,command_sha256,job_id,generation,step_id,
			creator_step_attempt,creator_worker_id,current_step_attempt,current_worker_id,
			project_id,owner_id,stage_id,workspace_id,workspace_root,
			source_state_id,expected_state_id,source_repository_snapshot_id,
			patch,patch_sha256,verification_plan_json,verification_plan_sha256,status
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$6,$7,$8,$9,$10,$11,$12,$13,$14,NULL,
			$15,$16,$17,$18,'prepared'
		)
	`, operationID, commandSHA, job.ID, claim.Step.Generation, claim.Step.ID,
		claim.Authority.Attempt, claim.Authority.WorkerID, project.ID, ownerID, stageID,
		workspaceID, root, sourceStateID, expectedStateID, patch, patchSHA,
		verificationPlan, verificationPlanSHA); err != nil {
		t.Fatal(err)
	}
	for ordinal := 0; ordinal < 32; ordinal++ {
		path := fmt.Sprintf("file-%02d.txt", ordinal)
		fileID := workspaceCutoverOpaqueID("workspace_file_", workspaceID, path)
		expectedSHA := workspaceCutoverDigest("x")
		if _, err := tx.Exec(fixture.ctx, `
			INSERT INTO workspace_mutation_files (
				operation_id,ordinal,file_id,path,
				source_present,source_kind,source_sha256,source_size,source_mode,source_link_target,
				expected_present,expected_kind,expected_sha256,expected_size,expected_mode,expected_link_target
			) VALUES ($1,$2,$3,$4,FALSE,NULL,NULL,NULL,NULL,NULL,TRUE,'regular',$5,1,420,NULL)
		`, operationID, ordinal, fileID, path, expectedSHA); err != nil {
			t.Fatalf("insert workspace mutation file %d: %v", ordinal, err)
		}
	}
	if _, err := tx.Exec(fixture.ctx, `
		UPDATE workspace_mutation_operations
		SET sealed_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1
	`, operationID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var patchBytes, fileCount int
	var sourceGitSnapshot *string
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT octet_length(operation.patch),COUNT(file.*),operation.source_repository_snapshot_id
		FROM workspace_mutation_operations AS operation
		JOIN workspace_mutation_files AS file ON file.operation_id=operation.id
		WHERE operation.id=$1
		GROUP BY operation.id
	`, operationID).Scan(&patchBytes, &fileCount, &sourceGitSnapshot); err != nil {
		t.Fatal(err)
	}
	if patchBytes <= 1024*1024 || fileCount != 32 || sourceGitSnapshot != nil {
		t.Fatalf("generic mutation patch/files/Git=%d/%d/%v", patchBytes, fileCount, sourceGitSnapshot)
	}

	var mutationEvidenceID int64
	if err := fixture.pool.QueryRow(fixture.ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,'generated_diff','workspace',$3,jsonb_build_object(
			'hash',$4::text,'metadata',jsonb_build_object(
				'workspace_mutation_operation_id',$5::text,
				'source_state_id',$6::text,'expected_state_id',$7::text
			)
		)) RETURNING id
	`, job.ID, claim.Step.ID, stageID, patchSHA, operationID,
		sourceStateID, expectedStateID).Scan(&mutationEvidenceID); err != nil {
		t.Fatal(err)
	}
	workspaceMutationTransition(t, fixture, operationID, `
		status='applying',apply_attempt_count=apply_attempt_count+1,
		applying_at=clock_timestamp(),last_error=NULL,indeterminate_phase=NULL
	`)
	workspaceMutationTransition(t, fixture, operationID, `
		status='indeterminate',indeterminate_phase='apply',last_error='simulated apply crash'
	`)
	workspaceMutationTransitionArgs(t, fixture, operationID, `
		status='applied',mutation_evidence_id=$2,applied_at=clock_timestamp(),
		indeterminate_phase=NULL,last_error=NULL
	`, mutationEvidenceID)
	workspaceMutationTransition(t, fixture, operationID, `
		status='verifying',verification_attempt_count=verification_attempt_count+1,
		verifying_at=clock_timestamp(),indeterminate_phase=NULL,last_error=NULL
	`)
	workspaceMutationTransition(t, fixture, operationID, `
		status='indeterminate',indeterminate_phase='verification',last_error='simulated verification crash'
	`)
	workspaceMutationTransition(t, fixture, operationID, `
		status='verifying',verification_attempt_count=verification_attempt_count+1,
		verifying_at=clock_timestamp(),indeterminate_phase=NULL,last_error=NULL
	`)

	var commandEvidenceID int64
	if err := fixture.pool.QueryRow(fixture.ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,'test_result','workspace_verification',$3,jsonb_build_object(
			'command',$4::text,'metadata',jsonb_build_object('succeeded',TRUE)
		)) RETURNING id
	`, job.ID, claim.Step.ID, operationID, verificationCommand).Scan(&commandEvidenceID); err != nil {
		t.Fatal(err)
	}
	receipt := fmt.Sprintf(
		`{"schema":"omnidex.workspace-mutation-verification-receipt.v1","operation_id":"%s","source_state_id":"%s","expected_state_id":"%s","observed_state_id":"%s","succeeded":true,"command_evidence_ids":[%d]}`,
		operationID, sourceStateID, expectedStateID, expectedStateID, commandEvidenceID,
	)
	receiptSHA := workspaceCutoverDigest(receipt)
	var verificationEvidenceID int64
	if err := fixture.pool.QueryRow(fixture.ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,'workspace_verification_receipt','workspace_mutation',$3,jsonb_build_object(
			'hash',$4::text,'excerpt',$5::text,'metadata',jsonb_build_object(
				'workspace_mutation_operation_id',$3::text,
				'observed_state_id',$6::text,'succeeded',TRUE
			)
		)) RETURNING id
	`, job.ID, claim.Step.ID, operationID, receiptSHA, receipt,
		expectedStateID).Scan(&verificationEvidenceID); err != nil {
		t.Fatal(err)
	}
	workspaceMutationTransitionArgs(t, fixture, operationID, `
		status='verified',verification_succeeded=TRUE,
		verification_receipt_json=$2,verification_receipt_sha256=$3,
		verification_evidence_id=$4,terminal_at=clock_timestamp(),last_error=NULL
	`, receipt, receiptSHA, verificationEvidenceID)

	var status string
	var applyAttempts, verificationAttempts int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT status,apply_attempt_count,verification_attempt_count
		FROM workspace_mutation_operations WHERE id=$1
	`, operationID).Scan(&status, &applyAttempts, &verificationAttempts); err != nil {
		t.Fatal(err)
	}
	if status != "verified" || applyAttempts != 1 || verificationAttempts != 2 {
		t.Fatalf("verified crash recovery journal=%s/%d/%d", status, applyAttempts, verificationAttempts)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE workspace_mutation_operations SET last_error='tampered' WHERE id=$1
	`, operationID); err == nil || !strings.Contains(err.Error(), "verified workspace mutation is terminal") {
		t.Fatalf("verified mutation update=%v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE evidence SET payload_json=payload_json || '{"tampered":true}'::jsonb WHERE id=$1
	`, commandEvidenceID); err == nil || !strings.Contains(err.Error(), "workspace mutation cited evidence is immutable") {
		t.Fatalf("workspace mutation evidence update=%v", err)
	}
}

func workspaceMutationTransition(
	t *testing.T,
	fixture legacyRepositoryMutationCutoverFixture,
	operationID, assignments string,
) {
	t.Helper()
	workspaceMutationTransitionArgs(t, fixture, operationID, assignments)
}

func workspaceMutationTransitionArgs(
	t *testing.T,
	fixture legacyRepositoryMutationCutoverFixture,
	operationID, assignments string,
	arguments ...any,
) {
	t.Helper()
	query := "UPDATE workspace_mutation_operations SET " + assignments +
		",updated_at=clock_timestamp() WHERE id=$1"
	values := append([]any{operationID}, arguments...)
	result, err := fixture.pool.Exec(fixture.ctx, query, values...)
	if err != nil {
		t.Fatalf("workspace mutation transition %q: %v", assignments, err)
	}
	if result.RowsAffected() != 1 {
		t.Fatalf("workspace mutation transition %q changed %d rows", assignments, result.RowsAffected())
	}
}
