package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type directCodingDeploymentProjectResources struct {
	ContainerIDs []string
	NetworkIDs   []string
	VolumeNames  []string
}

func observeDirectCodingDeploymentProjectResourcesWithClient(
	ctx context.Context,
	client *http.Client,
	project string,
) (directCodingDeploymentProjectResources, error) {
	if ctx == nil || client == nil || !v3DeploymentComposeProjectPattern.MatchString(project) {
		return directCodingDeploymentProjectResources{}, fmt.Errorf(
			"deployment project-resource observation authority is invalid",
		)
	}
	filters, err := json.Marshal(map[string][]string{
		"label": {"com.docker.compose.project=" + project},
	})
	if err != nil {
		return directCodingDeploymentProjectResources{}, fmt.Errorf(
			"encode deployment project resource filter: %w", err,
		)
	}
	containers := make([]directCodingRollbackDockerResource, 0)
	if err := getDirectCodingDeploymentRollbackResources(
		ctx, client, "/containers/json",
		url.Values{"all": {"1"}, "filters": {string(filters)}}, &containers,
	); err != nil {
		return directCodingDeploymentProjectResources{}, fmt.Errorf(
			"observe deployment project containers: %w", err,
		)
	}
	networks := make([]directCodingRollbackDockerResource, 0)
	if err := getDirectCodingDeploymentRollbackResources(
		ctx, client, "/networks", url.Values{"filters": {string(filters)}}, &networks,
	); err != nil {
		return directCodingDeploymentProjectResources{}, fmt.Errorf(
			"observe deployment project networks: %w", err,
		)
	}
	volumes := directCodingRollbackDockerVolumes{
		Volumes: make([]directCodingRollbackDockerResource, 0),
	}
	if err := getDirectCodingDeploymentRollbackResources(
		ctx, client, "/volumes", url.Values{"filters": {string(filters)}}, &volumes,
	); err != nil {
		return directCodingDeploymentProjectResources{}, fmt.Errorf(
			"observe deployment project volumes: %w", err,
		)
	}
	if len(volumes.Warnings) != 0 {
		return directCodingDeploymentProjectResources{}, fmt.Errorf(
			"deployment project volume observation returned warnings",
		)
	}
	containerIDs, err := canonicalDirectCodingRollbackResourceIDs(containers, project)
	if err != nil {
		return directCodingDeploymentProjectResources{}, fmt.Errorf("containers: %w", err)
	}
	networkIDs, err := canonicalDirectCodingRollbackResourceIDs(networks, project)
	if err != nil {
		return directCodingDeploymentProjectResources{}, fmt.Errorf("networks: %w", err)
	}
	volumeNames, err := canonicalDirectCodingRollbackVolumeNames(volumes.Volumes, project)
	if err != nil {
		return directCodingDeploymentProjectResources{}, fmt.Errorf("volumes: %w", err)
	}
	return directCodingDeploymentProjectResources{
		ContainerIDs: containerIDs, NetworkIDs: networkIDs, VolumeNames: volumeNames,
	}, nil
}
