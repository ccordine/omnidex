package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDeploymentProjectAuthorityIsStableAcrossJobs(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789abcdef0123456789abcdef")
	settings := validDirectCodingDeploymentSettingsForTest()
	first, err := resolveDirectCodingDeploymentProjectAuthority(41, key, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.ComposeProject != "omnidex-project-41" ||
		first.SecretGeneration != directCodingInitialDeploymentSecretGeneration ||
		first.EndpointPortAuthority != queue.GeneratedWorkloadDeploymentPortAllocate ||
		first.EndpointPort != 0 || first.PriorDeploymentID != "" ||
		first.HeadExpectation != (queue.GeneratedWorkloadProjectDeploymentHeadExpectation{}) ||
		len(first.DeploymentKeyFingerprintSHA256) != 64 {
		t.Fatalf("first deployment authority=%+v", first)
	}

	head := queue.GeneratedWorkloadProjectDeploymentHead{
		ProjectID: 41, ComposeProject: first.ComposeProject,
		SecretGeneration:               first.SecretGeneration,
		DeploymentKeyFingerprintSHA256: first.DeploymentKeyFingerprintSHA256,
		ActiveDeploymentID:             "generated_workload_deployment_" + strings.Repeat("a", 64),
		Endpoint: &queue.GeneratedWorkloadProjectDeploymentEndpoint{
			Scheme: "http", Host: settings.AdvertisedHost,
			Port: 49173, Path: directCodingDeploymentReadinessPath,
		},
		Revision: 3, Fence: 7,
	}
	successor, err := resolveDirectCodingDeploymentProjectAuthority(41, key, settings, &head)
	if err != nil {
		t.Fatal(err)
	}
	if successor.ComposeProject != first.ComposeProject ||
		successor.SecretGeneration != first.SecretGeneration ||
		successor.DeploymentKeyFingerprintSHA256 != first.DeploymentKeyFingerprintSHA256 ||
		successor.PriorDeploymentID != head.ActiveDeploymentID ||
		successor.EndpointPortAuthority != queue.GeneratedWorkloadDeploymentPortFixed ||
		successor.EndpointPort != head.Endpoint.Port ||
		successor.HeadExpectation.Revision != head.Revision ||
		successor.HeadExpectation.Fence != head.Fence {
		t.Fatalf("successor deployment authority=%+v", successor)
	}
}

func TestDeploymentProjectAuthorityRejectsChangedKeyOrEndpoint(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789abcdef0123456789abcdef")
	settings := validDirectCodingDeploymentSettingsForTest()
	first, err := resolveDirectCodingDeploymentProjectAuthority(73, key, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	head := queue.GeneratedWorkloadProjectDeploymentHead{
		ProjectID: 73, ComposeProject: first.ComposeProject,
		SecretGeneration:               first.SecretGeneration,
		DeploymentKeyFingerprintSHA256: first.DeploymentKeyFingerprintSHA256,
		ActiveDeploymentID:             "generated_workload_deployment_" + strings.Repeat("b", 64),
		Endpoint: &queue.GeneratedWorkloadProjectDeploymentEndpoint{
			Scheme: "http", Host: settings.AdvertisedHost,
			Port: 49174, Path: directCodingDeploymentReadinessPath,
		},
		Revision: 1, Fence: 1,
	}
	if _, err := resolveDirectCodingDeploymentProjectAuthority(
		73, []byte("fedcba9876543210fedcba9876543210"), settings, &head,
	); err == nil || !strings.Contains(err.Error(), "key authority") {
		t.Fatalf("changed deployment key error=%v", err)
	}
	head.Endpoint.Host = "different.example.test"
	if _, err := resolveDirectCodingDeploymentProjectAuthority(73, key, settings, &head); err == nil ||
		!strings.Contains(err.Error(), "endpoint differs") {
		t.Fatalf("changed deployment endpoint error=%v", err)
	}
}

func TestDeploymentProjectAuthorityRejectsUnpromotedActiveHead(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789abcdef0123456789abcdef")
	settings := validDirectCodingDeploymentSettingsForTest()
	first, err := resolveDirectCodingDeploymentProjectAuthority(89, key, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	head := queue.GeneratedWorkloadProjectDeploymentHead{
		ProjectID: 89, ComposeProject: first.ComposeProject,
		SecretGeneration:               first.SecretGeneration,
		DeploymentKeyFingerprintSHA256: first.DeploymentKeyFingerprintSHA256,
		ActiveDeploymentID:             "generated_workload_deployment_" + strings.Repeat("c", 64),
		Endpoint: &queue.GeneratedWorkloadProjectDeploymentEndpoint{
			Scheme: "http", Host: settings.AdvertisedHost,
			Port: 49175, Path: directCodingDeploymentReadinessPath,
		},
		Revision: 0, Fence: 1,
	}
	if _, err := resolveDirectCodingDeploymentProjectAuthority(89, key, settings, &head); err == nil ||
		!strings.Contains(err.Error(), "promoted revision") {
		t.Fatalf("unpromoted active head error=%v", err)
	}
}

func validDirectCodingDeploymentSettingsForTest() DeploymentSettings {
	return DeploymentSettings{
		KeyFile: "/var/lib/omnidex/deployment.key", BindAddress: "0.0.0.0",
		AdvertisedHost: "service.example.test", ProbeHost: "host.docker.internal",
	}
}
