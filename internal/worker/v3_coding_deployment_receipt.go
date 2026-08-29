package worker

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func directCodingGeneratedDeploymentReceipt(
	record queue.GeneratedWorkloadDeploymentRecord,
	command queue.GeneratedWorkloadDeploymentCommand,
	observation directCodingDeploymentObservation,
	appliedAt, observedAt time.Time,
	verificationReceiptID string,
	executionEvidenceIDs []int64,
	observationEvidenceIDs []int64,
) (queue.GeneratedWorkloadDeploymentReceipt, error) {
	if record.OperationID == "" || observation.Schema != directCodingDeploymentObservationSchema ||
		observation.Project != command.ComposeProject || observation.SHA256 == "" {
		return queue.GeneratedWorkloadDeploymentReceipt{}, fmt.Errorf("deployment receipt requires one canonical applied observation")
	}
	if !appliedAt.Equal(appliedAt.UTC().Truncate(time.Microsecond)) ||
		!observedAt.Equal(observedAt.UTC().Truncate(time.Microsecond)) ||
		observedAt.Before(appliedAt) {
		return queue.GeneratedWorkloadDeploymentReceipt{}, fmt.Errorf("deployment receipt requires ordered UTC microsecond timestamps")
	}
	services := make([]queue.GeneratedWorkloadDeploymentServiceReceipt, len(observation.Services))
	for index, service := range observation.Services {
		if service.RestartPolicy != "unless-stopped" {
			return queue.GeneratedWorkloadDeploymentReceipt{}, fmt.Errorf("deployment service %s has an unsupported restart policy", service.Service)
		}
		services[index] = queue.GeneratedWorkloadDeploymentServiceReceipt{
			Service: service.Service, ContainerID: service.ContainerID,
			ImageDigest:   service.ImageID,
			RestartPolicy: queue.GeneratedWorkloadDeploymentRestartUnlessStopped,
			State:         service.State, Health: service.Health,
		}
	}
	return queue.GeneratedWorkloadDeploymentReceipt{
		Schema:      queue.GeneratedWorkloadDeploymentReceiptV2,
		OperationID: record.OperationID, ConfigSHA256: command.ConfigSHA256,
		ComposeProject: observation.Project,
		EndpointScheme: observation.Endpoint.Scheme,
		EndpointHost:   observation.Endpoint.Host,
		EndpointPort:   observation.Endpoint.Port,
		EndpointPath:   observation.Endpoint.Path,
		Services:       services, AppliedAt: appliedAt, ObservedAt: observedAt,
		WorkspaceVerificationReceiptID: verificationReceiptID,
		ExecutionEvidenceIDs:           append([]int64(nil), executionEvidenceIDs...),
		ObservationEvidenceIDs:         append([]int64(nil), observationEvidenceIDs...),
		PriorDeploymentID:              command.PriorDeploymentID,
	}, nil
}

func directCodingDeploymentTimestamp() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func directCodingDeploymentURLs(
	endpoint directCodingObservedEndpoint,
) (string, string, error) {
	if endpoint.Scheme != "http" || endpoint.Host == "" || endpoint.Port == 0 ||
		endpoint.Path != directCodingDeploymentReadinessPath {
		return "", "", fmt.Errorf("persisted deployment endpoint is invalid")
	}
	host := net.JoinHostPort(endpoint.Host, strconv.Itoa(int(endpoint.Port)))
	service := (&url.URL{Scheme: endpoint.Scheme, Host: host}).String()
	health := (&url.URL{Scheme: endpoint.Scheme, Host: host, Path: endpoint.Path}).String()
	return service, health, nil
}
