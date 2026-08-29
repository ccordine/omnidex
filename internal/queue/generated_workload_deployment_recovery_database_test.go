package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
)

func TestDeploymentRecoveryPersistsObservationBeforeStartedForwardFence(t *testing.T) {
	fixture := generatedDeploymentApplyingFixture(t, "started-observe-first")
	build := fixture.manifest.Commands[0]
	generatedDeploymentQualifyProtectedExecution(t, fixture, fixture.authority, build)
	if _, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, build,
	); err != nil || !created {
		t.Fatalf("begin started forward execution: created=%t err=%v", created, err)
	}
	detail := generatedDeploymentSHA("exact-started-forward-rows")
	terminal := GeneratedWorkloadDeploymentTransition{
		State: GeneratedWorkloadDeploymentRolledBack,
		Code:  "recovered_side_effect", DetailSHA256: detail,
	}
	observation := generatedDeploymentRecoveryObservation(t, fixture.rollback, false)
	deployment, recorded, err := fixture.repository.RecordGeneratedWorkloadDeploymentPreRollbackObservation(
		fixture.ctx, fixture.authority, fixture.command, fixture.rollback, observation, terminal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.State != GeneratedWorkloadDeploymentIndeterminate ||
		deployment.TerminalCode != "external_quiescence_unproven" ||
		deployment.DetailSHA256 != detail || recorded.Outcome != GeneratedWorkloadDeploymentRollbackClean {
		t.Fatalf("observe-first fence=%+v observation=%+v", deployment, recorded)
	}
	replayed, same, err := fixture.repository.RecordGeneratedWorkloadDeploymentPreRollbackObservation(
		fixture.ctx, fixture.authority, fixture.command, fixture.rollback, observation, terminal,
	)
	if err != nil || replayed.State != GeneratedWorkloadDeploymentIndeterminate ||
		same.EvidenceID != recorded.EvidenceID {
		t.Fatalf("indeterminate observation replay=%+v observation=%+v err=%v", replayed, same, err)
	}
	head, err := fixture.repository.CurrentGeneratedWorkloadProjectDeploymentHead(fixture.ctx, fixture.projectID)
	if err != nil || head == nil || head.Candidate == nil {
		t.Fatalf("started-forward fence released candidate: head=%+v err=%v", head, err)
	}
	var attemptCount, observationCount int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT (SELECT count(*) FROM generated_workload_deployment_rollback_attempts WHERE operation_id=$1),
		       (SELECT count(*) FROM generated_workload_deployment_rollback_observations WHERE operation_id=$1)
	`, deployment.OperationID).Scan(&attemptCount, &observationCount); err != nil {
		t.Fatal(err)
	}
	if attemptCount != 0 || observationCount != 1 {
		t.Fatalf("attempts=%d observations=%d want 0/1", attemptCount, observationCount)
	}
	if _, _, err := fixture.repository.BeginGeneratedWorkloadDeploymentRollbackAttempt(
		fixture.ctx, fixture.authority, fixture.command, fixture.rollback,
	); err == nil {
		t.Fatal("started forward execution authorized a rollback command")
	}
}

func TestDeploymentRecoveryCleanAndResidualConvergeWithoutFakeAttempt(t *testing.T) {
	for _, residual := range []bool{false, true} {
		residual := residual
		name := "clean"
		if residual {
			name = "residual"
		}
		t.Run(name, func(t *testing.T) {
			fixture := generatedDeploymentApplyingFixtureAtAuthorityHardening(t, "pre-observe-"+name)
			generatedDeploymentQualifyAndCompleteProtected(t, fixture, fixture.manifest.Commands[0])
			generatedDeploymentQualifyAndCompleteProtected(t, fixture, fixture.manifest.Commands[1])
			terminal := GeneratedWorkloadDeploymentTransition{
				State: GeneratedWorkloadDeploymentRolledBack,
				Code:  "recovered_side_effect", DetailSHA256: generatedDeploymentSHA(name),
			}
			deployment, _, err := fixture.repository.RecordGeneratedWorkloadDeploymentPreRollbackObservation(
				fixture.ctx, fixture.authority, fixture.command, fixture.rollback,
				generatedDeploymentRecoveryObservation(t, fixture.rollback, residual), terminal,
			)
			if err != nil {
				t.Fatal(err)
			}
			head, err := fixture.repository.CurrentGeneratedWorkloadProjectDeploymentHead(fixture.ctx, fixture.projectID)
			if err != nil {
				t.Fatal(err)
			}
			if !residual {
				if deployment.State != GeneratedWorkloadDeploymentRolledBack || head.Candidate != nil {
					t.Fatalf("clean convergence=%+v head=%+v", deployment, head)
				}
				return
			}
			if deployment.State != GeneratedWorkloadDeploymentIndeterminate || head.Candidate == nil {
				t.Fatalf("residual convergence=%+v head=%+v", deployment, head)
			}
			attempt, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentRollbackAttempt(
				fixture.ctx, fixture.authority, fixture.command, fixture.rollback,
			)
			if err != nil || !created || attempt.StepAttempt != fixture.authority.Attempt {
				t.Fatalf("same-attempt cleanup start=%+v created=%t err=%v", attempt, created, err)
			}
		})
	}
}

func TestDeploymentExecutionOrderingAndTakeoverSealAreFenced(t *testing.T) {
	fixture := generatedDeploymentApplyingFixture(t, "execution-order")
	first, second := fixture.manifest.Commands[0], fixture.manifest.Commands[1]
	if _, _, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, second,
	); err == nil {
		t.Fatal("later manifest slot started before first absent command")
	}
	generatedDeploymentQualifyProtectedExecution(t, fixture, fixture.authority, first)
	if _, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, first,
	); err != nil || !created {
		t.Fatalf("begin first slot: created=%t err=%v", created, err)
	}
	if _, _, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, second,
	); err == nil {
		t.Fatal("later manifest slot started after uncompleted predecessor")
	}
	generatedDeploymentCompleteStartedExecution(t, fixture, first)
	generatedDeploymentQualifyProtectedExecution(t, fixture, fixture.authority, second)
	if _, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, second,
	); err != nil || !created {
		t.Fatalf("begin exact next slot: created=%t err=%v", created, err)
	}

	sealed := generatedDeploymentApplyingFixture(t, "takeover-seal")
	receipt := sealed.executeSuccessfulRail(t, sealed.authority)
	reclaimed := reclaimGeneratedDeploymentAttempt(t, sealed)
	sealed.reserve(t, reclaimed, GeneratedWorkloadProjectDeploymentHeadExpectation{Fence: 1})
	if _, err := sealed.repository.SealGeneratedWorkloadDeploymentApplied(
		sealed.ctx, reclaimed, sealed.command, receipt,
	); err == nil || !strings.Contains(err.Error(), "successful lifecycle manifest") {
		t.Fatalf("takeover sealed predecessor execution evidence: %v", err)
	}
}

func generatedDeploymentApplyingFixture(t *testing.T, label string) generatedDeploymentDatabaseFixture {
	t.Helper()
	fixture := newGeneratedDeploymentDatabaseFixture(t, label)
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

func generatedDeploymentCompleteExecution(
	t *testing.T, fixture generatedDeploymentDatabaseFixture,
	command GeneratedWorkloadDeploymentExecutionCommand,
) {
	t.Helper()
	if _, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, command,
	); err != nil || !created {
		t.Fatalf("begin %s: created=%t err=%v", command.Slot.Name, created, err)
	}
	generatedDeploymentCompleteStartedExecution(t, fixture, command)
}

func generatedDeploymentCompleteStartedExecution(
	t *testing.T, fixture generatedDeploymentDatabaseFixture,
	command GeneratedWorkloadDeploymentExecutionCommand,
) {
	t.Helper()
	result := evidence.Record{
		Kind: evidence.KindCommandOutput, SourceType: "command",
		Command:  generatedDeploymentTestExecutionText(command.Slot),
		Summary:  "Exact deployment command completed.",
		Metadata: map[string]any{"execution": true, "side_effect_possible": true, "succeeded": true},
	}
	if _, err := fixture.repository.CompleteGeneratedWorkloadDeploymentExecution(
		fixture.ctx, fixture.authority, fixture.command, command, result,
	); err != nil {
		t.Fatal(err)
	}
}

func generatedDeploymentRecoveryObservation(
	t *testing.T, plan GeneratedWorkloadDeploymentRollbackPlan, residual bool,
) GeneratedWorkloadDeploymentRollbackObservation {
	t.Helper()
	containers := []string{}
	if residual {
		containers = []string{strings.Repeat("a", 64)}
	}
	observation := GeneratedWorkloadDeploymentRollbackObservation{
		Schema:         GeneratedWorkloadDeploymentRollbackObservationV1,
		ComposeProject: plan.ComposeProject, ContainerIDs: containers,
		NetworkIDs: []string{}, VolumeNames: []string{}, PostconditionSHA256: plan.PostconditionSHA256,
	}
	bound, _, err := BindGeneratedWorkloadDeploymentRollbackObservation(plan, observation)
	if err != nil {
		t.Fatal(err)
	}
	return bound
}
