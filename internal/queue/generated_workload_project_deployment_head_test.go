package queue

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func validGeneratedWorkloadProjectDeploymentHead() GeneratedWorkloadProjectDeploymentHead {
	return GeneratedWorkloadProjectDeploymentHead{
		ProjectID:                      17,
		ComposeProject:                 "project-17-service",
		SecretGeneration:               1,
		DeploymentKeyFingerprintSHA256: strings.Repeat("a", 64),
		Revision:                       0,
		Fence:                          1,
		Candidate: &GeneratedWorkloadProjectDeploymentCandidate{
			DeploymentID: "generated_workload_deployment_" + strings.Repeat("b", 64),
			Authority: GeneratedWorkloadDeploymentAuthority{
				JobID: 23, Generation: 1, StepID: 31, ProjectID: 17,
			},
			Executor: GeneratedWorkloadDeploymentExecutor{
				StepAttempt: 1, WorkerID: "worker-1",
			},
		},
		CreatedAt: time.Unix(1_800_000_000, 0).UTC(),
		UpdatedAt: time.Unix(1_800_000_001, 0).UTC(),
	}
}

func TestGeneratedWorkloadProjectDeploymentHeadValidation(t *testing.T) {
	head := validGeneratedWorkloadProjectDeploymentHead()
	if err := validateGeneratedWorkloadProjectDeploymentHead(head); err != nil {
		t.Fatal(err)
	}
	reservation, err := generatedWorkloadProjectDeploymentReservation(head)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.ProjectID != head.ProjectID || reservation.Fence != head.Fence ||
		reservation.DeploymentID != head.Candidate.DeploymentID {
		t.Fatalf("reservation=%+v head=%+v", reservation, head)
	}

	invalid := head
	invalid.Endpoint = &GeneratedWorkloadProjectDeploymentEndpoint{
		Scheme: "https", Host: "service.example.test", Port: 443, Path: "/ready",
	}
	if err := validateGeneratedWorkloadProjectDeploymentHead(invalid); err == nil {
		t.Fatal("head accepted endpoint without an active deployment")
	}
	invalid = head
	invalid.DeploymentKeyFingerprintSHA256 = strings.Repeat("z", 64)
	if err := validateGeneratedWorkloadProjectDeploymentHead(invalid); err == nil {
		t.Fatal("head accepted an invalid deployment-key fingerprint")
	}
	if err := validateGeneratedWorkloadProjectDeploymentExpectation(
		GeneratedWorkloadProjectDeploymentHeadExpectation{Revision: 1},
	); err == nil {
		t.Fatal("head expectation accepted a revision without a fence")
	}
}

func TestGeneratedWorkloadProjectDeploymentHeadTypesContainFingerprintsNotSecrets(t *testing.T) {
	for _, value := range []any{
		GeneratedWorkloadProjectDeploymentHead{},
		GeneratedWorkloadProjectDeploymentCandidate{},
		GeneratedWorkloadProjectDeploymentReservation{},
	} {
		typeOf := reflect.TypeOf(value)
		for index := 0; index < typeOf.NumField(); index++ {
			name := strings.ToLower(typeOf.Field(index).Name)
			for _, forbidden := range []string{"secretvalue", "rawsecret", "deploymentkeyvalue"} {
				if strings.Contains(name, forbidden) {
					t.Fatalf("%s contains forbidden secret-bearing field %q", typeOf.Name(), name)
				}
			}
		}
	}
}
