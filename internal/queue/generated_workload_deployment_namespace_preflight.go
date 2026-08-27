package queue

import (
	"fmt"
	"regexp"
)

const (
	GeneratedWorkloadDeploymentNamespacePreflightV1 = "omnidex.generated-deployment-namespace-preflight.v1"
	GeneratedWorkloadDeploymentNamespaceMetadataKey = "namespace_preflight"
	maxGeneratedDeploymentNamespaceResources        = 1024
)

var generatedDeploymentNamespaceVolumePattern = regexp.MustCompile(
	`^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$`,
)

type GeneratedWorkloadDeploymentNamespacePreflight struct {
	Schema         string   `json:"schema"`
	ComposeProject string   `json:"compose_project"`
	ContainerIDs   []string `json:"container_ids"`
	NetworkIDs     []string `json:"network_ids"`
	VolumeNames    []string `json:"volume_names"`
	SHA256         string   `json:"sha256"`
}

func BindGeneratedWorkloadDeploymentNamespacePreflight(
	proof GeneratedWorkloadDeploymentNamespacePreflight,
) (GeneratedWorkloadDeploymentNamespacePreflight, string, error) {
	if proof.Schema != GeneratedWorkloadDeploymentNamespacePreflightV1 ||
		!generatedDeploymentProject.MatchString(proof.ComposeProject) {
		return GeneratedWorkloadDeploymentNamespacePreflight{}, "",
			fmt.Errorf("deployment namespace preflight authority is invalid")
	}
	if err := validateGeneratedDeploymentNamespaceResources(
		"container", proof.ContainerIDs, validSHA256Digest,
	); err != nil {
		return GeneratedWorkloadDeploymentNamespacePreflight{}, "", err
	}
	if err := validateGeneratedDeploymentNamespaceResources(
		"network", proof.NetworkIDs, validSHA256Digest,
	); err != nil {
		return GeneratedWorkloadDeploymentNamespacePreflight{}, "", err
	}
	if err := validateGeneratedDeploymentNamespaceResources(
		"volume", proof.VolumeNames, generatedDeploymentNamespaceVolumePattern.MatchString,
	); err != nil {
		return GeneratedWorkloadDeploymentNamespacePreflight{}, "", err
	}
	payload := struct {
		ComposeProject string   `json:"compose_project"`
		ContainerIDs   []string `json:"container_ids"`
		NetworkIDs     []string `json:"network_ids"`
		Schema         string   `json:"schema"`
		VolumeNames    []string `json:"volume_names"`
	}{
		proof.ComposeProject, proof.ContainerIDs, proof.NetworkIDs,
		proof.Schema, proof.VolumeNames,
	}
	encoded, err := canonicalGeneratedDeploymentJSON(payload)
	if err != nil {
		return GeneratedWorkloadDeploymentNamespacePreflight{}, "", err
	}
	digest := generatedDeploymentSHA(encoded)
	if proof.SHA256 != "" && proof.SHA256 != digest {
		return GeneratedWorkloadDeploymentNamespacePreflight{}, "",
			fmt.Errorf("deployment namespace preflight digest differs")
	}
	proof.SHA256 = digest
	return proof, encoded, nil
}

func GeneratedWorkloadDeploymentNamespaceVacant(
	proof GeneratedWorkloadDeploymentNamespacePreflight,
) bool {
	return len(proof.ContainerIDs) == 0 && len(proof.NetworkIDs) == 0 &&
		len(proof.VolumeNames) == 0
}

func validateGeneratedDeploymentNamespaceResources(
	label string,
	values []string,
	valid func(string) bool,
) error {
	if values == nil || len(values) > maxGeneratedDeploymentNamespaceResources {
		return fmt.Errorf("deployment namespace %s resource set is invalid", label)
	}
	previous := ""
	for index, value := range values {
		if !valid(value) || index > 0 && value <= previous {
			return fmt.Errorf(
				"deployment namespace %s resources must be valid, sorted, and unique", label,
			)
		}
		previous = value
	}
	return nil
}
