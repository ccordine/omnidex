package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func TestDeploymentRecoveryUpgradeRejectsDirtyTerminalCandidate(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixtureAtPrefix(t, "upgrade-dirty-candidate", "143")
	operationID := prepareGeneratedDeploymentAtPrefix143(t, fixture)
	fixture.reserve(t, fixture.authority, GeneratedWorkloadProjectDeploymentHeadExpectation{})
	if _, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE generated_workload_deployments
		SET status='failed',terminal_code='legacy_failure',terminal_detail_sha256=$2,
		    terminal_at=clock_timestamp(),updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
		WHERE id=$1
	`, operationID, generatedDeploymentSHA("legacy terminal candidate")); err != nil {
		t.Fatal(err)
	}
	err := fixture.repository.EnsureSchema(
		fixture.ctx, loadMigrationBundleThroughPrefix(t, "144"),
	)
	if err == nil || !strings.Contains(err.Error(), "explicit recovery of every pre-rail project candidate") {
		t.Fatalf("dirty terminal candidate upgrade error=%v", err)
	}
	var candidate string
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT candidate_deployment_id FROM generated_workload_project_deployment_heads WHERE project_id=$1
	`, fixture.projectID).Scan(&candidate); err != nil {
		t.Fatal(err)
	}
	if candidate != operationID {
		t.Fatalf("failed migration altered dirty candidate=%q want %q", candidate, operationID)
	}
}

func TestDeploymentRecoveryUpgradeRejectsAppliedPredecessorExecutionOwnership(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixtureAtPrefix(t, "upgrade-owner-mismatch", "143")
	operationID := prepareGeneratedDeploymentAtPrefix143(t, fixture)
	fixture.reserve(t, fixture.authority, GeneratedWorkloadProjectDeploymentHeadExpectation{})
	installPrefix143CleanupReadStubs(t, fixture)
	if _, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE generated_workload_deployments
		SET status='applying',attempt_count=attempt_count+1,applying_at=clock_timestamp(),
		    updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
		WHERE id=$1
	`, operationID); err != nil {
		t.Fatal(err)
	}
	receipt := fixture.executeSuccessfulRail(t, fixture.authority)
	reclaimed := reclaimGeneratedDeploymentAttempt(t, fixture)
	fixture.reserve(t, reclaimed, GeneratedWorkloadProjectDeploymentHeadExpectation{Fence: 1})
	sealGeneratedDeploymentAtPrefix143(t, fixture, reclaimed, receipt)
	removePrefix143CleanupReadStubs(t, fixture)
	var mismatched bool
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT EXISTS(SELECT 1 FROM generated_workload_deployment_executions
		 WHERE operation_id=$1 AND ROW(step_attempt,worker_id) IS DISTINCT FROM ROW($2,$3))
	`, operationID, reclaimed.Attempt, reclaimed.WorkerID).Scan(&mismatched); err != nil {
		t.Fatal(err)
	}
	if !mismatched {
		t.Fatal("prefix-143 fixture did not retain predecessor execution ownership")
	}
	err := fixture.repository.EnsureSchema(
		fixture.ctx, loadMigrationBundleThroughPrefix(t, "144"),
	)
	if err == nil || !strings.Contains(err.Error(), "applied predecessor execution ownership") {
		t.Fatalf("legacy applied owner mismatch upgrade error=%v", err)
	}
}

func prepareGeneratedDeploymentAtPrefix143(
	t *testing.T, fixture generatedDeploymentDatabaseFixture,
) string {
	t.Helper()
	identity, err := generatedWorkloadDeploymentOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, manifestSHA, err := canonicalGeneratedDeploymentLifecycleManifest(
		fixture.command, fixture.manifest,
	)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.ctx)
	if _, err := tx.Exec(fixture.ctx, `
		INSERT INTO generated_workload_deployments(
		 id,command_sha256,command_json,job_id,generation,step_id,
		 creator_step_attempt,creator_worker_id,current_step_attempt,current_worker_id,
		 project_id,compose_project,bind_host,endpoint_port_authority,
		 requested_endpoint_port,prior_deployment_id,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,''),'prepared')
	`, identity.OperationID, identity.CommandSHA256, identity.CommandJSON,
		fixture.command.Authority.JobID, fixture.command.Authority.Generation,
		fixture.command.Authority.StepID, fixture.authority.Attempt, fixture.authority.WorkerID,
		fixture.command.Authority.ProjectID, fixture.command.ComposeProject, fixture.command.BindHost,
		fixture.command.EndpointPortAuthority, fixture.command.EndpointPort,
		fixture.command.PriorDeploymentID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.ctx, `
		INSERT INTO generated_workload_deployment_verifications(
		 operation_id,verification_id,workspace_sha256,lifecycle_manifest_json,lifecycle_manifest_sha256)
		VALUES($1,$2,$3,$4,$5)
	`, identity.OperationID, fixture.verification.ID, fixture.command.WorkspaceSHA256,
		manifestJSON, manifestSHA); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	return identity.OperationID
}

func installPrefix143CleanupReadStubs(
	t *testing.T, fixture generatedDeploymentDatabaseFixture,
) {
	t.Helper()
	for _, statement := range []string{
		`CREATE TABLE generated_workload_deployment_rollback_attempts(operation_id TEXT NOT NULL)`,
		`CREATE TABLE generated_workload_deployment_rollback_observations(operation_id TEXT NOT NULL)`,
	} {
		if _, err := fixture.pool.Exec(fixture.ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
}

func removePrefix143CleanupReadStubs(
	t *testing.T, fixture generatedDeploymentDatabaseFixture,
) {
	t.Helper()
	if _, err := fixture.pool.Exec(fixture.ctx, `
		DROP TABLE generated_workload_deployment_rollback_observations,
		           generated_workload_deployment_rollback_attempts
	`); err != nil {
		t.Fatal(err)
	}
}

func sealGeneratedDeploymentAtPrefix143(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	authority model.StepAttemptAuthority,
	receipt GeneratedWorkloadDeploymentReceipt,
) {
	t.Helper()
	identity, err := generatedWorkloadDeploymentOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	receiptJSON, receiptSHA, err := canonicalGeneratedWorkloadDeploymentReceipt(
		fixture.command, receipt, identity,
	)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.pool.BeginTx(fixture.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.ctx)
	evidenceID, err := insertGeneratedDeploymentEvidenceTx(
		fixture.ctx, tx, fixture.command, receipt, receiptJSON, receiptSHA,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.ctx, `
		UPDATE generated_workload_deployments
		SET status='applied',terminal_code=NULL,terminal_detail_sha256=NULL,
		    receipt_json=$2,receipt_sha256=$3,evidence_id=$4,healthy_endpoint_port=$5,
		    applied_at=$6,observed_at=$7,current_step_attempt=$8,current_worker_id=$9,
		    terminal_at=NULL,updated_at=clock_timestamp()
		WHERE id=$1
	`, identity.OperationID, receiptJSON, receiptSHA, evidenceID, receipt.EndpointPort,
		receipt.AppliedAt, receipt.ObservedAt, authority.Attempt, authority.WorkerID); err != nil {
		t.Fatal(err)
	}
	if _, err := sealGeneratedWorkloadProjectDeploymentHeadTx(
		fixture.ctx, tx, authority, fixture.command, receipt,
	); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatal(err)
	}
}
