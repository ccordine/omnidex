package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestGeneratedDeploymentCommandContainsOnlyCodeOwnedAuthorityAndSecretNames(t *testing.T) {
	program := phpDurableStateProgramFixture(t)
	command, err := directCodingGeneratedDeploymentCommand(
		model.StepAttemptAuthority{JobID: 7, Generation: 2, StepID: 9, Attempt: 1, WorkerID: "worker-1"},
		directCodingDeploymentProjectAuthority{
			ProjectID: 4, ComposeProject: "omnidex-project-4", SecretGeneration: 1,
			DeploymentKeyFingerprintSHA256: strings.Repeat("a", 64),
			EndpointPortAuthority:          queue.GeneratedWorkloadDeploymentPortAllocate,
		},
		directCodingServiceDeploymentResolution{
			Disposition:               assemblyline.ApplicationServiceDeploymentPersistCurrentHost,
			DispositionJobID:          strings.Repeat("1", 64),
			DispositionResponseSHA256: strings.Repeat("2", 64),
		},
		program,
		directCodingDeploymentWorkspaceIdentity{
			WorkspaceSHA256: strings.Repeat("3", 64), ComposeSHA256: strings.Repeat("4", 64), FileCount: 2,
		},
		DeploymentSettings{
			KeyFile: "/var/lib/omnidex-deployment/key", BindAddress: "0.0.0.0",
			AdvertisedHost: "service.example.test", ProbeHost: "host.docker.internal",
		},
		*genericPHPDeploymentDescriptor(),
		map[string]string{
			"HOST_BIND_ADDRESS": "0.0.0.0", "HOST_HTTP_PORT": "0",
			"SERVICE_STATE_DB_PASSWORD": "raw-secret",
		},
		strings.Repeat("5", 64),
		strings.Repeat("6", 64),
	)
	if err != nil {
		t.Fatal(err)
	}
	if command.BindHost != queue.GeneratedWorkloadDeploymentBindAllInterfaces ||
		command.EndpointPortAuthority != queue.GeneratedWorkloadDeploymentPortAllocate ||
		command.EndpointPort != 0 ||
		len(command.RequiredSecretNames) != 1 ||
		command.RequiredSecretNames[0] != "SERVICE_STATE_DB_PASSWORD" ||
		command.SecretSetSHA256 != strings.Repeat("5", 64) {
		t.Fatalf("command=%+v", command)
	}
	if strings.Contains(command.DeploymentIntentJobID, "raw-secret") ||
		strings.Contains(command.ConfigSHA256, "raw-secret") {
		t.Fatal("deployment command retained a secret value")
	}
}

func TestGeneratedDeploymentBindHostRejectsUnregisteredSpecificInterface(t *testing.T) {
	if _, err := directCodingGeneratedDeploymentBindHost("192.0.2.10"); err == nil {
		t.Fatal("specific-interface deployment was accepted without typed journal authority")
	}
}
