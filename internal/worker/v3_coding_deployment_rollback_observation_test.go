package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDirectCodingDeploymentRollbackObservationIsCanonicalForUnrelatedFixtures(t *testing.T) {
	t.Parallel()
	projectLabel := func(project string) map[string]string {
		return map[string]string{"com.docker.compose.project": project}
	}
	fixtures := []struct {
		name        string
		project     string
		markerSHA   string
		containers  []directCodingRollbackDockerResource
		networks    []directCodingRollbackDockerResource
		volumes     []directCodingRollbackDockerResource
		wantIDs     []string
		wantNets    []string
		wantVolumes []string
		wantClean   bool
	}{
		{
			name: "document catalog clean", project: "fixture-document-catalog",
			containers: []directCodingRollbackDockerResource{},
			networks:   []directCodingRollbackDockerResource{},
			volumes:    []directCodingRollbackDockerResource{},
			wantIDs:    []string{}, wantNets: []string{}, wantVolumes: []string{}, wantClean: true,
		},
		{
			name: "shipment ledger retains state volume", project: "fixture-shipment-ledger",
			markerSHA: strings.Repeat("e", 64),
			containers: []directCodingRollbackDockerResource{
				{ID: strings.Repeat("b", 64), Labels: projectLabel("fixture-shipment-ledger")},
				{ID: strings.Repeat("a", 64), Labels: projectLabel("fixture-shipment-ledger")},
			},
			networks: []directCodingRollbackDockerResource{
				{ID: strings.Repeat("d", 64), Labels: projectLabel("fixture-shipment-ledger")},
				{ID: strings.Repeat("c", 64), Labels: projectLabel("fixture-shipment-ledger")},
			},
			volumes: []directCodingRollbackDockerResource{
				{Name: "fixture-shipment-ledger_state", Labels: projectLabel("fixture-shipment-ledger")},
				{Name: "fixture-shipment-ledger_cache", Labels: projectLabel("fixture-shipment-ledger")},
			},
			wantIDs:     []string{strings.Repeat("a", 64), strings.Repeat("b", 64)},
			wantNets:    []string{strings.Repeat("c", 64), strings.Repeat("d", 64)},
			wantVolumes: []string{"fixture-shipment-ledger_cache", "fixture-shipment-ledger_state"},
			wantClean:   false,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			plan := directCodingRollbackObservationTestPlan(t, fixture.project, fixture.markerSHA)
			client, calls := directCodingRollbackObservationTestClient(
				t, fixture.project, fixture.containers, fixture.networks, fixture.volumes,
			)
			first, err := observeDirectCodingDeploymentRollbackWithClient(
				context.Background(), client, plan,
			)
			if err != nil {
				t.Fatal(err)
			}
			second, err := observeDirectCodingDeploymentRollbackWithClient(
				context.Background(), client, plan,
			)
			if err != nil {
				t.Fatal(err)
			}
			if *calls != 6 {
				t.Fatalf("Docker GET calls=%d want 6", *calls)
			}
			if !reflect.DeepEqual(first.ContainerIDs, fixture.wantIDs) ||
				!reflect.DeepEqual(first.NetworkIDs, fixture.wantNets) ||
				!reflect.DeepEqual(first.VolumeNames, fixture.wantVolumes) {
				t.Fatalf("unexpected canonical resources: %+v", first)
			}
			clean := len(first.ContainerIDs) == 0 && len(first.NetworkIDs) == 0 && len(first.VolumeNames) == 0
			if clean != fixture.wantClean {
				t.Fatalf("clean=%t want %t", clean, fixture.wantClean)
			}
			if first.Schema != directCodingDeploymentRollbackObservationSchema ||
				first.ComposeProject != fixture.project ||
				first.PostconditionSHA256 != plan.PostconditionSHA256 ||
				first.SHA256 != second.SHA256 {
				t.Fatalf("unexpected observation authority: %+v", first)
			}
			bound, encoded, err := queue.BindGeneratedWorkloadDeploymentRollbackObservation(plan, first)
			if err != nil {
				t.Fatal(err)
			}
			if first.SHA256 != bound.SHA256 {
				t.Fatal("observation SHA does not digest the exact canonical JSON bytes")
			}
			if fixture.markerSHA != "" && bytes.Contains([]byte(encoded), []byte(fixture.markerSHA)) {
				t.Fatal("expected state-marker digest leaked into runtime resource observation")
			}
		})
	}
}

func TestDirectCodingDeploymentRollbackObservationRejectsUnboundResources(t *testing.T) {
	t.Parallel()
	plan := directCodingRollbackObservationTestPlan(t, "fixture-resource-rejection", "")
	wrong := []directCodingRollbackDockerResource{{
		ID:     strings.Repeat("a", 64),
		Labels: map[string]string{"com.docker.compose.project": "another-project"},
	}}
	client, _ := directCodingRollbackObservationTestClient(
		t, plan.ComposeProject, wrong, []directCodingRollbackDockerResource{},
		[]directCodingRollbackDockerResource{},
	)
	_, err := observeDirectCodingDeploymentRollbackWithClient(context.Background(), client, plan)
	if err == nil || !strings.Contains(err.Error(), "project label is invalid") {
		t.Fatalf("unexpected mismatched-label result: %v", err)
	}
	plan.PostconditionSHA256 = strings.Repeat("f", 64)
	_, err = observeDirectCodingDeploymentRollbackWithClient(context.Background(), client, plan)
	if err == nil || !strings.Contains(err.Error(), "postcondition authority is invalid") {
		t.Fatalf("unexpected unbound-plan result: %v", err)
	}
}

func directCodingRollbackObservationTestPlan(
	t *testing.T,
	project string,
	markerSHA string,
) queue.GeneratedWorkloadDeploymentRollbackPlan {
	t.Helper()
	plan := queue.GeneratedWorkloadDeploymentRollbackPlan{
		Policy:                  queue.GeneratedWorkloadDeploymentRollbackDestroyFirstV1,
		MaxAttempts:             queue.MaxGeneratedWorkloadDeploymentRollbackAttempts,
		ComposeProject:          project,
		ResourceObservation:     queue.GeneratedWorkloadDeploymentRollbackResourcesV1,
		RequireContainerAbsence: true,
		RequireNetworkAbsence:   true,
		RequireVolumeAbsence:    true,
		StateMarkerSHA256:       markerSHA,
	}
	var err error
	plan.PostconditionJSON, plan.PostconditionSHA256, err =
		queue.CanonicalGeneratedWorkloadDeploymentRollbackPostcondition(plan)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func directCodingRollbackObservationTestClient(
	t *testing.T,
	project string,
	containers []directCodingRollbackDockerResource,
	networks []directCodingRollbackDockerResource,
	volumes []directCodingRollbackDockerResource,
) (*http.Client, *int) {
	t.Helper()
	calls := 0
	client := &http.Client{Transport: directCodingRollbackObservationRoundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls++
			if request.Method != http.MethodGet || request.Host != "docker.local" || request.Body != nil {
				t.Fatalf("rollback observation was not one side-effect-free Docker GET: %+v", request)
			}
			var filters map[string][]string
			if err := json.Unmarshal([]byte(request.URL.Query().Get("filters")), &filters); err != nil {
				t.Fatalf("decode Docker filters: %v", err)
			}
			wantFilter := map[string][]string{
				"label": {"com.docker.compose.project=" + project},
			}
			if !reflect.DeepEqual(filters, wantFilter) {
				t.Fatalf("filters=%v want %v", filters, wantFilter)
			}
			var body any
			switch request.URL.Path {
			case "/containers/json":
				if request.URL.Query().Get("all") != "1" {
					t.Fatal("container observation omitted all stopped containers")
				}
				body = containers
			case "/networks":
				body = networks
			case "/volumes":
				body = directCodingRollbackDockerVolumes{Volumes: volumes, Warnings: []string{}}
			default:
				t.Fatalf("unexpected Docker observation path %q", request.URL.Path)
			}
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(encoded)),
				Request:    request,
			}, nil
		},
	)}
	return client, &calls
}

type directCodingRollbackObservationRoundTripFunc func(*http.Request) (*http.Response, error)

func (function directCodingRollbackObservationRoundTripFunc) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return function(request)
}
