package queue

import (
	"errors"
	"testing"
)

func TestProjectDeletionRejectsImmutableDeploymentHistoryExplicitly(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixture(t, "project-delete-audit")
	prepared := fixture.prepare(t, fixture.authority, fixture.verification.ID)
	if _, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command,
		GeneratedWorkloadDeploymentTransition{
			State: GeneratedWorkloadDeploymentFailed, Code: "test_cleanup",
			DetailSHA256: generatedDeploymentSHA(prepared.OperationID),
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.CancelJob(
		fixture.ctx,
		testCancelCommand(t, fixture.jobID, "project-delete-audit", "test cleanup"),
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
	if _, lookupErr := fixture.repository.GetProject(
		fixture.ctx, fixture.projectID,
	); lookupErr != nil {
		t.Fatalf("rejected deletion changed project: %v", lookupErr)
	}
}
