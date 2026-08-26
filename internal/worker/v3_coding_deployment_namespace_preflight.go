package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/queue"
)

var errDirectCodingDeploymentNamespaceOccupied = errors.New(
	"deployment Compose namespace contains pre-existing Docker resources",
)

func proveDirectCodingDeploymentNamespaceVacant(
	parent context.Context,
	socketPath string,
	project string,
) (queue.GeneratedWorkloadDeploymentNamespacePreflight, error) {
	if parent == nil {
		return queue.GeneratedWorkloadDeploymentNamespacePreflight{}, fmt.Errorf(
			"deployment namespace preflight requires a context",
		)
	}
	if err := validateV3DockerSocket(socketPath); err != nil {
		return queue.GeneratedWorkloadDeploymentNamespacePreflight{}, err
	}
	if err := validateV3RootlessDockerDaemon(parent, socketPath); err != nil {
		return queue.GeneratedWorkloadDeploymentNamespacePreflight{}, err
	}
	ctx, cancel := context.WithTimeout(parent, directCodingDockerObserveTimeout)
	defer cancel()
	transport := directCodingDockerUnixTransport(socketPath)
	defer transport.CloseIdleConnections()
	return proveDirectCodingDeploymentNamespaceVacantWithClient(
		ctx, &http.Client{Transport: transport}, project,
	)
}

func proveDirectCodingDeploymentNamespaceVacantWithClient(
	ctx context.Context,
	client *http.Client,
	project string,
) (queue.GeneratedWorkloadDeploymentNamespacePreflight, error) {
	resources, err := observeDirectCodingDeploymentProjectResourcesWithClient(ctx, client, project)
	if err != nil {
		return queue.GeneratedWorkloadDeploymentNamespacePreflight{}, err
	}
	proof, _, err := queue.BindGeneratedWorkloadDeploymentNamespacePreflight(
		queue.GeneratedWorkloadDeploymentNamespacePreflight{
			Schema:         queue.GeneratedWorkloadDeploymentNamespacePreflightV1,
			ComposeProject: project,
			ContainerIDs:   resources.ContainerIDs,
			NetworkIDs:     resources.NetworkIDs,
			VolumeNames:    resources.VolumeNames,
		},
	)
	if err != nil {
		return queue.GeneratedWorkloadDeploymentNamespacePreflight{}, err
	}
	if !queue.GeneratedWorkloadDeploymentNamespaceVacant(proof) {
		return queue.GeneratedWorkloadDeploymentNamespacePreflight{}, fmt.Errorf(
			"%w: deployment Compose project %s already contains Docker resources (%d containers, %d networks, %d volumes)",
			errDirectCodingDeploymentNamespaceOccupied,
			project, len(proof.ContainerIDs), len(proof.NetworkIDs), len(proof.VolumeNames),
		)
	}
	return proof, nil
}
