package queue

import "testing"

func generatedDeploymentVacantNamespaceProof(
	t *testing.T,
	project string,
) GeneratedWorkloadDeploymentNamespacePreflight {
	t.Helper()
	proof, _, err := BindGeneratedWorkloadDeploymentNamespacePreflight(
		GeneratedWorkloadDeploymentNamespacePreflight{
			Schema:         GeneratedWorkloadDeploymentNamespacePreflightV1,
			ComposeProject: project, ContainerIDs: []string{},
			NetworkIDs: []string{}, VolumeNames: []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return proof
}

func generatedDeploymentApplyingFixtureAtAuthorityHardening(
	t *testing.T,
	label string,
) generatedDeploymentDatabaseFixture {
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

func generatedDeploymentQualifyAndCompleteProtected(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	execution GeneratedWorkloadDeploymentExecutionCommand,
) {
	t.Helper()
	generatedDeploymentQualifyProtectedExecution(t, fixture, fixture.authority, execution)
	generatedDeploymentCompleteExecution(t, fixture, execution)
}
