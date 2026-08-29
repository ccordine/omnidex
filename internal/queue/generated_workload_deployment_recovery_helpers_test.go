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
	proof := generatedDeploymentVacantNamespaceProof(t, fixture.command.ComposeProject)
	if _, created, err := fixture.repository.RecordGeneratedWorkloadDeploymentNamespaceRequalification(
		fixture.ctx, fixture.authority, fixture.command, execution, proof,
	); err != nil || !created {
		t.Fatalf("qualify %s namespace: created=%t err=%v", execution.Slot.Name, created, err)
	}
	generatedDeploymentCompleteExecution(t, fixture, execution)
}
