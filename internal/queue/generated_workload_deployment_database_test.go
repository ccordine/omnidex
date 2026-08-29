package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestGeneratedWorkloadDeploymentEvidenceRailIsExactImmutableAndRecoverable(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixture(t, "exact-rail")
	prepared := fixture.prepare(t, fixture.authority, fixture.verification.ID)
	fixture.reserve(t, fixture.authority, GeneratedWorkloadProjectDeploymentHeadExpectation{})
	replayed := fixture.prepare(t, fixture.authority, fixture.verification.ID)
	if prepared.OperationID != replayed.OperationID || replayed.State != GeneratedWorkloadDeploymentPrepared {
		t.Fatalf("preparation replay=%+v want operation %s", replayed, prepared.OperationID)
	}
	changed := fixture.command
	changed.SourceSnapshotSHA256 = strings.Repeat("9", 64)
	if _, err := fixture.repository.PrepareGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, changed, fixture.verification.ID, fixture.manifest, fixture.rollback,
	); !errors.Is(err, ErrGeneratedWorkloadDeploymentConflict) {
		t.Fatalf("changed command replay error=%v", err)
	}
	applying := GeneratedWorkloadDeploymentTransition{State: GeneratedWorkloadDeploymentApplying}
	firstApply, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command, applying,
	)
	if err != nil {
		t.Fatal(err)
	}
	secondApply, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command, applying,
	)
	if err != nil {
		t.Fatal(err)
	}
	if firstApply.AttemptCount != 1 || secondApply.AttemptCount != 1 {
		t.Fatalf("idempotent applying attempts=%d/%d want 1/1", firstApply.AttemptCount, secondApply.AttemptCount)
	}
	receipt := fixture.executeSuccessfulRail(t, fixture.authority)
	applied, err := fixture.repository.SealGeneratedWorkloadDeploymentApplied(
		fixture.ctx, fixture.authority, fixture.command, receipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	replayedApplied, err := fixture.repository.SealGeneratedWorkloadDeploymentApplied(
		fixture.ctx, fixture.authority, fixture.command, receipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if applied.State != GeneratedWorkloadDeploymentApplied || applied.EvidenceID <= 0 ||
		replayedApplied.EvidenceID != applied.EvidenceID || replayedApplied.ReceiptSHA256 != applied.ReceiptSHA256 {
		t.Fatalf("applied/replayed records disagree: %+v %+v", applied, replayedApplied)
	}
	var executionBound, observationBound bool
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT execution.source_type=$3 AND execution.source_ref=$4 AND
		       execution.payload_json->'metadata'->>'slot'='build' AND
		       execution.payload_json->'metadata'->>'workspace_sha256'=$5 AND
		       NOT (execution.payload_json->'metadata' ? 'workspace'),
		       observation.source_type='docker_compose_observation' AND observation.source_ref=$4 AND
		       observation.payload_json->'metadata'->>'slot'='initial_observe' AND
		       observation.payload_json->'metadata'->>'workspace_sha256'=$5
		FROM evidence AS execution,evidence AS observation
		WHERE execution.id=$1 AND observation.id=$2
	`, receipt.ExecutionEvidenceIDs[0], receipt.ObservationEvidenceIDs[0],
		generatedWorkloadDeploymentExecutionEvidenceSource, applied.OperationID,
		fixture.command.WorkspaceSHA256).Scan(&executionBound, &observationBound); err != nil {
		t.Fatal(err)
	}
	if !executionBound || !observationBound {
		t.Fatal("deployment command or observation evidence lacks exact operation/slot/workspace binding")
	}
	snapshot, err := fixture.repository.GeneratedWorkloadDeploymentEvidence(
		fixture.ctx, fixture.jobID, fixture.authority.Generation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot == nil || snapshot.Verification.ID != fixture.verification.ID ||
		snapshot.Binding.VerificationID != fixture.verification.ID ||
		len(snapshot.Executions) != len(fixture.manifest.Commands) || len(snapshot.Observations) != 2 {
		t.Fatalf("recovery evidence snapshot=%+v", snapshot)
	}
	for _, evidenceID := range []int64{
		fixture.evidenceID, fixture.verification.EvidenceID, receipt.ExecutionEvidenceIDs[0],
		receipt.ObservationEvidenceIDs[0], applied.EvidenceID,
	} {
		if _, err := fixture.pool.Exec(fixture.ctx, `
			UPDATE evidence SET source_ref=source_ref || '-changed' WHERE id=$1
		`, evidenceID); err == nil || !strings.Contains(err.Error(), "cited evidence is immutable") {
			t.Fatalf("evidence %d mutation error=%v", evidenceID, err)
		}
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE generated_workload_deployments SET receipt_json=receipt_json || ' ' WHERE id=$1
	`, applied.OperationID); err == nil || !strings.Contains(err.Error(), "receipt is immutable") {
		t.Fatalf("receipt mutation error=%v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `
		DELETE FROM generated_workload_deployment_observations WHERE operation_id=$1
	`, applied.OperationID); err == nil || !strings.Contains(err.Error(), "evidence rail is immutable") {
		t.Fatalf("observation deletion error=%v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `
		TRUNCATE generated_workload_deployment_observations
	`); err == nil || !strings.Contains(err.Error(), "evidence rail is immutable") {
		t.Fatalf("observation truncate error=%v", err)
	}
	assertGeneratedDeploymentStoredTextIsSecretFree(t, fixture)
}

func TestGeneratedWorkloadDeploymentRejectsStaleAttemptWithoutJournalRow(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixture(t, "stale")
	staleAuthority := fixture.authority
	staleAuthority.WorkerID += "-stale"
	_, err := fixture.repository.PrepareGeneratedWorkloadDeployment(
		fixture.ctx, staleAuthority, fixture.command, fixture.verification.ID, fixture.manifest, fixture.rollback,
	)
	if !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("stale deployment preparation error=%v", err)
	}
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT COUNT(*) FROM generated_workload_deployments WHERE job_id=$1
	`, fixture.jobID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale deployment persisted %d operation rows", count)
	}
}

func TestGeneratedWorkloadDeploymentPrepareAndTransitionShareAuthorityFirstLockOrder(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixture(t, "lock-order")
	fixture.prepare(t, fixture.authority, fixture.verification.ID)
	fixture.reserve(t, fixture.authority, GeneratedWorkloadProjectDeploymentHeadExpectation{})
	ctx, cancel := context.WithTimeout(fixture.ctx, 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	errors := make(chan error, 2)
	go func() {
		<-start
		_, err := fixture.repository.PrepareGeneratedWorkloadDeployment(
			ctx, fixture.authority, fixture.command, fixture.verification.ID, fixture.manifest, fixture.rollback,
		)
		errors <- err
	}()
	go func() {
		<-start
		_, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
			ctx, fixture.authority, fixture.command,
			GeneratedWorkloadDeploymentTransition{State: GeneratedWorkloadDeploymentApplying},
		)
		errors <- err
	}()
	close(start)
	for index := 0; index < 2; index++ {
		if err := <-errors; err != nil {
			t.Fatalf("concurrent authority/deployment lock %d: %v", index, err)
		}
	}
}

func TestGeneratedWorkloadDeploymentReclaimReusesSemanticVerificationAndKeepsStartedExecutionIndeterminate(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixture(t, "reclaim")
	prepared := fixture.prepare(t, fixture.authority, fixture.verification.ID)
	fixture.reserve(t, fixture.authority, GeneratedWorkloadProjectDeploymentHeadExpectation{})
	if _, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command,
		GeneratedWorkloadDeploymentTransition{State: GeneratedWorkloadDeploymentApplying},
	); err != nil {
		t.Fatal(err)
	}
	build := fixture.manifest.Commands[0]
	generatedDeploymentQualifyProtectedExecution(t, fixture, fixture.authority, build)
	started, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, build,
	)
	if err != nil || !created || started.Status != GeneratedWorkloadDeploymentExecutionStarted {
		t.Fatalf("started execution=%+v created=%t err=%v", started, created, err)
	}
	reclaimed := reclaimGeneratedDeploymentAttempt(t, fixture)
	fixture.reserve(t, reclaimed, GeneratedWorkloadProjectDeploymentHeadExpectation{Revision: 0, Fence: 1})
	fresh, _ := recordGeneratedDeploymentVerification(
		t, fixture.repository, fixture.ctx, reclaimed,
		fixture.command, "verify workspace", "docker compose config --hash=*",
	)
	if fresh.ID == fixture.verification.ID {
		t.Fatal("fresh execution evidence unexpectedly reused the original receipt identity")
	}
	replayed, err := fixture.repository.PrepareGeneratedWorkloadDeployment(
		fixture.ctx, reclaimed, fixture.command, fresh.ID, fixture.manifest, fixture.rollback,
	)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.OperationID != prepared.OperationID || replayed.State != GeneratedWorkloadDeploymentApplying {
		t.Fatalf("reclaimed preparation=%+v want applying operation %s", replayed, prepared.OperationID)
	}
	bound, err := fixture.repository.BoundGeneratedWorkloadVerification(
		fixture.ctx, fixture.jobID, fixture.authority.Generation,
	)
	if err != nil || bound == nil || bound.ID != fixture.verification.ID {
		t.Fatalf("bound verification=%+v err=%v", bound, err)
	}
	recovered, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, reclaimed, fixture.command, build,
	)
	if err != nil || created || recovered.Status != GeneratedWorkloadDeploymentExecutionStarted || recovered.EvidenceID != 0 {
		t.Fatalf("recovered execution=%+v created=%t err=%v", recovered, created, err)
	}
	snapshot, err := fixture.repository.GeneratedWorkloadDeploymentEvidence(
		fixture.ctx, fixture.jobID, fixture.authority.Generation,
	)
	if err != nil || snapshot == nil || len(snapshot.Executions) != 1 ||
		snapshot.Executions[0].Status != GeneratedWorkloadDeploymentExecutionStarted {
		t.Fatalf("recovery snapshot=%+v err=%v", snapshot, err)
	}
	detail := generatedDeploymentSHA("side effect may have occurred before completion evidence")
	indeterminate, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, reclaimed, fixture.command, GeneratedWorkloadDeploymentTransition{
			State: GeneratedWorkloadDeploymentIndeterminate,
			Code:  "execution_interrupted", DetailSHA256: detail,
		},
	)
	if err != nil || indeterminate.State != GeneratedWorkloadDeploymentIndeterminate {
		t.Fatalf("indeterminate record=%+v err=%v", indeterminate, err)
	}
}

func reclaimGeneratedDeploymentAttempt(
	t *testing.T, fixture generatedDeploymentDatabaseFixture,
) model.StepAttemptAuthority {
	t.Helper()
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.ctx)
	if _, err := tx.Exec(fixture.ctx, `ALTER TABLE job_step_attempts DISABLE TRIGGER job_step_attempt_change_validate`); err != nil {
		t.Fatal(err)
	}
	expired := time.Now().UTC().Add(-2 * StepAttemptLeaseDuration)
	if _, err := tx.Exec(fixture.ctx, `
		UPDATE job_step_attempts SET claimed_at=$6::TIMESTAMPTZ,renewed_at=$6::TIMESTAMPTZ,
		expires_at=$6::TIMESTAMPTZ+INTERVAL '75 seconds'
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5
	`, fixture.authority.JobID, fixture.authority.Generation, fixture.authority.StepID,
		fixture.authority.Attempt, fixture.authority.WorkerID, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.ctx, `ALTER TABLE job_step_attempts ENABLE TRIGGER job_step_attempt_change_validate`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	claim, err := fixture.repository.ClaimNextStep(fixture.ctx, "deployment-recovery-worker")
	if err != nil || claim == nil || claim.Authority.Attempt != fixture.authority.Attempt+1 {
		t.Fatalf("replacement claim=%+v err=%v", claim, err)
	}
	return claim.Authority
}
