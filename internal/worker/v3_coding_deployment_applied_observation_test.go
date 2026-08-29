package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestAppliedDeploymentRawObservationUsesOnlyDockerGETsForUnrelatedServices(t *testing.T) {
	t.Parallel()
	for _, names := range [][]string{{"app", "nginx"}, {"app", "nginx", "postgres"}} {
		names := names
		t.Run(strings.Join(names, "-"), func(t *testing.T) {
			t.Parallel()
			expected, resources, inspections := appliedObservationFixture(t, names)
			client, calls := appliedObservationClient(t, resources, inspections)
			probes := 0
			observed, err := observeDirectCodingAppliedDeploymentWithClient(
				context.Background(), client, expected,
				queue.GeneratedWorkloadDeploymentBindLoopback, *genericPHPDeploymentDescriptor(),
				func(_ context.Context, host string, port uint16, path string) error {
					probes++
					if host != expected.Endpoint.Host || port != expected.Endpoint.Port || path != expected.Endpoint.Path {
						t.Fatalf("unexpected readiness probe %s:%d%s", host, port, path)
					}
					return nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if observed.SHA256 != expected.SHA256 || *calls != len(names)+1 || probes != 1 {
				t.Fatalf("observed=%+v calls=%d probes=%d", observed, *calls, probes)
			}
		})
	}
}

func TestAppliedDeploymentRawObservationRejectsDriftAndUnsafeBindings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*[]directCodingRollbackDockerResource, map[string]directCodingDockerInspection)
		ready  error
	}{
		{name: "extra project container", mutate: func(resources *[]directCodingRollbackDockerResource, _ map[string]directCodingDockerInspection) {
			*resources = append(*resources, directCodingRollbackDockerResource{ID: strings.Repeat("f", 64), Labels: map[string]string{"com.docker.compose.project": "omnidex-project-7"}})
		}},
		{name: "missing project container", mutate: func(resources *[]directCodingRollbackDockerResource, _ map[string]directCodingDockerInspection) {
			*resources = (*resources)[:1]
		}},
		{name: "wrong service label", mutate: func(_ *[]directCodingRollbackDockerResource, values map[string]directCodingDockerInspection) {
			value := values["app"]
			value.Config.Labels["com.docker.compose.service"] = "nginx"
			values["app"] = value
		}},
		{name: "wrong project label", mutate: func(_ *[]directCodingRollbackDockerResource, values map[string]directCodingDockerInspection) {
			value := values["app"]
			value.Config.Labels["com.docker.compose.project"] = "different-project"
			values["app"] = value
		}},
		{name: "changed image", mutate: func(_ *[]directCodingRollbackDockerResource, values map[string]directCodingDockerInspection) {
			value := values["app"]
			value.Image = "sha256:" + strings.Repeat("f", 64)
			values["app"] = value
		}},
		{name: "unhealthy", mutate: func(_ *[]directCodingRollbackDockerResource, values map[string]directCodingDockerInspection) {
			value := values["app"]
			value.State.Health.Status = "unhealthy"
			values["app"] = value
		}},
		{name: "stopped", mutate: func(_ *[]directCodingRollbackDockerResource, values map[string]directCodingDockerInspection) {
			value := values["app"]
			value.State.Status, value.State.Running = "exited", false
			values["app"] = value
		}},
		{name: "restart drift", mutate: func(_ *[]directCodingRollbackDockerResource, values map[string]directCodingDockerInspection) {
			value := values["app"]
			value.HostConfig.RestartPolicy.Name = "no"
			values["app"] = value
		}},
		{name: "gateway binding swapped", mutate: func(_ *[]directCodingRollbackDockerResource, values map[string]directCodingDockerInspection) {
			app, gateway := values["app"], values["nginx"]
			app.NetworkSettings.Ports, gateway.NetworkSettings.Ports = gateway.NetworkSettings.Ports, map[string][]directCodingDockerPortBinding{}
			values["app"], values["nginx"] = app, gateway
		}},
		{name: "wrong gateway target", mutate: func(_ *[]directCodingRollbackDockerResource, values map[string]directCodingDockerInspection) {
			value := values["nginx"]
			binding := value.NetworkSettings.Ports["80/tcp"]
			value.NetworkSettings.Ports = map[string][]directCodingDockerPortBinding{"81/tcp": binding}
			values["nginx"] = value
		}},
		{name: "noncanonical published port", mutate: func(_ *[]directCodingRollbackDockerResource, values map[string]directCodingDockerInspection) {
			value := values["nginx"]
			value.NetworkSettings.Ports["80/tcp"][0].HostPort = "049173"
			values["nginx"] = value
		}},
		{name: "multiple gateway bindings", mutate: func(_ *[]directCodingRollbackDockerResource, values map[string]directCodingDockerInspection) {
			value := values["nginx"]
			value.NetworkSettings.Ports["443/tcp"] = []directCodingDockerPortBinding{{HostIP: "127.0.0.1", HostPort: "49173"}}
			values["nginx"] = value
		}},
		{name: "readiness failure", ready: fmt.Errorf("endpoint refused")},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			expected, resources, inspections := appliedObservationFixture(t, []string{"app", "nginx"})
			if testCase.mutate != nil {
				testCase.mutate(&resources, inspections)
			}
			client, _ := appliedObservationClient(t, resources, inspections)
			_, err := observeDirectCodingAppliedDeploymentWithClient(
				context.Background(), client, expected,
				queue.GeneratedWorkloadDeploymentBindLoopback, *genericPHPDeploymentDescriptor(),
				func(context.Context, string, uint16, string) error { return testCase.ready },
			)
			if err == nil {
				t.Fatal("drifted applied deployment observation succeeded")
			}
		})
	}
}

func appliedObservationFixture(
	t *testing.T,
	names []string,
) (directCodingDeploymentObservation, []directCodingRollbackDockerResource, map[string]directCodingDockerInspection) {
	t.Helper()
	services := make([]directCodingObservedService, len(names))
	resources := make([]directCodingRollbackDockerResource, len(names))
	inspections := make(map[string]directCodingDockerInspection, len(names))
	for index, name := range names {
		containerID := strings.Repeat(string(rune('a'+index)), 64)
		imageID := "sha256:" + strings.Repeat(string(rune('d'+index)), 64)
		services[index] = directCodingObservedService{
			Service: name, ContainerID: containerID, ImageID: imageID,
			RestartPolicy: "unless-stopped", State: "running", Health: "healthy",
		}
		labels := map[string]string{
			"com.docker.compose.project": "omnidex-project-7",
			"com.docker.compose.service": name, "com.docker.compose.image": imageID,
		}
		resources[index] = directCodingRollbackDockerResource{ID: containerID, Labels: labels}
		ports := map[string][]directCodingDockerPortBinding{}
		if name == "nginx" {
			ports["80/tcp"] = []directCodingDockerPortBinding{{HostIP: "127.0.0.1", HostPort: "49173"}}
		}
		inspections[name] = appliedDockerInspection(t, containerID, imageID, labels, ports)
	}
	expected, err := bindDirectCodingAppliedObservation(
		"omnidex-project-7", services,
		directCodingObservedEndpoint{Scheme: "http", Host: "service.example.test", Port: 49173, Path: directCodingDeploymentReadinessPath},
	)
	if err != nil {
		t.Fatal(err)
	}
	return expected, resources, inspections
}

func appliedDockerInspection(
	t *testing.T,
	id, image string,
	labels map[string]string,
	ports map[string][]directCodingDockerPortBinding,
) directCodingDockerInspection {
	t.Helper()
	raw := map[string]any{
		"Id": id, "Image": image, "Config": map[string]any{"Labels": labels},
		"HostConfig":      map[string]any{"RestartPolicy": map[string]any{"Name": "unless-stopped"}},
		"State":           map[string]any{"Status": "running", "Running": true, "Health": map[string]any{"Status": "healthy"}},
		"NetworkSettings": map[string]any{"Ports": ports},
	}
	encoded, _ := json.Marshal(raw)
	var value directCodingDockerInspection
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func appliedObservationClient(
	t *testing.T,
	resources []directCodingRollbackDockerResource,
	inspections map[string]directCodingDockerInspection,
) (*http.Client, *int) {
	t.Helper()
	calls := 0
	return &http.Client{Transport: directCodingRollbackObservationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Method != http.MethodGet || request.Body != nil {
			t.Fatalf("applied observer issued non-read request: %+v", request)
		}
		var body any
		if request.URL.Path == "/containers/json" {
			body = resources
		} else {
			parts := strings.Split(request.URL.Path, "/")
			if len(parts) != 4 || parts[1] != "containers" || parts[3] != "json" {
				t.Fatalf("unexpected Docker path %q", request.URL.Path)
			}
			for _, value := range inspections {
				if value.ID == parts[2] {
					body = value
				}
			}
			if body == nil {
				t.Fatalf("unexpected container %q", parts[2])
			}
		}
		encoded, _ := json.Marshal(body)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(encoded)), Request: request}, nil
	})}, &calls
}
