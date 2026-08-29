package queue

import (
	"errors"
	"strings"
	"testing"
)

func TestGeneratedWorkloadProjectDeploymentHeadFirstSeal(t *testing.T) {
	fixture := newGeneratedProjectDeploymentHeadFixture(t, "first-seal")
	reservation, head := reserveSealAndPromoteGeneratedProjectDeployment(
		t, fixture, GeneratedWorkloadProjectDeploymentHeadExpectation{},
	)
	if reservation.Revision != 0 || reservation.Fence != 1 ||
		head.Revision != 1 || head.Fence != 1 || head.Candidate != nil ||
		head.ActiveDeploymentID != reservation.DeploymentID ||
		head.ComposeProject != fixture.command.ComposeProject ||
		head.SecretGeneration != generatedProjectDeploymentTestSecretGeneration ||
		head.DeploymentKeyFingerprintSHA256 != generatedProjectDeploymentTestKeyFingerprint ||
		head.Endpoint == nil || head.Endpoint.Port != fixture.receipt.EndpointPort {
		t.Fatalf("reservation=%+v head=%+v", reservation, head)
	}
	loaded, err := fixture.repository.CurrentGeneratedWorkloadProjectDeploymentHead(
		fixture.ctx, fixture.projectID,
	)
	if err != nil || loaded == nil || loaded.ActiveDeploymentID != head.ActiveDeploymentID {
		t.Fatalf("loaded head=%+v err=%v", loaded, err)
	}
	deployment, err := fixture.repository.CurrentGeneratedWorkloadDeployment(
		fixture.ctx, fixture.jobID, fixture.authority.Generation,
	)
	if err != nil || deployment == nil || deployment.Receipt == nil {
		t.Fatalf("sealed deployment=%+v err=%v", deployment, err)
	}
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := sealGeneratedWorkloadProjectDeploymentHeadTx(
		fixture.ctx, tx, fixture.authority, fixture.command, *deployment.Receipt,
	)
	if err != nil {
		_ = tx.Rollback(fixture.ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if replayed.Revision != head.Revision || replayed.Fence != head.Fence {
		t.Fatalf("seal replay changed head: before=%+v after=%+v", head, replayed)
	}
	assertGeneratedProjectDeploymentHeadHistory(t, fixture, []string{"reserved", "promoted"})
}

func TestGeneratedWorkloadProjectDeploymentHeadRejectsCrossJobSuccessorWithoutCutoverRail(t *testing.T) {
	first := newGeneratedProjectDeploymentHeadFixture(t, "cross-job-first")
	_, firstHead := reserveSealAndPromoteGeneratedProjectDeployment(
		t, first, GeneratedWorkloadProjectDeploymentHeadExpectation{},
	)
	project, err := first.repository.GetProject(first.ctx, first.projectID)
	if err != nil {
		t.Fatal(err)
	}
	second := newGeneratedProjectDeploymentJobFixture(
		t, first.repository, first.pool, first.ctx, first.projectID, project.Location,
		"cross-job-second", firstHead.ComposeProject, firstHead.ActiveDeploymentID,
		firstHead.Endpoint.Port,
	)
	_, err = second.repository.PrepareGeneratedWorkloadDeployment(
		second.ctx, second.authority, second.command, second.verification.ID,
		second.manifest, second.rollback,
	)
	if err == nil || !strings.Contains(err.Error(), "cannot target a successor deployment") {
		t.Fatalf("successor without cutover rail error=%v", err)
	}
	loaded, err := first.repository.CurrentGeneratedWorkloadProjectDeploymentHead(
		first.ctx, first.projectID,
	)
	if err != nil || loaded == nil || loaded.ActiveDeploymentID != firstHead.ActiveDeploymentID ||
		loaded.Revision != firstHead.Revision || loaded.Fence != firstHead.Fence || loaded.Candidate != nil {
		t.Fatalf("rejected successor changed active head: head=%+v err=%v", loaded, err)
	}
	assertGeneratedProjectDeploymentHeadHistory(
		t, first, []string{"reserved", "promoted"},
	)
}

func TestGeneratedWorkloadProjectDeploymentHeadRejectsStaleCandidateAndFence(t *testing.T) {
	fixture := newGeneratedProjectDeploymentHeadFixture(t, "stale-fence")
	fixture.prepare(t, fixture.authority, fixture.verification.ID)
	_, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command,
		GeneratedWorkloadDeploymentTransition{State: GeneratedWorkloadDeploymentApplying},
	)
	if err == nil || !strings.Contains(err.Error(), "fenced project candidate authority") {
		t.Fatalf("unreserved deployment transition error=%v", err)
	}
	reservation, err := fixture.repository.ReserveGeneratedWorkloadProjectDeploymentCandidate(
		fixture.ctx, fixture.authority, fixture.command,
		generatedProjectDeploymentTestSecretGeneration,
		generatedProjectDeploymentTestKeyFingerprint,
		GeneratedWorkloadProjectDeploymentHeadExpectation{},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.repository.ReserveGeneratedWorkloadProjectDeploymentCandidate(
		fixture.ctx, fixture.authority, fixture.command,
		generatedProjectDeploymentTestSecretGeneration,
		generatedProjectDeploymentTestKeyFingerprint,
		GeneratedWorkloadProjectDeploymentHeadExpectation{},
	)
	if !errors.Is(err, ErrGeneratedWorkloadProjectDeploymentHeadConflict) {
		t.Fatalf("stale reservation error=%v", err)
	}
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	stale := reservation
	stale.Fence++
	_, err = advanceGeneratedWorkloadProjectDeploymentHeadTx(
		fixture.ctx, tx, stale, fixture.command, fixture.receipt,
	)
	_ = tx.Rollback(fixture.ctx)
	if !errors.Is(err, ErrGeneratedWorkloadProjectDeploymentHeadConflict) {
		t.Fatalf("stale promotion error=%v", err)
	}
	loaded, err := fixture.repository.CurrentGeneratedWorkloadProjectDeploymentHead(
		fixture.ctx, fixture.projectID,
	)
	if err != nil || loaded == nil || loaded.Fence != reservation.Fence ||
		loaded.Candidate == nil || loaded.Candidate.DeploymentID != reservation.DeploymentID {
		t.Fatalf("stale mutation changed head=%+v err=%v", loaded, err)
	}
}

func TestGeneratedWorkloadProjectDeploymentHeadAndHistoryCannotBeDeleted(t *testing.T) {
	fixture := newGeneratedProjectDeploymentHeadFixture(t, "no-delete")
	_, _ = reserveSealAndPromoteGeneratedProjectDeployment(
		t, fixture, GeneratedWorkloadProjectDeploymentHeadExpectation{},
	)
	if _, err := fixture.pool.Exec(fixture.ctx, `
		DELETE FROM generated_workload_project_deployment_head_history WHERE project_id=$1
	`, fixture.projectID); err == nil || !strings.Contains(err.Error(), "history is immutable") {
		t.Fatalf("history deletion error=%v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `
		DELETE FROM generated_workload_project_deployment_heads WHERE project_id=$1
	`, fixture.projectID); err == nil || !strings.Contains(err.Error(), "head is immutable") {
		t.Fatalf("head deletion error=%v", err)
	}
	if _, err := fixture.repository.CancelJob(
		fixture.ctx,
		testCancelCommand(t, fixture.jobID, "project-head-no-delete", "test cleanup"),
	); err != nil {
		t.Fatal(err)
	}
	project, err := fixture.repository.GetProject(fixture.ctx, fixture.projectID)
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.repository.DeleteProjectAtRevision(
		fixture.ctx, fixture.projectID, project.UpdatedAt,
	)
	if !errors.Is(err, ErrProjectDeploymentAudit) {
		t.Fatalf("project deletion error=%v", err)
	}
}

func assertGeneratedProjectDeploymentHeadHistory(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	want []string,
) {
	t.Helper()
	rows, err := fixture.pool.Query(fixture.ctx, `
		SELECT event FROM generated_workload_project_deployment_head_history
		WHERE project_id=$1 ORDER BY id
	`, fixture.projectID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var events []string
	for rows.Next() {
		var event string
		if err := rows.Scan(&event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("head history=%v want=%v", events, want)
	}
}
