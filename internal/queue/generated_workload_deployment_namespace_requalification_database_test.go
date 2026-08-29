package queue

import (
	"strings"
	"testing"
)

func TestDeploymentNamespaceRequalificationAuthorizesOnlyExactProtectedExecution(t *testing.T) {
	fixture := generatedDeploymentApplyingFixtureAtNamespaceRequalification(t, "exact-protected")
	build, initialStart := fixture.manifest.Commands[0], fixture.manifest.Commands[1]
	required, err := fixture.repository.GeneratedWorkloadDeploymentNeedsNamespaceRequalification(
		fixture.ctx, fixture.authority, fixture.command, build,
	)
	if err != nil || !required {
		t.Fatalf("build namespace requalification required=%t err=%v", required, err)
	}
	if _, _, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, build,
	); err == nil || !strings.Contains(err.Error(), "lacks exact current-attempt namespace requalification") {
		t.Fatalf("unqualified protected execution error=%v", err)
	}
	assertGeneratedDeploymentExecutionCount(t, fixture, 0)

	proof := generatedDeploymentVacantNamespaceProof(t, fixture.command.ComposeProject)
	recorded, created, err := fixture.repository.RecordGeneratedWorkloadDeploymentNamespaceRequalification(
		fixture.ctx, fixture.authority, fixture.command, build, proof,
	)
	if err != nil || !created || recorded.EvidenceID <= 0 ||
		recorded.StepAttempt != fixture.authority.Attempt || recorded.WorkerID != fixture.authority.WorkerID {
		t.Fatalf("recorded namespace requalification=%+v created=%t err=%v", recorded, created, err)
	}
	replayed, created, err := fixture.repository.RecordGeneratedWorkloadDeploymentNamespaceRequalification(
		fixture.ctx, fixture.authority, fixture.command, build, proof,
	)
	if err != nil || created || replayed.EvidenceID != recorded.EvidenceID ||
		replayed.ProofSHA256 != recorded.ProofSHA256 {
		t.Fatalf("replayed namespace requalification=%+v created=%t err=%v", replayed, created, err)
	}
	started, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, build,
	)
	if err != nil || !created || started.StepAttempt != fixture.authority.Attempt {
		t.Fatalf("qualified build=%+v created=%t err=%v", started, created, err)
	}
	required, err = fixture.repository.GeneratedWorkloadDeploymentNeedsNamespaceRequalification(
		fixture.ctx, fixture.authority, fixture.command, build,
	)
	if err != nil || required {
		t.Fatalf("durable build requalification required=%t err=%v", required, err)
	}
	generatedDeploymentCompleteStartedExecution(t, fixture, build)

	if _, _, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, initialStart,
	); err == nil || !strings.Contains(err.Error(), "lacks exact current-attempt namespace requalification") {
		t.Fatalf("build proof authorized initial_start: %v", err)
	}
	assertGeneratedDeploymentExecutionCount(t, fixture, 1)
	dirty := proof
	dirty.ContainerIDs = []string{strings.Repeat("a", 64)}
	dirty.SHA256 = ""
	if _, _, err := fixture.repository.RecordGeneratedWorkloadDeploymentNamespaceRequalification(
		fixture.ctx, fixture.authority, fixture.command, initialStart, dirty,
	); err == nil || !strings.Contains(err.Error(), "lacks an exact vacant proof") {
		t.Fatalf("dirty namespace requalification error=%v", err)
	}
	assertGeneratedDeploymentExecutionCount(t, fixture, 1)
	if _, created, err := fixture.repository.RecordGeneratedWorkloadDeploymentNamespaceRequalification(
		fixture.ctx, fixture.authority, fixture.command, initialStart, proof,
	); err != nil || !created {
		t.Fatalf("record initial_start namespace proof: created=%t err=%v", created, err)
	}
	if _, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, initialStart,
	); err != nil || !created {
		t.Fatalf("begin qualified initial_start: created=%t err=%v", created, err)
	}

	if _, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE generated_workload_deployment_namespace_requalifications
		SET proof_json=proof_json||' ' WHERE evidence_id=$1
	`, recorded.EvidenceID); err == nil || !strings.Contains(err.Error(), "evidence rail is immutable") {
		t.Fatalf("namespace requalification mutation error=%v", err)
	}
	for name, statement := range map[string]string{
		"null JSON": `UPDATE generated_workload_deployment_namespace_requalifications
		 SET proof_json=NULL WHERE evidence_id=$1`,
		"malformed JSON": `UPDATE generated_workload_deployment_namespace_requalifications
		 SET proof_json='{' WHERE evidence_id=$1`,
		"tampered digest": `UPDATE generated_workload_deployment_namespace_requalifications
		 SET proof_sha256=repeat('0',64) WHERE evidence_id=$1`,
	} {
		if _, err := fixture.pool.Exec(fixture.ctx, statement, recorded.EvidenceID); err == nil {
			t.Fatalf("namespace requalification %s mutation succeeded", name)
		}
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE evidence SET source_ref=source_ref||'-changed' WHERE id=$1
	`, recorded.EvidenceID); err == nil ||
		!strings.Contains(err.Error(), "namespace requalification evidence is immutable") {
		t.Fatalf("namespace requalification evidence mutation error=%v", err)
	}
}

func TestDeploymentRecoveryMustReobserveNamespaceUnderReplacementAttempt(t *testing.T) {
	fixture := generatedDeploymentApplyingFixtureAtNamespaceRequalification(t, "replacement-attempt")
	build := fixture.manifest.Commands[0]
	proof := generatedDeploymentVacantNamespaceProof(t, fixture.command.ComposeProject)
	old, created, err := fixture.repository.RecordGeneratedWorkloadDeploymentNamespaceRequalification(
		fixture.ctx, fixture.authority, fixture.command, build, proof,
	)
	if err != nil || !created {
		t.Fatalf("record predecessor namespace proof: created=%t err=%v", created, err)
	}
	reclaimed := reclaimGeneratedDeploymentAttempt(t, fixture)
	fixture.reserve(t, reclaimed, GeneratedWorkloadProjectDeploymentHeadExpectation{Fence: 1})
	required, err := fixture.repository.GeneratedWorkloadDeploymentNeedsNamespaceRequalification(
		fixture.ctx, reclaimed, fixture.command, build,
	)
	if err != nil || !required {
		t.Fatalf("replacement namespace requalification required=%t err=%v", required, err)
	}
	if _, _, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, reclaimed, fixture.command, build,
	); err == nil || !strings.Contains(err.Error(), "lacks exact current-attempt namespace requalification") {
		t.Fatalf("stale predecessor proof authorized replacement execution: %v", err)
	}
	assertGeneratedDeploymentExecutionCount(t, fixture, 0)
	fresh, created, err := fixture.repository.RecordGeneratedWorkloadDeploymentNamespaceRequalification(
		fixture.ctx, reclaimed, fixture.command, build, proof,
	)
	if err != nil || !created || fresh.EvidenceID == old.EvidenceID ||
		fresh.StepAttempt != reclaimed.Attempt || fresh.WorkerID != reclaimed.WorkerID {
		t.Fatalf("replacement namespace proof=%+v created=%t err=%v", fresh, created, err)
	}
	if _, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, reclaimed, fixture.command, build,
	); err != nil || !created {
		t.Fatalf("fresh replacement proof did not authorize build: created=%t err=%v", created, err)
	}
	var qualifications int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT count(*) FROM generated_workload_deployment_namespace_requalifications
		WHERE operation_id=$1 AND slot_ordinal=$2
	`, fresh.OperationID, build.Slot.Ordinal).Scan(&qualifications); err != nil {
		t.Fatal(err)
	}
	if qualifications != 2 {
		t.Fatalf("durable historical/current namespace qualifications=%d want 2", qualifications)
	}
	var childTriggerDeferrable, childTriggerInitiallyDeferred bool
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT tgdeferrable,tginitdeferred FROM pg_trigger
		WHERE tgname='generated_deployment_head_consistency_from_namespace_requalification'
	`).Scan(&childTriggerDeferrable, &childTriggerInitiallyDeferred); err != nil {
		t.Fatal(err)
	}
	if !childTriggerDeferrable || !childTriggerInitiallyDeferred {
		t.Fatal("namespace requalification child is not attached to migration-144 deferred convergence")
	}
}

func TestDeploymentNamespaceRequalificationUpgradeRejectsLegacyProtectedExecution(t *testing.T) {
	fixture := generatedDeploymentApplyingFixture(t, "requalification-upgrade")
	build := fixture.manifest.Commands[0]
	if _, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, build,
	); err != nil || !created {
		t.Fatalf("create legacy protected execution: created=%t err=%v", created, err)
	}
	err := fixture.repository.EnsureSchema(
		fixture.ctx, loadMigrationBundleThroughPrefix(t, "146"),
	)
	if err == nil || !strings.Contains(err.Error(), "explicit audit of every legacy protected execution") {
		t.Fatalf("legacy protected execution upgrade error=%v", err)
	}
	assertGeneratedDeploymentExecutionCount(t, fixture, 1)
	var qualificationTableExists bool
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT to_regclass('generated_workload_deployment_namespace_requalifications') IS NOT NULL
	`).Scan(&qualificationTableExists); err != nil {
		t.Fatal(err)
	}
	if qualificationTableExists {
		t.Fatal("failed namespace requalification upgrade left a partial table")
	}
}

func TestDeploymentNamespaceRequalificationRejectsNullManifestAuthority(t *testing.T) {
	for _, testCase := range []struct {
		name string
		path []string
	}{
		{name: "slot name", path: []string{"commands", "0", "slot", "name"}},
		{name: "command digest", path: []string{"commands", "0", "command_sha256"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := generatedDeploymentApplyingFixtureAtNamespaceRequalification(
				t, "null-manifest-"+strings.ReplaceAll(testCase.name, " ", "-"),
			)
			operationID := generatedDeploymentOperationID(t, fixture.command)
			corrupt, err := fixture.pool.Begin(fixture.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer corrupt.Rollback(fixture.ctx)
			if _, err := corrupt.Exec(fixture.ctx, `
				ALTER TABLE generated_workload_deployment_verifications
				DISABLE TRIGGER generated_deployment_verification_change_immutable
			`); err != nil {
				t.Fatal(err)
			}
			if _, err := corrupt.Exec(fixture.ctx, `
				WITH changed AS (
				 SELECT jsonb_set(lifecycle_manifest_json::JSONB,$2::TEXT[],'null'::JSONB,false)::TEXT AS manifest
				 FROM generated_workload_deployment_verifications WHERE operation_id=$1
				)
				UPDATE generated_workload_deployment_verifications AS binding
				SET lifecycle_manifest_json=changed.manifest,
				    lifecycle_manifest_sha256=encode(digest(convert_to(changed.manifest,'UTF8'),'sha256'),'hex')
				FROM changed WHERE binding.operation_id=$1
			`, operationID, testCase.path); err != nil {
				t.Fatal(err)
			}
			if _, err := corrupt.Exec(fixture.ctx, `
				ALTER TABLE generated_workload_deployment_verifications
				ENABLE TRIGGER generated_deployment_verification_change_immutable
			`); err != nil {
				t.Fatal(err)
			}
			if err := corrupt.Commit(fixture.ctx); err != nil {
				t.Fatal(err)
			}

			build := fixture.manifest.Commands[0]
			proof := generatedDeploymentVacantNamespaceProof(t, fixture.command.ComposeProject)
			_, proofJSON, err := BindGeneratedWorkloadDeploymentNamespacePreflight(proof)
			if err != nil {
				t.Fatal(err)
			}
			insert, err := fixture.pool.Begin(fixture.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer insert.Rollback(fixture.ctx)
			evidenceID, observedAt, err := insertGeneratedDeploymentNamespaceRequalificationEvidenceTx(
				fixture.ctx, insert, fixture.authority, fixture.command,
				operationID, build, proof, proofJSON,
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = insert.Exec(fixture.ctx, `
				INSERT INTO generated_workload_deployment_namespace_requalifications(
				 operation_id,job_id,generation,step_id,slot_name,slot_ordinal,
				 command_sha256,workspace_sha256,compose_project,step_attempt,worker_id,
				 proof_json,proof_sha256,evidence_id,observed_at)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			`, operationID, fixture.command.Authority.JobID, fixture.command.Authority.Generation,
				fixture.command.Authority.StepID, build.Slot.Name, build.Slot.Ordinal,
				build.CommandSHA256, build.WorkspaceSHA256, fixture.command.ComposeProject,
				fixture.authority.Attempt, fixture.authority.WorkerID, proofJSON,
				proof.SHA256, evidenceID, observedAt)
			if err == nil || !strings.Contains(err.Error(), "namespace requalification authority is invalid") {
				t.Fatalf("null %s manifest authority error=%v", testCase.name, err)
			}
			assertGeneratedDeploymentExecutionCount(t, fixture, 0)
		})
	}
}

func generatedDeploymentApplyingFixtureAtNamespaceRequalification(
	t *testing.T,
	label string,
) generatedDeploymentDatabaseFixture {
	t.Helper()
	fixture := newGeneratedDeploymentDatabaseFixtureAtPrefix(t, label, "146")
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

func generatedDeploymentVacantNamespaceProof(
	t *testing.T,
	project string,
) GeneratedWorkloadDeploymentNamespacePreflight {
	t.Helper()
	proof, _, err := BindGeneratedWorkloadDeploymentNamespacePreflight(
		GeneratedWorkloadDeploymentNamespacePreflight{
			Schema:         GeneratedWorkloadDeploymentNamespacePreflightV1,
			ComposeProject: project, ContainerIDs: []string{}, NetworkIDs: []string{}, VolumeNames: []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return proof
}

func assertGeneratedDeploymentExecutionCount(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	want int,
) {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT count(*) FROM generated_workload_deployment_executions WHERE operation_id=$1
	`, generatedDeploymentOperationID(t, fixture.command)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("durable execution count=%d want %d", count, want)
	}
}
