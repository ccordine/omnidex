package worker

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func TestGeneratedDeploymentReceiptProjectsOnlyCanonicalObservedReality(t *testing.T) {
	when := time.Unix(1_800_000_000, 123_456_000).UTC()
	command := queue.GeneratedWorkloadDeploymentCommand{
		ComposeProject: "omnidex-job-7-g2", ConfigSHA256: strings.Repeat("a", 64),
	}
	observation := directCodingDeploymentObservation{
		Schema: directCodingDeploymentObservationSchema, Project: command.ComposeProject,
		Services: []directCodingObservedService{{
			Service: "app", ContainerID: strings.Repeat("b", 64),
			ImageID: "sha256:" + strings.Repeat("c", 64), RestartPolicy: "unless-stopped",
			State: "running", Health: "healthy",
		}},
		Endpoint: directCodingObservedEndpoint{
			Scheme: "http", Host: "service.example.test", Port: 18080,
			Path: directCodingDeploymentReadinessPath,
		},
		SHA256: strings.Repeat("d", 64),
	}
	receipt, err := directCodingGeneratedDeploymentReceipt(
		queue.GeneratedWorkloadDeploymentRecord{OperationID: "generated_workload_deployment_" + strings.Repeat("e", 64)},
		command, observation, when, when.Add(time.Second),
		"generated_workload_verification_"+strings.Repeat("f", 64),
		[]int64{4, 5, 6, 7, 8, 9}, []int64{10, 11},
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.EndpointPort != 18080 || len(receipt.Services) != 1 ||
		receipt.Services[0].ImageDigest != observation.Services[0].ImageID {
		t.Fatalf("receipt=%+v", receipt)
	}
}
