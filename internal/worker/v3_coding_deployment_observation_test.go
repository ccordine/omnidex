package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestDirectCodingDeploymentObservationIsCanonicalForUnrelatedServiceSets(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name       string
		project    string
		services   []string
		gateway    string
		targetPort uint16
	}{
		{name: "forecast delivery", project: "fixture-forecast", services: []string{"api", "gateway"}, gateway: "gateway", targetPort: 80},
		{name: "equipment registry", project: "fixture-equipment", services: []string{"edge", "store", "worker"}, gateway: "edge", targetPort: 8080},
	}
	for fixtureIndex, fixture := range fixtures {
		fixtureIndex, fixture := fixtureIndex, fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			probeHost := "127.0.0.1"
			probePort := uint16(42000 + fixtureIndex)
			inspections := make(map[string]any, len(fixture.services))
			rows := make([]map[string]any, 0, len(fixture.services))
			for serviceIndex, service := range fixture.services {
				digit := fmt.Sprintf("%x", fixtureIndex*4+serviceIndex+1)
				containerID := strings.Repeat(digit, 64)
				imageID := "sha256:" + strings.Repeat(digit, 64)
				publishers := []directCodingComposePublisher{}
				ports := map[string]any{"9000/tcp": nil}
				if service == fixture.gateway {
					publishers = []directCodingComposePublisher{{
						URL: probeHost, TargetPort: int(fixture.targetPort),
						PublishedPort: int(probePort), Protocol: "tcp",
					}}
					ports[strconv.Itoa(int(fixture.targetPort))+"/tcp"] = []map[string]string{{
						"HostIp": probeHost, "HostPort": strconv.Itoa(int(probePort)),
					}}
				}
				rows = append(rows, directCodingComposePSFixtureRow(
					containerID[:12], fixture.project, service, publishers,
				))
				inspections[containerID[:12]] = directCodingDockerInspectionFixture(
					containerID, imageID, fixture.project, service, ports,
				)
			}
			reverseDirectCodingMaps(rows)
			request := directCodingDeploymentObservationRequest{
				Project: fixture.project, ExpectedServices: fixture.services,
				GatewayService: fixture.gateway, GatewayContainerPort: fixture.targetPort,
				BindAddress: probeHost, ProbeHost: probeHost,
				AdvertisedHost: "service.example.test", ReadinessPath: directCodingDeploymentReadinessPath,
			}
			output := directCodingComposePSFixture(t, rows)
			client := directCodingDockerObservationClient(t, inspections)
			inspection := func(
				ctx context.Context,
				rows []directCodingComposePSRow,
				request directCodingDeploymentObservationRequest,
			) ([]directCodingObservedService, uint16, error) {
				return inspectDirectCodingDeploymentWithClient(ctx, client, rows, request)
			}
			readinessCalls := 0
			readiness := func(_ context.Context, host string, port uint16, path string) error {
				readinessCalls++
				if host != probeHost && host != request.AdvertisedHost ||
					port != probePort || path != directCodingDeploymentReadinessPath {
					return fmt.Errorf("unexpected readiness endpoint")
				}
				return nil
			}
			first, err := observeDirectCodingDeploymentWithObservers(
				context.Background(), output, request, inspection, readiness,
			)
			if err != nil {
				t.Fatal(err)
			}
			second, err := observeDirectCodingDeploymentWithObservers(
				context.Background(), output, request, inspection, readiness,
			)
			if err != nil {
				t.Fatal(err)
			}
			if first.SHA256 != second.SHA256 || first.ServicesSHA256 != second.ServicesSHA256 ||
				first.EndpointSHA256 != second.EndpointSHA256 {
				t.Fatal("identical observations produced different hashes")
			}
			if readinessCalls != 4 {
				t.Fatalf("readiness calls=%d want probe plus advertised for each observation", readinessCalls)
			}
			if len(first.SHA256) != 64 || len(first.Services) != len(fixture.services) ||
				first.Endpoint.Port != probePort || first.Endpoint.Host != "service.example.test" {
				t.Fatalf("unexpected canonical observation: %+v", first)
			}
			for index, service := range first.Services {
				if service.Service != fixture.services[index] || service.State != "running" ||
					service.Health != "healthy" || service.RestartPolicy != "unless-stopped" {
					t.Fatalf("unexpected service observation: %+v", service)
				}
			}
			encoded, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "DO_NOT_RECORD") || strings.Contains(string(encoded), "Env") {
				t.Fatal("observation retained container environment")
			}
		})
	}
}

func directCodingComposePSFixtureRow(
	id, project, service string,
	publishers []directCodingComposePublisher,
) map[string]any {
	return map[string]any{
		"ID": id, "Project": project, "Service": service,
		"State": "running", "Health": "healthy", "Publishers": publishers,
	}
}

func directCodingDockerInspectionFixture(
	containerID, imageID, project, service string,
	ports map[string]any,
) map[string]any {
	return map[string]any{
		"Id": containerID, "Image": imageID,
		"Config": map[string]any{
			"Env": []string{"TOKEN=DO_NOT_RECORD"},
			"Labels": map[string]string{
				"com.docker.compose.project": project,
				"com.docker.compose.service": service,
				"com.docker.compose.image":   imageID,
			},
		},
		"HostConfig": map[string]any{"RestartPolicy": map[string]string{"Name": "unless-stopped"}},
		"State": map[string]any{
			"Status": "running", "Running": true,
			"Health": map[string]string{"Status": "healthy"},
		},
		"NetworkSettings": map[string]any{"Ports": ports},
	}
}

func directCodingComposePSFixture(t *testing.T, rows []map[string]any) []byte {
	t.Helper()
	var output strings.Builder
	for _, row := range rows {
		encoded, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		output.Write(encoded)
		output.WriteByte('\n')
	}
	return []byte(output.String())
}

func reverseDirectCodingMaps(values []map[string]any) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func directCodingDockerObservationClient(
	t *testing.T,
	inspections map[string]any,
) *http.Client {
	t.Helper()
	return &http.Client{Transport: directCodingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		const prefix = "/containers/"
		const suffix = "/json"
		id := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, prefix), suffix)
		inspection, exists := inspections[id]
		if !strings.HasPrefix(request.URL.Path, prefix) ||
			!strings.HasSuffix(request.URL.Path, suffix) || !exists {
			return directCodingHTTPResponse(request, http.StatusNotFound, "application/json", []byte(`{}`)), nil
		}
		encoded, err := json.Marshal(inspection)
		if err != nil {
			t.Fatal(err)
		}
		return directCodingHTTPResponse(request, http.StatusOK, "application/json", encoded), nil
	})}
}

type directCodingRoundTripFunc func(*http.Request) (*http.Response, error)

func (function directCodingRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func directCodingHTTPResponse(
	request *http.Request,
	status int,
	contentType string,
	body []byte,
) *http.Response {
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}},
		Body: io.NopCloser(bytes.NewReader(body)), Request: request,
	}
}
