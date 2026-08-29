package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDirectCodingComposeObservationRejectsMalformedAndMismatchedOutput(t *testing.T) {
	t.Parallel()
	valid := directCodingComposePSFixture(t, []map[string]any{
		directCodingComposePSFixtureRow(strings.Repeat("a", 12), "fixture-project", "api", nil),
	})
	cases := []struct {
		name     string
		output   []byte
		project  string
		expected []string
		want     string
	}{
		{name: "malformed", output: []byte(`{"ID":`), project: "fixture-project", expected: []string{"api"}, want: "decode"},
		{name: "unknown field", output: []byte(`{"ID":"aaaaaaaaaaaa","Project":"fixture-project","Service":"api","State":"running","Health":"healthy","Publishers":[],"Unexpected":true}`), project: "fixture-project", expected: []string{"api"}, want: "unknown field"},
		{name: "project mismatch", output: valid, project: "other-project", expected: []string{"api"}, want: "expected"},
		{name: "missing health", output: []byte(`{"ID":"aaaaaaaaaaaa","Project":"fixture-project","Service":"api","State":"running","Publishers":[]}`), project: "fixture-project", expected: []string{"api"}, want: "not running and healthy"},
		{name: "array encoding", output: []byte(`[{}]`), project: "fixture-project", expected: []string{"api"}, want: "decode"},
		{name: "carriage return", output: append(append([]byte(nil), valid...), '\r'), project: "fixture-project", expected: []string{"api"}, want: "invalid bytes"},
		{name: "wrong service set", output: valid, project: "fixture-project", expected: []string{"edge"}, want: "differs"},
		{name: "oversize", output: bytes.Repeat([]byte{'x'}, directCodingComposePSLimit+1), project: "fixture-project", expected: []string{"api"}, want: "byte limit"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseDirectCodingComposePS(testCase.output, testCase.project, testCase.expected)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error=%v, want substring %q", err, testCase.want)
			}
		})
	}
}

func TestDirectCodingDockerInspectionRejectsUntrustedRuntimeState(t *testing.T) {
	t.Parallel()
	containerID := strings.Repeat("a", 64)
	imageID := "sha256:" + strings.Repeat("b", 64)
	request := directCodingDeploymentObservationRequest{
		Project: "fixture-project", ExpectedServices: []string{"gateway"},
		GatewayService: "gateway", GatewayContainerPort: 80,
		BindAddress: "127.0.0.1", ProbeHost: "127.0.0.1",
		AdvertisedHost: "service.example.test", ReadinessPath: directCodingDeploymentReadinessPath,
	}
	row := directCodingComposePSRow{
		ID: containerID[:12], Project: request.Project, Service: "gateway",
		State: "running", Health: "healthy",
		Publishers: []directCodingComposePublisher{{
			URL: "127.0.0.1", TargetPort: 80, PublishedPort: 41000, Protocol: "tcp",
		}},
	}
	base := directCodingDockerInspectionFixture(
		containerID, imageID, request.Project, row.Service,
		map[string]any{"80/tcp": []map[string]string{{"HostIp": "127.0.0.1", "HostPort": "41000"}}},
	)
	cases := []struct {
		name   string
		mutate func(*directCodingDockerInspection)
		want   string
	}{
		{name: "project label", mutate: func(value *directCodingDockerInspection) { value.Config.Labels["com.docker.compose.project"] = "other" }, want: "labels disagree"},
		{name: "service label", mutate: func(value *directCodingDockerInspection) { value.Config.Labels["com.docker.compose.service"] = "other" }, want: "labels disagree"},
		{name: "mutable image", mutate: func(value *directCodingDockerInspection) { value.Image = "gateway:latest" }, want: "immutable"},
		{name: "unhealthy", mutate: func(value *directCodingDockerInspection) { value.State.Health.Status = "unhealthy" }, want: "not running and healthy"},
		{name: "restart policy", mutate: func(value *directCodingDockerInspection) { value.HostConfig.RestartPolicy.Name = "no" }, want: "restart policy"},
		{name: "published host", mutate: func(value *directCodingDockerInspection) { value.NetworkSettings.Ports["80/tcp"][0].HostIP = "0.0.0.0" }, want: "server authority"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			inspection := decodeDirectCodingDockerInspectionFixture(t, base)
			testCase.mutate(&inspection)
			_, _, err := validateDirectCodingContainerInspection(row, inspection, request)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error=%v, want substring %q", err, testCase.want)
			}
		})
	}
}

func TestDirectCodingDockerInspectionRejectsOversizeResponse(t *testing.T) {
	t.Parallel()
	client := &http.Client{Transport: directCodingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := append([]byte(`{"Padding":"`), bytes.Repeat([]byte{'x'}, directCodingDockerInspectLimit+1)...)
		body = append(body, []byte(`"}`)...)
		return directCodingHTTPResponse(request, http.StatusOK, "application/json", body), nil
	})}
	_, err := inspectDirectCodingContainer(context.Background(), client, strings.Repeat("a", 12))
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("error=%v", err)
	}
}

func TestDirectCodingObservationAllowsAllInterfaceBindOnlyForDockerAuthority(t *testing.T) {
	t.Parallel()
	request := directCodingDeploymentObservationRequest{
		Project: "fixture-project", ExpectedServices: []string{"gateway"},
		GatewayService: "gateway", GatewayContainerPort: 80,
		BindAddress: "0.0.0.0", ProbeHost: "127.0.0.1",
		AdvertisedHost: "service.example.test", ReadinessPath: directCodingDeploymentReadinessPath,
	}
	if err := validateDirectCodingObservationRequest(request); err != nil {
		t.Fatalf("all-interface bind was rejected: %v", err)
	}
	request.ProbeHost = "0.0.0.0"
	if err := validateDirectCodingObservationRequest(request); err == nil {
		t.Fatal("unspecified probe host was accepted")
	}
	request.ProbeHost = "127.0.0.1"
	request.AdvertisedHost = "0.0.0.0"
	if err := validateDirectCodingObservationRequest(request); err == nil {
		t.Fatal("unspecified advertised host was accepted")
	}
}

func decodeDirectCodingDockerInspectionFixture(
	t *testing.T,
	fixture map[string]any,
) directCodingDockerInspection {
	t.Helper()
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var inspection directCodingDockerInspection
	if err := json.Unmarshal(encoded, &inspection); err != nil {
		t.Fatal(err)
	}
	return inspection
}
