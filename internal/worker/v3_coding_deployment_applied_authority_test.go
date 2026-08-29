package worker

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestPersistedAppliedDescriptorBindsRegisteredTransportAuthority(t *testing.T) {
	t.Parallel()
	base := queue.GeneratedWorkloadDeploymentCommand{
		AdapterID: genericPHPServiceAdapter, AdapterVersion: "1",
		ProfileID: phpServiceVersionProfileV1, ProfileVersion: "1",
		Services: []string{"app", "nginx"}, RequiredSecretNames: []string{},
		EndpointScheme: "http", EndpointPath: directCodingDeploymentReadinessPath,
	}
	stateful := base
	stateful.Services = []string{"app", "nginx", "postgres"}
	stateful.RequiredSecretNames = []string{"SERVICE_STATE_DB_PASSWORD"}
	for _, command := range []queue.GeneratedWorkloadDeploymentCommand{base, stateful} {
		if _, err := directCodingPersistedDeploymentDescriptor(command); err != nil {
			t.Fatalf("registered applied descriptor rejected: %v", err)
		}
	}
	tests := []struct {
		name   string
		mutate func(*queue.GeneratedWorkloadDeploymentCommand)
	}{
		{name: "adapter version", mutate: func(value *queue.GeneratedWorkloadDeploymentCommand) { value.AdapterVersion = "2" }},
		{name: "profile version", mutate: func(value *queue.GeneratedWorkloadDeploymentCommand) { value.ProfileVersion = "2" }},
		{name: "nondeployment adapter", mutate: func(value *queue.GeneratedWorkloadDeploymentCommand) {
			value.AdapterID, value.ProfileID = genericTypeScriptBrowserAdapter, typeScriptBrowserVersionProfileV1
		}},
		{name: "profile from another stack", mutate: func(value *queue.GeneratedWorkloadDeploymentCommand) {
			value.ProfileID = typeScriptBrowserVersionProfileV1
		}},
		{name: "arbitrary service set", mutate: func(value *queue.GeneratedWorkloadDeploymentCommand) {
			value.Services = []string{"app", "cache", "nginx"}
		}},
		{name: "state secret mismatch", mutate: func(value *queue.GeneratedWorkloadDeploymentCommand) {
			value.Services = []string{"app", "nginx", "postgres"}
			value.RequiredSecretNames = []string{}
		}},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			command := base
			testCase.mutate(&command)
			if _, err := directCodingPersistedDeploymentDescriptor(command); err == nil {
				t.Fatal("invalid persisted transport authority was accepted")
			}
		})
	}
}

func TestSealedAppliedHeadValidatesInitialAndSuccessorLineage(t *testing.T) {
	t.Parallel()
	initial, head := sealedAppliedHeadFixture()
	if err := validateDirectCodingSealedAppliedHead(&head, &initial); err != nil {
		t.Fatal(err)
	}
	successor := initial
	successor.Command.PriorDeploymentID = "generated_workload_deployment_" + strings.Repeat("b", 64)
	successor.Command.EndpointPortAuthority = queue.GeneratedWorkloadDeploymentPortFixed
	successor.Command.EndpointPort = 49173
	successor.Receipt.PriorDeploymentID = successor.Command.PriorDeploymentID
	head.Revision = 2
	if err := validateDirectCodingSealedAppliedHead(&head, &successor); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*queue.GeneratedWorkloadDeploymentSnapshot, *queue.GeneratedWorkloadProjectDeploymentHead){
		func(snapshot *queue.GeneratedWorkloadDeploymentSnapshot, _ *queue.GeneratedWorkloadProjectDeploymentHead) {
			snapshot.Command.EndpointPortAuthority = queue.GeneratedWorkloadDeploymentPortFixed
		},
		func(_ *queue.GeneratedWorkloadDeploymentSnapshot, value *queue.GeneratedWorkloadProjectDeploymentHead) {
			value.Revision = 2
		},
		func(snapshot *queue.GeneratedWorkloadDeploymentSnapshot, _ *queue.GeneratedWorkloadProjectDeploymentHead) {
			snapshot.Receipt.PriorDeploymentID = "different"
		},
	} {
		snapshot, value := sealedAppliedHeadFixture()
		mutate(&snapshot, &value)
		if validateDirectCodingSealedAppliedHead(&value, &snapshot) == nil {
			t.Fatal("invalid sealed applied lineage was accepted")
		}
	}
}

func TestDeploymentRecoveryGatePrecedesWorkspaceResolution(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("v3_coding_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	early := strings.Index(string(source), "recoverDeploymentBeforeWorkspace(request)")
	workspace := strings.Index(string(source), "workspaceScopeForV3Job")
	if early < 0 || workspace < 0 || early >= workspace {
		t.Fatal("deployment recovery is not the first pre-workspace runtime gate")
	}
}

func sealedAppliedHeadFixture() (
	queue.GeneratedWorkloadDeploymentSnapshot,
	queue.GeneratedWorkloadProjectDeploymentHead,
) {
	endpoint := queue.GeneratedWorkloadProjectDeploymentEndpoint{
		Scheme: "http", Host: "service.example.test", Port: 49173,
		Path: directCodingDeploymentReadinessPath,
	}
	receipt := &queue.GeneratedWorkloadDeploymentReceipt{
		ComposeProject: "omnidex-project-7", EndpointScheme: endpoint.Scheme,
		EndpointHost: endpoint.Host, EndpointPort: endpoint.Port, EndpointPath: endpoint.Path,
	}
	snapshot := queue.GeneratedWorkloadDeploymentSnapshot{
		Command: queue.GeneratedWorkloadDeploymentCommand{
			ComposeProject: "omnidex-project-7", EndpointPortAuthority: queue.GeneratedWorkloadDeploymentPortAllocate,
			EndpointScheme: endpoint.Scheme, EndpointHost: endpoint.Host, EndpointPath: endpoint.Path,
		},
		Record:  queue.GeneratedWorkloadDeploymentRecord{OperationID: "generated_workload_deployment_" + strings.Repeat("a", 64)},
		Receipt: receipt,
	}
	return snapshot, queue.GeneratedWorkloadProjectDeploymentHead{
		ActiveDeploymentID: snapshot.Record.OperationID, Endpoint: &endpoint, Revision: 1,
	}
}
