package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

func observeDirectCodingAppliedDeployment(
	parent context.Context,
	socketPath string,
	expected directCodingDeploymentObservation,
	bindHost queue.GeneratedWorkloadDeploymentBindHost,
	descriptor directCodingDeploymentDescriptor,
) (directCodingDeploymentObservation, error) {
	if parent == nil {
		return directCodingDeploymentObservation{}, fmt.Errorf("applied deployment observation requires a context")
	}
	if err := validateV3DockerSocket(socketPath); err != nil {
		return directCodingDeploymentObservation{}, err
	}
	if err := validateV3RootlessDockerDaemon(parent, socketPath); err != nil {
		return directCodingDeploymentObservation{}, err
	}
	ctx, cancel := context.WithTimeout(parent, directCodingDockerObserveTimeout)
	defer cancel()
	transport := directCodingDockerUnixTransport(socketPath)
	defer transport.CloseIdleConnections()
	return observeDirectCodingAppliedDeploymentWithClient(
		ctx, &http.Client{Transport: transport}, expected, bindHost, descriptor,
		probeDirectCodingDeploymentReadiness,
	)
}

func observeDirectCodingAppliedDeploymentWithClient(
	ctx context.Context,
	client *http.Client,
	expected directCodingDeploymentObservation,
	bindHost queue.GeneratedWorkloadDeploymentBindHost,
	descriptor directCodingDeploymentDescriptor,
	readiness func(context.Context, string, uint16, string) error,
) (directCodingDeploymentObservation, error) {
	bindAddress, err := directCodingAppliedBindAddress(bindHost)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	if ctx == nil || client == nil || readiness == nil {
		return directCodingDeploymentObservation{}, fmt.Errorf("applied deployment observation requires exact observers")
	}
	if err := validateDirectCodingAppliedExpectedObservation(expected); err != nil {
		return directCodingDeploymentObservation{}, err
	}
	if err := descriptor.validate(); err != nil {
		return directCodingDeploymentObservation{}, err
	}
	filters, err := json.Marshal(map[string][]string{
		"label": {"com.docker.compose.project=" + expected.Project},
	})
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	resources := make([]directCodingRollbackDockerResource, 0)
	if err := getDirectCodingDeploymentRollbackResources(
		ctx, client, "/containers/json",
		url.Values{"all": {"1"}, "filters": {string(filters)}}, &resources,
	); err != nil {
		return directCodingDeploymentObservation{}, fmt.Errorf("list applied deployment containers: %w", err)
	}
	actualIDs, err := canonicalDirectCodingRollbackResourceIDs(resources, expected.Project)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	expectedIDs := make([]string, len(expected.Services))
	for index, service := range expected.Services {
		expectedIDs[index] = service.ContainerID
	}
	sort.Strings(expectedIDs)
	if !reflect.DeepEqual(actualIDs, expectedIDs) {
		return directCodingDeploymentObservation{}, fmt.Errorf("applied deployment service set differs from sealed observation")
	}
	services := make([]directCodingObservedService, len(expected.Services))
	published := 0
	for index, service := range expected.Services {
		inspection, err := inspectDirectCodingContainer(ctx, client, service.ContainerID)
		if err != nil {
			return directCodingDeploymentObservation{}, fmt.Errorf("inspect applied service %s: %w", service.Service, err)
		}
		observed, bindings, err := validateDirectCodingAppliedInspection(
			expected.Project, service, inspection, bindAddress, expected.Endpoint.Port, descriptor,
		)
		if err != nil {
			return directCodingDeploymentObservation{}, fmt.Errorf("validate applied service %s: %w", service.Service, err)
		}
		services[index] = observed
		published += bindings
	}
	if published != 1 {
		return directCodingDeploymentObservation{}, fmt.Errorf("applied deployment requires one exact published endpoint")
	}
	if err := readiness(ctx, expected.Endpoint.Host, expected.Endpoint.Port, expected.Endpoint.Path); err != nil {
		return directCodingDeploymentObservation{}, fmt.Errorf("probe sealed applied endpoint: %w", err)
	}
	observed, err := bindDirectCodingAppliedObservation(expected.Project, services, expected.Endpoint)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	if !reflect.DeepEqual(observed, expected) {
		return directCodingDeploymentObservation{}, fmt.Errorf("current deployment differs from exact sealed final observation")
	}
	return observed, nil
}

func validateDirectCodingAppliedInspection(
	project string,
	expected directCodingObservedService,
	inspection directCodingDockerInspection,
	bindAddress string,
	endpointPort uint16,
	descriptor directCodingDeploymentDescriptor,
) (directCodingObservedService, int, error) {
	if inspection.ID != expected.ContainerID || inspection.Image != expected.ImageID ||
		inspection.Config == nil || inspection.HostConfig == nil ||
		inspection.State == nil || inspection.NetworkSettings == nil {
		return directCodingObservedService{}, 0, fmt.Errorf("container identity or inspection authority differs")
	}
	labels := inspection.Config.Labels
	if labels["com.docker.compose.project"] != project ||
		labels["com.docker.compose.service"] != expected.Service ||
		labels["com.docker.compose.image"] != expected.ImageID ||
		inspection.HostConfig.RestartPolicy.Name != expected.RestartPolicy ||
		inspection.State.Status != expected.State || !inspection.State.Running ||
		inspection.State.Health == nil || inspection.State.Health.Status != expected.Health {
		return directCodingObservedService{}, 0, fmt.Errorf("container state differs from sealed service receipt")
	}
	bindings := 0
	for key, values := range inspection.NetworkSettings.Ports {
		if len(values) == 0 {
			continue
		}
		parts := strings.Split(key, "/")
		port, parseErr := strconv.Atoi(parts[0])
		if expected.Service != descriptor.GatewayService || len(parts) != 2 || parseErr != nil ||
			port != descriptor.GatewayContainerPort ||
			strconv.Itoa(port) != parts[0] || parts[1] != "tcp" ||
			len(values) != 1 || values[0].HostIP != bindAddress ||
			values[0].HostPort != strconv.Itoa(int(endpointPort)) {
			return directCodingObservedService{}, 0, fmt.Errorf("published binding differs from sealed endpoint")
		}
		bindings++
	}
	if expected.Service == descriptor.GatewayService && bindings != 1 ||
		expected.Service != descriptor.GatewayService && bindings != 0 {
		return directCodingObservedService{}, 0, fmt.Errorf("service published-port role differs from registered descriptor")
	}
	return expected, bindings, nil
}

func bindDirectCodingAppliedObservation(
	project string,
	services []directCodingObservedService,
	endpoint directCodingObservedEndpoint,
) (directCodingDeploymentObservation, error) {
	serviceSHA, err := directCodingObservationHash("services.v1", services)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	endpointSHA, err := directCodingObservationHash("endpoint.v1", endpoint)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	canonical := struct {
		Schema   string                        `json:"schema"`
		Project  string                        `json:"project"`
		Services []directCodingObservedService `json:"services"`
		Endpoint directCodingObservedEndpoint  `json:"endpoint"`
	}{directCodingDeploymentObservationSchema, project, services, endpoint}
	sha, err := directCodingObservationHash("observation.v1", canonical)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	return directCodingDeploymentObservation{
		Schema: directCodingDeploymentObservationSchema, Project: project,
		Services: services, Endpoint: endpoint, ServicesSHA256: serviceSHA,
		EndpointSHA256: endpointSHA, SHA256: sha,
	}, nil
}

func validateDirectCodingAppliedExpectedObservation(expected directCodingDeploymentObservation) error {
	if expected.Schema != directCodingDeploymentObservationSchema ||
		!v3DeploymentComposeProjectPattern.MatchString(expected.Project) ||
		expected.Endpoint.Scheme != "http" || expected.Endpoint.Port == 0 ||
		expected.Endpoint.Path != directCodingDeploymentReadinessPath ||
		validateDirectCodingDeploymentHost("applied", expected.Endpoint.Host) != nil {
		return fmt.Errorf("sealed applied deployment observation is invalid")
	}
	if err := validateDirectCodingObservedServices(
		expected.Services, directCodingAppliedServiceNames(expected.Services), expected.Endpoint.Port,
	); err != nil {
		return err
	}
	previous := ""
	containers := make(map[string]struct{}, len(expected.Services))
	for _, service := range expected.Services {
		if !generatedDeploymentServicePattern.MatchString(service.Service) ||
			(previous != "" && service.Service <= previous) {
			return fmt.Errorf("sealed applied services are not canonical, sorted, and unique")
		}
		if _, exists := containers[service.ContainerID]; exists {
			return fmt.Errorf("sealed applied services repeat a container identity")
		}
		containers[service.ContainerID] = struct{}{}
		previous = service.Service
	}
	bound, err := bindDirectCodingAppliedObservation(expected.Project, expected.Services, expected.Endpoint)
	if err != nil || !reflect.DeepEqual(bound, expected) {
		return fmt.Errorf("sealed applied deployment hashes are invalid: %w", err)
	}
	return nil
}

func directCodingAppliedServiceNames(services []directCodingObservedService) []string {
	values := make([]string, len(services))
	for index, service := range services {
		values[index] = service.Service
	}
	return values
}

func directCodingAppliedBindAddress(bindHost queue.GeneratedWorkloadDeploymentBindHost) (string, error) {
	switch bindHost {
	case queue.GeneratedWorkloadDeploymentBindLoopback:
		return "127.0.0.1", nil
	case queue.GeneratedWorkloadDeploymentBindAllInterfaces:
		return "0.0.0.0", nil
	default:
		return "", fmt.Errorf("applied deployment bind-host authority is invalid")
	}
}
