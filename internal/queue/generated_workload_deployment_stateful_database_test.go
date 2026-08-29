package queue

import "testing"

func TestGeneratedWorkloadDeploymentStatefulManifestSealsExactRequiredRail(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixture(t, "stateful-rail")
	fixture.manifest = generatedDeploymentTestManifest(fixture.command, true)
	fixture.prepare(t, fixture.authority, fixture.verification.ID)
	fixture.reserve(t, fixture.authority, GeneratedWorkloadProjectDeploymentHeadExpectation{})
	if _, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command,
		GeneratedWorkloadDeploymentTransition{State: GeneratedWorkloadDeploymentApplying},
	); err != nil {
		t.Fatal(err)
	}
	receipt := fixture.executeSuccessfulRail(t, fixture.authority)
	applied, err := fixture.repository.SealGeneratedWorkloadDeploymentApplied(
		fixture.ctx, fixture.authority, fixture.command, receipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if applied.State != GeneratedWorkloadDeploymentApplied ||
		len(receipt.ExecutionEvidenceIDs) != 9 {
		t.Fatalf("stateful deployment rail=%+v receipt=%+v", applied, receipt)
	}
}
