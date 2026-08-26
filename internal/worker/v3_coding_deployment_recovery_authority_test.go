package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestRecoveredDeploymentProjectAuthorityIsStableAndReadOnly(t *testing.T) {
	key := []byte(strings.Repeat("k", directCodingDeploymentKeyBytes))
	snapshot := &queue.GeneratedWorkloadDeploymentSnapshot{
		Command: queue.GeneratedWorkloadDeploymentCommand{
			Authority:      queue.GeneratedWorkloadDeploymentAuthority{ProjectID: 7},
			ComposeProject: "omnidex-project-7",
		},
		Record: queue.GeneratedWorkloadDeploymentRecord{
			OperationID: "generated_workload_deployment_" + strings.Repeat("a", 64),
		},
	}
	project, err := directCodingRecoveredProjectAuthority(
		7, key, nil, snapshot,
	)
	if err != nil {
		t.Fatal(err)
	}
	if project.secretGeneration != directCodingInitialDeploymentSecretGeneration ||
		project.expectation != (queue.GeneratedWorkloadProjectDeploymentHeadExpectation{}) {
		t.Fatalf("first project authority=%+v", project)
	}
	fingerprint, err := directCodingDeploymentKeyFingerprintSHA256(key)
	if err != nil {
		t.Fatal(err)
	}
	head := &queue.GeneratedWorkloadProjectDeploymentHead{
		ProjectID: 7, ComposeProject: snapshot.Command.ComposeProject,
		SecretGeneration: 3, DeploymentKeyFingerprintSHA256: fingerprint,
		Candidate: &queue.GeneratedWorkloadProjectDeploymentCandidate{
			DeploymentID: snapshot.Record.OperationID,
		},
		Revision: 0, Fence: 4,
	}
	project, err = directCodingRecoveredProjectAuthority(
		7, key, head, snapshot,
	)
	if err != nil {
		t.Fatal(err)
	}
	if project.secretGeneration != 3 || project.expectation.Revision != 0 ||
		project.expectation.Fence != 4 {
		t.Fatalf("applied project authority=%+v", project)
	}
}

func TestRecoveredDeploymentCannotBypassSuccessorOrHeadFence(t *testing.T) {
	key := []byte(strings.Repeat("k", directCodingDeploymentKeyBytes))
	snapshot := &queue.GeneratedWorkloadDeploymentSnapshot{
		Command: queue.GeneratedWorkloadDeploymentCommand{
			Authority:      queue.GeneratedWorkloadDeploymentAuthority{ProjectID: 7},
			ComposeProject: "omnidex-project-7", PriorDeploymentID: "prior",
		},
	}
	if _, err := directCodingRecoveredProjectAuthority(
		7, key, nil, snapshot,
	); err == nil || !strings.Contains(err.Error(), "successor") {
		t.Fatalf("successor recovery error=%v", err)
	}
	snapshot.Command.PriorDeploymentID = ""
	snapshot.Record.OperationID = "generated_workload_deployment_" + strings.Repeat("a", 64)
	fingerprint, _ := directCodingDeploymentKeyFingerprintSHA256(key)
	head := &queue.GeneratedWorkloadProjectDeploymentHead{
		ProjectID: 7, ComposeProject: snapshot.Command.ComposeProject,
		SecretGeneration: 1, DeploymentKeyFingerprintSHA256: fingerprint,
		Revision: 0, Fence: 2,
		Candidate: &queue.GeneratedWorkloadProjectDeploymentCandidate{
			DeploymentID: "generated_workload_deployment_" + strings.Repeat("b", 64),
		},
	}
	if _, err := directCodingRecoveredProjectAuthority(
		7, key, head, snapshot,
	); err == nil || !strings.Contains(err.Error(), "different deployment candidate") {
		t.Fatalf("foreign fence error=%v", err)
	}
}
