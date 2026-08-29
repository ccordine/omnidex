package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

const (
	directCodingDeploymentRollbackObservationSchema  = queue.GeneratedWorkloadDeploymentRollbackObservationV1
	directCodingDeploymentRollbackObservationLimit   = 1 << 20
	directCodingDeploymentRollbackObservationTimeout = 8 * time.Second
)

var (
	directCodingRollbackDockerIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	directCodingRollbackVolumePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$`)
)

type directCodingRollbackDockerResource struct {
	ID     string            `json:"Id"`
	Name   string            `json:"Name"`
	Labels map[string]string `json:"Labels"`
}

type directCodingRollbackDockerVolumes struct {
	Volumes  []directCodingRollbackDockerResource `json:"Volumes"`
	Warnings []string                             `json:"Warnings"`
}

func observeDirectCodingDeploymentRollback(
	parent context.Context,
	socketPath string,
	plan queue.GeneratedWorkloadDeploymentRollbackPlan,
) (queue.GeneratedWorkloadDeploymentRollbackObservation, error) {
	if parent == nil {
		return queue.GeneratedWorkloadDeploymentRollbackObservation{}, fmt.Errorf(
			"deployment rollback observation requires a context",
		)
	}
	if err := validateV3DockerSocket(socketPath); err != nil {
		return queue.GeneratedWorkloadDeploymentRollbackObservation{}, err
	}
	if err := validateV3DockerDaemon(parent, socketPath); err != nil {
		return queue.GeneratedWorkloadDeploymentRollbackObservation{}, err
	}
	ctx, cancel := context.WithTimeout(parent, directCodingDeploymentRollbackObservationTimeout)
	defer cancel()
	transport := directCodingDockerUnixTransport(socketPath)
	defer transport.CloseIdleConnections()
	return observeDirectCodingDeploymentRollbackWithClient(
		ctx, &http.Client{Transport: transport}, plan,
	)
}

func observeDirectCodingDeploymentRollbackWithClient(
	ctx context.Context,
	client *http.Client,
	plan queue.GeneratedWorkloadDeploymentRollbackPlan,
) (queue.GeneratedWorkloadDeploymentRollbackObservation, error) {
	if ctx == nil || client == nil {
		return queue.GeneratedWorkloadDeploymentRollbackObservation{}, fmt.Errorf(
			"deployment rollback observation requires context and Docker client authority",
		)
	}
	if err := validateDirectCodingDeploymentRollbackObservationPlan(plan); err != nil {
		return queue.GeneratedWorkloadDeploymentRollbackObservation{}, err
	}
	resources, err := observeDirectCodingDeploymentProjectResourcesWithClient(
		ctx, client, plan.ComposeProject,
	)
	if err != nil {
		return queue.GeneratedWorkloadDeploymentRollbackObservation{}, fmt.Errorf(
			"observe deployment rollback resources: %w", err,
		)
	}
	observation := queue.GeneratedWorkloadDeploymentRollbackObservation{
		Schema:              directCodingDeploymentRollbackObservationSchema,
		ComposeProject:      plan.ComposeProject,
		ContainerIDs:        resources.ContainerIDs,
		NetworkIDs:          resources.NetworkIDs,
		VolumeNames:         resources.VolumeNames,
		PostconditionSHA256: plan.PostconditionSHA256,
	}
	bound, _, err := queue.BindGeneratedWorkloadDeploymentRollbackObservation(plan, observation)
	if err != nil {
		return queue.GeneratedWorkloadDeploymentRollbackObservation{}, fmt.Errorf(
			"encode canonical deployment rollback observation: %w", err,
		)
	}
	return bound, nil
}

func validateDirectCodingDeploymentRollbackObservationPlan(
	plan queue.GeneratedWorkloadDeploymentRollbackPlan,
) error {
	if plan.Policy != queue.GeneratedWorkloadDeploymentRollbackDestroyFirstV1 ||
		plan.ResourceObservation != queue.GeneratedWorkloadDeploymentRollbackResourcesV1 ||
		!plan.RequireContainerAbsence || !plan.RequireNetworkAbsence || !plan.RequireVolumeAbsence ||
		!v3DeploymentComposeProjectPattern.MatchString(plan.ComposeProject) ||
		!directCodingRollbackDockerIDPattern.MatchString(plan.PostconditionSHA256) ||
		plan.StateMarkerSHA256 != "" && !directCodingRollbackDockerIDPattern.MatchString(plan.StateMarkerSHA256) {
		return fmt.Errorf("deployment rollback resource-observation authority is invalid")
	}
	postconditionJSON, postconditionSHA, err :=
		queue.CanonicalGeneratedWorkloadDeploymentRollbackPostcondition(plan)
	if err != nil || postconditionJSON != plan.PostconditionJSON || postconditionSHA != plan.PostconditionSHA256 {
		return fmt.Errorf("deployment rollback postcondition authority is invalid: %w", err)
	}
	return nil
}

func getDirectCodingDeploymentRollbackResources(
	ctx context.Context,
	client *http.Client,
	path string,
	query url.Values,
	destination any,
) error {
	request, err := http.NewRequestWithContext(
		ctx, http.MethodGet, "http://docker.local"+path+"?"+query.Encode(), nil,
	)
	if err != nil {
		return fmt.Errorf("construct Docker resource request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("execute Docker resource request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Docker resource request returned HTTP status %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("Docker resource request returned a non-JSON content type")
	}
	limited := &io.LimitedReader{R: response.Body, N: directCodingDeploymentRollbackObservationLimit + 1}
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode Docker resource response: %w", err)
	}
	if limited.N <= 0 {
		return fmt.Errorf(
			"Docker resource response exceeds the %d-byte limit",
			directCodingDeploymentRollbackObservationLimit,
		)
	}
	if err := requireDirectCodingJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func canonicalDirectCodingRollbackResourceIDs(
	resources []directCodingRollbackDockerResource,
	project string,
) ([]string, error) {
	values := make([]string, 0, len(resources))
	for _, resource := range resources {
		if !directCodingRollbackDockerIDPattern.MatchString(resource.ID) ||
			resource.Labels["com.docker.compose.project"] != project {
			return nil, fmt.Errorf("Docker resource identity or project label is invalid")
		}
		values = append(values, resource.ID)
	}
	return sortUniqueDirectCodingRollbackResources(values)
}

func canonicalDirectCodingRollbackVolumeNames(
	resources []directCodingRollbackDockerResource,
	project string,
) ([]string, error) {
	values := make([]string, 0, len(resources))
	for _, resource := range resources {
		if !directCodingRollbackVolumePattern.MatchString(resource.Name) ||
			resource.Labels["com.docker.compose.project"] != project {
			return nil, fmt.Errorf("Docker volume identity or project label is invalid")
		}
		values = append(values, resource.Name)
	}
	return sortUniqueDirectCodingRollbackResources(values)
}

func sortUniqueDirectCodingRollbackResources(values []string) ([]string, error) {
	sort.Strings(values)
	for index := 1; index < len(values); index++ {
		if values[index] == values[index-1] {
			return nil, fmt.Errorf("Docker resource observation contains a duplicate identity")
		}
	}
	return values, nil
}
