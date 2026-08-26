package queue

import (
	"strings"
	"testing"
)

func TestDeploymentAuthorityHardeningRejectsUnprotectedNullCommandDigest(t *testing.T) {
	fixture := generatedDeploymentApplyingFixtureAtAuthorityHardening(t, "unprotected-null-digest")
	generatedDeploymentQualifyAndCompleteProtected(t, fixture, fixture.manifest.Commands[0])
	generatedDeploymentQualifyAndCompleteProtected(t, fixture, fixture.manifest.Commands[1])
	corruptGeneratedDeploymentManifestPath(
		t, fixture, []string{"commands", "2", "command_sha256"},
	)
	initialObserve := fixture.manifest.Commands[2]
	_, err := fixture.pool.Exec(fixture.ctx, `
		INSERT INTO generated_workload_deployment_executions(
		 operation_id,slot_name,slot_ordinal,step_attempt,worker_id,
		 command_sha256,workspace_sha256,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,'started')
	`, generatedDeploymentOperationID(t, fixture.command), initialObserve.Slot.Name,
		initialObserve.Slot.Ordinal, fixture.authority.Attempt, fixture.authority.WorkerID,
		strings.Repeat("f", 64), initialObserve.WorkspaceSHA256)
	if err == nil || !strings.Contains(err.Error(), "execution start authority is invalid") {
		t.Fatalf("unprotected null digest execution error=%v", err)
	}
	assertGeneratedDeploymentExecutionCount(t, fixture, 2)
}

func TestDeploymentAuthorityHardeningUpgradeRejectsExistingNullManifest(t *testing.T) {
	fixture := generatedDeploymentApplyingFixtureAtNamespaceRequalification(t, "null-upgrade")
	corruptGeneratedDeploymentManifestPath(
		t, fixture, []string{"commands", "2", "command_sha256"},
	)
	err := fixture.repository.EnsureSchema(
		fixture.ctx, loadMigrationBundleThroughPrefix(t, "147"),
	)
	if err == nil || !strings.Contains(err.Error(), "exactly constructible") {
		t.Fatalf("null-manifest authority upgrade error=%v", err)
	}
	var helperExists bool
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT to_regprocedure('generated_deployment_lifecycle_manifest_valid(text,text)') IS NOT NULL
	`).Scan(&helperExists); err != nil {
		t.Fatal(err)
	}
	if helperExists {
		t.Fatal("failed authority-hardening migration left a partial validator")
	}
}

func TestDeploymentAuthorityHardeningRejectsNumericCommandDigestType(t *testing.T) {
	numericDigest := strings.Repeat("1", 64)
	t.Run("upgrade preflight", func(t *testing.T) {
		fixture := generatedDeploymentApplyingFixtureAtNamespaceRequalification(t, "numeric-digest-upgrade")
		replaceGeneratedDeploymentManifestPath(
			t, fixture, []string{"commands", "2", "command_sha256"}, numericDigest,
		)
		err := fixture.repository.EnsureSchema(
			fixture.ctx, loadMigrationBundleThroughPrefix(t, "147"),
		)
		if err == nil || !strings.Contains(err.Error(), "exactly constructible") {
			t.Fatalf("numeric-digest authority upgrade error=%v", err)
		}
	})

	t.Run("new binding", func(t *testing.T) {
		fixture := generatedDeploymentApplyingFixtureAtAuthorityHardening(t, "numeric-digest-insert")
		tx, err := fixture.pool.Begin(fixture.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(fixture.ctx)
		if _, err := tx.Exec(fixture.ctx, `
			ALTER TABLE generated_workload_deployment_verifications
			DISABLE TRIGGER generated_deployment_verification_change_immutable
		`); err != nil {
			t.Fatal(err)
		}
		var manifest string
		if err := tx.QueryRow(fixture.ctx, `
			SELECT jsonb_set(
			 lifecycle_manifest_json::JSONB,ARRAY['commands','2','command_sha256'],$2::JSONB,false
			)::TEXT
			FROM generated_workload_deployment_verifications WHERE operation_id=$1
		`, generatedDeploymentOperationID(t, fixture.command), numericDigest).Scan(&manifest); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(fixture.ctx, `
			DELETE FROM generated_workload_deployment_verifications WHERE operation_id=$1
		`, generatedDeploymentOperationID(t, fixture.command)); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(fixture.ctx, `
			INSERT INTO generated_workload_deployment_verifications(
			 operation_id,verification_id,workspace_sha256,
			 lifecycle_manifest_json,lifecycle_manifest_sha256)
			VALUES($1,$2,$3,$4,encode(digest(convert_to($4,'UTF8'),'sha256'),'hex'))
		`, generatedDeploymentOperationID(t, fixture.command), fixture.verification.ID,
			fixture.command.WorkspaceSHA256, manifest)
		if err == nil || !strings.Contains(err.Error(), "binding authority is invalid") {
			t.Fatalf("numeric-digest binding insert error=%v", err)
		}
	})
}

func TestDeploymentAuthorityHardeningRejectsResolvedConfigScalarTypes(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		path        []string
		replacement string
	}{
		{
			name: "numeric service digest", path: []string{"service_hashes", "0", "sha256"},
			replacement: strings.Repeat("1", 64),
		},
		{name: "string boolean", path: []string{"implicit_env_disabled"}, replacement: `"true"`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := generatedDeploymentApplyingFixtureAtAuthorityHardening(
				t, "config-scalar-"+strings.ReplaceAll(testCase.name, " ", "-"),
			)
			tx, err := fixture.pool.Begin(fixture.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(fixture.ctx)
			if _, err := tx.Exec(fixture.ctx, `
				ALTER TABLE generated_workload_deployment_verifications
				DISABLE TRIGGER generated_deployment_verification_change_immutable
			`); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(fixture.ctx, `
				ALTER TABLE evidence DISABLE TRIGGER generated_deployment_evidence_immutable
			`); err != nil {
				t.Fatal(err)
			}
			operationID := generatedDeploymentOperationID(t, fixture.command)
			var manifest, manifestSHA string
			if err := tx.QueryRow(fixture.ctx, `
				DELETE FROM generated_workload_deployment_verifications WHERE operation_id=$1
				RETURNING lifecycle_manifest_json,lifecycle_manifest_sha256
			`, operationID).Scan(&manifest, &manifestSHA); err != nil {
				t.Fatal(err)
			}
			configEvidenceID := fixture.verification.CommandEvidenceIDs[len(fixture.verification.CommandEvidenceIDs)-1]
			if _, err := tx.Exec(fixture.ctx, `
				UPDATE evidence SET payload_json=jsonb_set(
				 payload_json,ARRAY['metadata']||$2::TEXT[],$3::JSONB,false
				) WHERE id=$1
			`, configEvidenceID, testCase.path, testCase.replacement); err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(fixture.ctx, `
				INSERT INTO generated_workload_deployment_verifications(
				 operation_id,verification_id,workspace_sha256,
				 lifecycle_manifest_json,lifecycle_manifest_sha256)
				VALUES($1,$2,$3,$4,$5)
			`, operationID, fixture.verification.ID, fixture.command.WorkspaceSHA256,
				manifest, manifestSHA)
			if err == nil || !strings.Contains(err.Error(), "resolved config proof is invalid") {
				t.Fatalf("resolved-config scalar binding error=%v", err)
			}
		})
	}
}

func TestDeploymentAuthorityHardeningUpgradeRejectsLegacyResolvedConfigScalar(t *testing.T) {
	fixture := generatedDeploymentApplyingFixtureAtNamespaceRequalification(t, "config-scalar-upgrade")
	replaceGeneratedResolvedConfigMetadataPath(
		t, fixture, []string{"service_hashes", "0", "sha256"}, strings.Repeat("1", 64),
	)
	err := fixture.repository.EnsureSchema(
		fixture.ctx, loadMigrationBundleThroughPrefix(t, "147"),
	)
	if err == nil || !strings.Contains(err.Error(), "exactly typed resolved-config evidence") {
		t.Fatalf("legacy resolved-config scalar upgrade error=%v", err)
	}
}

func TestDeploymentAuthorityHardeningUpgradeRejectsLegacyUnownedRollbackAttempt(t *testing.T) {
	fixture := generatedDeploymentApplyingFixtureAtNamespaceRequalification(t, "unowned-rollback-upgrade")
	if err := insertGeneratedDeploymentRollbackAttemptTxDirect(t, fixture); err != nil {
		t.Fatalf("create legacy build-free rollback attempt: %v", err)
	}
	err := fixture.repository.EnsureSchema(
		fixture.ctx, loadMigrationBundleThroughPrefix(t, "147"),
	)
	if err == nil || !strings.Contains(err.Error(), "initial_start execution ownership") {
		t.Fatalf("unowned rollback authority upgrade error=%v", err)
	}
	var attempts int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT count(*) FROM generated_workload_deployment_rollback_attempts WHERE operation_id=$1
	`, generatedDeploymentOperationID(t, fixture.command)).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("failed hardening upgrade changed legacy rollback attempts=%d", attempts)
	}
}

func TestDeploymentRollbackAttemptRequiresDurableInitialStart(t *testing.T) {
	fixture := generatedDeploymentApplyingFixtureAtAuthorityHardening(t, "rollback-ownership")
	generatedDeploymentQualifyAndCompleteProtected(t, fixture, fixture.manifest.Commands[0])
	if _, _, err := fixture.repository.BeginGeneratedWorkloadDeploymentRollbackAttempt(
		fixture.ctx, fixture.authority, fixture.command, fixture.rollback,
	); err == nil || !strings.Contains(err.Error(), "requires a durable initial_start execution") {
		t.Fatalf("build-only repository rollback error=%v", err)
	}
	if err := insertGeneratedDeploymentRollbackAttemptTxDirect(t, fixture); err == nil ||
		!strings.Contains(err.Error(), "rollback attempt authority is invalid") {
		t.Fatalf("build-only SQL rollback error=%v", err)
	}
	var attempts int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT count(*) FROM generated_workload_deployment_rollback_attempts WHERE operation_id=$1
	`, generatedDeploymentOperationID(t, fixture.command)).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Fatalf("build-only rollback attempts=%d want 0", attempts)
	}
	generatedDeploymentQualifyAndCompleteProtected(t, fixture, fixture.manifest.Commands[1])
	if _, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentRollbackAttempt(
		fixture.ctx, fixture.authority, fixture.command, fixture.rollback,
	); err != nil || !created {
		t.Fatalf("initial_start-owned rollback created=%t err=%v", created, err)
	}
}

func TestDeploymentPreAttemptResidualCapAllowsOneLaterCleanConvergence(t *testing.T) {
	fixture := generatedDeploymentApplyingFixtureAtAuthorityHardening(t, "clean-after-cap")
	generatedDeploymentQualifyAndCompleteProtected(t, fixture, fixture.manifest.Commands[0])
	terminal := GeneratedWorkloadDeploymentTransition{
		State: GeneratedWorkloadDeploymentRolledBack,
		Code:  "recovered_side_effect", DetailSHA256: generatedDeploymentSHA("clean-after-cap"),
	}
	for observationNumber := 1; observationNumber <= fixture.rollback.MaxAttempts; observationNumber++ {
		deployment, observation, err := fixture.repository.RecordGeneratedWorkloadDeploymentPreRollbackObservation(
			fixture.ctx, fixture.authority, fixture.command, fixture.rollback,
			generatedDeploymentRecoveryObservation(t, fixture.rollback, true), terminal,
		)
		if err != nil || deployment.State != GeneratedWorkloadDeploymentIndeterminate ||
			observation.Outcome != GeneratedWorkloadDeploymentRollbackResidual {
			t.Fatalf("residual observation %d deployment=%+v observation=%+v err=%v",
				observationNumber, deployment, observation, err)
		}
		if observationNumber < fixture.rollback.MaxAttempts {
			fixture = reclaimAndReserveGeneratedDeploymentFixture(t, fixture, int64(observationNumber))
		}
	}
	fixture = reclaimAndReserveGeneratedDeploymentFixture(
		t, fixture, int64(fixture.rollback.MaxAttempts),
	)
	if _, _, err := fixture.repository.RecordGeneratedWorkloadDeploymentPreRollbackObservation(
		fixture.ctx, fixture.authority, fixture.command, fixture.rollback,
		generatedDeploymentRecoveryObservation(t, fixture.rollback, true), terminal,
	); err == nil || !strings.Contains(err.Error(), "residual rollback observation authority is exhausted") {
		t.Fatalf("post-cap residual observation error=%v", err)
	}
	deployment, observation, err := fixture.repository.RecordGeneratedWorkloadDeploymentPreRollbackObservation(
		fixture.ctx, fixture.authority, fixture.command, fixture.rollback,
		generatedDeploymentRecoveryObservation(t, fixture.rollback, false), terminal,
	)
	if err != nil || deployment.State != GeneratedWorkloadDeploymentRolledBack ||
		observation.Outcome != GeneratedWorkloadDeploymentRollbackClean {
		t.Fatalf("post-cap clean deployment=%+v observation=%+v err=%v", deployment, observation, err)
	}
	var observationCount int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT count(*) FROM generated_workload_deployment_rollback_observations WHERE operation_id=$1
	`, deployment.OperationID).Scan(&observationCount); err != nil {
		t.Fatal(err)
	}
	if observationCount != fixture.rollback.MaxAttempts+1 {
		t.Fatalf("terminal observation count=%d want %d", observationCount, fixture.rollback.MaxAttempts+1)
	}
	head, err := fixture.repository.CurrentGeneratedWorkloadProjectDeploymentHead(
		fixture.ctx, fixture.projectID,
	)
	if err != nil || head == nil || head.Candidate != nil {
		t.Fatalf("clean convergence head=%+v err=%v", head, err)
	}
}

func TestDeploymentPreAttemptCapRejectsCleanWhileForwardExecutionIsStarted(t *testing.T) {
	fixture := generatedDeploymentApplyingFixtureAtAuthorityHardening(t, "started-clean-cap")
	build := fixture.manifest.Commands[0]
	proof := generatedDeploymentVacantNamespaceProof(t, fixture.command.ComposeProject)
	if _, created, err := fixture.repository.RecordGeneratedWorkloadDeploymentNamespaceRequalification(
		fixture.ctx, fixture.authority, fixture.command, build, proof,
	); err != nil || !created {
		t.Fatalf("qualify started build: created=%t err=%v", created, err)
	}
	if _, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, build,
	); err != nil || !created {
		t.Fatalf("begin started build: created=%t err=%v", created, err)
	}
	terminal := GeneratedWorkloadDeploymentTransition{
		State: GeneratedWorkloadDeploymentRolledBack,
		Code:  "recovered_side_effect", DetailSHA256: generatedDeploymentSHA("started-forward-detail"),
	}
	for observationNumber := 1; observationNumber <= fixture.rollback.MaxAttempts; observationNumber++ {
		deployment, _, err := fixture.repository.RecordGeneratedWorkloadDeploymentPreRollbackObservation(
			fixture.ctx, fixture.authority, fixture.command, fixture.rollback,
			generatedDeploymentRecoveryObservation(t, fixture.rollback, true), terminal,
		)
		if err != nil || deployment.State != GeneratedWorkloadDeploymentIndeterminate ||
			deployment.TerminalCode != "external_quiescence_unproven" {
			t.Fatalf("started residual observation %d deployment=%+v err=%v", observationNumber, deployment, err)
		}
		if observationNumber < fixture.rollback.MaxAttempts {
			fixture = reclaimAndReserveGeneratedDeploymentFixture(t, fixture, int64(observationNumber))
		}
	}
	fixture = reclaimAndReserveGeneratedDeploymentFixture(
		t, fixture, int64(fixture.rollback.MaxAttempts),
	)
	clean := generatedDeploymentRecoveryObservation(t, fixture.rollback, false)
	if _, _, err := fixture.repository.RecordGeneratedWorkloadDeploymentPreRollbackObservation(
		fixture.ctx, fixture.authority, fixture.command, fixture.rollback, clean, terminal,
	); err == nil || !strings.Contains(err.Error(), "residual rollback observation authority is exhausted") {
		t.Fatalf("started post-cap clean repository error=%v", err)
	}
	operationID := generatedDeploymentOperationID(t, fixture.command)
	observationJSON, outcome, err := canonicalGeneratedDeploymentRollbackObservation(fixture.rollback, clean)
	if err != nil || outcome != GeneratedWorkloadDeploymentRollbackClean {
		t.Fatalf("canonical clean outcome=%s err=%v", outcome, err)
	}
	payload, err := generatedDeploymentRollbackObservationEvidence(
		fixture.command, operationID, -fixture.authority.Attempt,
		GeneratedWorkloadDeploymentRollbackObservationPreAttempt,
		observationJSON, outcome, clean,
	)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.ctx)
	var evidenceID int64
	if err := tx.QueryRow(fixture.ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,'deployment_observation',$3,$4,$5::JSONB) RETURNING id
	`, fixture.authority.JobID, fixture.authority.StepID,
		generatedWorkloadDeploymentRollbackObservationSource, operationID, payload).Scan(&evidenceID); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(fixture.ctx, `
		INSERT INTO generated_workload_deployment_rollback_observations(
		 operation_id,rollback_step_attempt,observer_job_id,observer_generation,
		 observer_step_id,observer_step_attempt,observer_worker_id,basis,outcome,
		 observation_json,observation_sha256,evidence_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,'pre_attempt','clean',$8,$9,$10)
	`, operationID, -fixture.authority.Attempt, fixture.authority.JobID,
		fixture.authority.Generation, fixture.authority.StepID, fixture.authority.Attempt,
		fixture.authority.WorkerID, observationJSON, clean.SHA256, evidenceID)
	if err == nil || !strings.Contains(err.Error(), "rollback observation authority is invalid") {
		t.Fatalf("started post-cap clean SQL error=%v", err)
	}
	if err := tx.Rollback(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var observationCount int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT count(*) FROM generated_workload_deployment_rollback_observations WHERE operation_id=$1
	`, operationID).Scan(&observationCount); err != nil {
		t.Fatal(err)
	}
	if observationCount != fixture.rollback.MaxAttempts {
		t.Fatalf("started capped observations=%d want %d", observationCount, fixture.rollback.MaxAttempts)
	}
}

func generatedDeploymentApplyingFixtureAtAuthorityHardening(
	t *testing.T,
	label string,
) generatedDeploymentDatabaseFixture {
	t.Helper()
	fixture := newGeneratedDeploymentDatabaseFixtureAtPrefix(t, label, "147")
	fixture.prepare(t, fixture.authority, fixture.verification.ID)
	fixture.reserve(t, fixture.authority, GeneratedWorkloadProjectDeploymentHeadExpectation{})
	if _, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command,
		GeneratedWorkloadDeploymentTransition{State: GeneratedWorkloadDeploymentApplying},
	); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func generatedDeploymentQualifyAndCompleteProtected(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	execution GeneratedWorkloadDeploymentExecutionCommand,
) {
	t.Helper()
	proof := generatedDeploymentVacantNamespaceProof(t, fixture.command.ComposeProject)
	if _, created, err := fixture.repository.RecordGeneratedWorkloadDeploymentNamespaceRequalification(
		fixture.ctx, fixture.authority, fixture.command, execution, proof,
	); err != nil || !created {
		t.Fatalf("qualify %s namespace: created=%t err=%v", execution.Slot.Name, created, err)
	}
	generatedDeploymentCompleteExecution(t, fixture, execution)
}

func corruptGeneratedDeploymentManifestPath(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	path []string,
) {
	t.Helper()
	replaceGeneratedDeploymentManifestPath(t, fixture, path, "null")
}

func replaceGeneratedDeploymentManifestPath(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	path []string,
	replacementJSON string,
) {
	t.Helper()
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.ctx)
	if _, err := tx.Exec(fixture.ctx, `
		ALTER TABLE generated_workload_deployment_verifications
		DISABLE TRIGGER generated_deployment_verification_change_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.ctx, `
		WITH changed AS (
		 SELECT jsonb_set(lifecycle_manifest_json::JSONB,$2::TEXT[],$3::JSONB,false)::TEXT AS manifest
		 FROM generated_workload_deployment_verifications WHERE operation_id=$1
		)
		UPDATE generated_workload_deployment_verifications AS binding
		SET lifecycle_manifest_json=changed.manifest,
		    lifecycle_manifest_sha256=encode(digest(convert_to(changed.manifest,'UTF8'),'sha256'),'hex')
		FROM changed WHERE binding.operation_id=$1
	`, generatedDeploymentOperationID(t, fixture.command), path, replacementJSON); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.ctx, `
		ALTER TABLE generated_workload_deployment_verifications
		ENABLE TRIGGER generated_deployment_verification_change_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatal(err)
	}
}

func insertGeneratedDeploymentRollbackAttemptTxDirect(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
) error {
	t.Helper()
	_, err := fixture.pool.Exec(fixture.ctx, `
		INSERT INTO generated_workload_deployment_rollback_attempts(
		 operation_id,job_id,generation,step_id,step_attempt,worker_id,
		 command_sha256,workspace_sha256,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,'started')
	`, generatedDeploymentOperationID(t, fixture.command), fixture.authority.JobID,
		fixture.authority.Generation, fixture.authority.StepID, fixture.authority.Attempt,
		fixture.authority.WorkerID, fixture.rollback.Execution.CommandSHA256,
		fixture.rollback.Execution.WorkspaceSHA256)
	return err
}

func replaceGeneratedResolvedConfigMetadataPath(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	path []string,
	replacementJSON string,
) {
	t.Helper()
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.ctx)
	if _, err := tx.Exec(fixture.ctx, `
		ALTER TABLE evidence DISABLE TRIGGER generated_deployment_evidence_immutable
	`); err != nil {
		t.Fatal(err)
	}
	configEvidenceID := fixture.verification.CommandEvidenceIDs[len(fixture.verification.CommandEvidenceIDs)-1]
	if _, err := tx.Exec(fixture.ctx, `
		UPDATE evidence SET payload_json=jsonb_set(
		 payload_json,ARRAY['metadata']||$2::TEXT[],$3::JSONB,false
		) WHERE id=$1
	`, configEvidenceID, path, replacementJSON); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.ctx, `
		ALTER TABLE evidence ENABLE TRIGGER generated_deployment_evidence_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatal(err)
	}
}

func reclaimAndReserveGeneratedDeploymentFixture(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	wantFence int64,
) generatedDeploymentDatabaseFixture {
	t.Helper()
	reclaimed := reclaimGeneratedDeploymentAttempt(t, fixture)
	fixture.reserve(t, reclaimed, GeneratedWorkloadProjectDeploymentHeadExpectation{Fence: wantFence})
	fixture.authority = reclaimed
	return fixture
}
