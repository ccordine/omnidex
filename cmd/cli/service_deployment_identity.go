package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/envfile"
)

const dockerContextEnvironmentKey = "DOCKER_CONTEXT"
const composeProjectEnvironmentKey = "COMPOSE_PROJECT_NAME"

type serviceDeploymentIdentity struct {
	DockerContext  string
	ComposeProject string
}

func readServiceDeploymentIdentity(root string) (serviceDeploymentIdentity, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return serviceDeploymentIdentity{}, fmt.Errorf("service deployment root is required")
	}
	path := filepath.Join(root, ".env")
	values, err := envfile.Read(path)
	if err != nil {
		return serviceDeploymentIdentity{}, fmt.Errorf("read service deployment identity from %s: %w", path, err)
	}
	contextName, found := values[dockerContextEnvironmentKey]
	if !found {
		return serviceDeploymentIdentity{}, fmt.Errorf("managed environment %s does not define %s", path, dockerContextEnvironmentKey)
	}
	projectName, found := values[composeProjectEnvironmentKey]
	if !found {
		return serviceDeploymentIdentity{}, fmt.Errorf("managed environment %s does not define %s", path, composeProjectEnvironmentKey)
	}
	if err := validateServiceDeploymentIdentifier(dockerContextEnvironmentKey, contextName); err != nil {
		return serviceDeploymentIdentity{}, err
	}
	if err := validateServiceDeploymentIdentifier(composeProjectEnvironmentKey, projectName); err != nil {
		return serviceDeploymentIdentity{}, err
	}
	return serviceDeploymentIdentity{DockerContext: contextName, ComposeProject: projectName}, nil
}

func validateServiceDeploymentIdentifier(label, value string) error {
	if value == "" {
		return fmt.Errorf("%s must be explicit and non-empty", label)
	}
	for index, character := range value {
		valid := character >= 'A' && character <= 'Z' ||
			character >= 'a' && character <= 'z' ||
			character >= '0' && character <= '9' ||
			index > 0 && (character == '_' || character == '.' || character == '-')
		if !valid {
			return fmt.Errorf("%s contains unsupported characters", label)
		}
	}
	return nil
}

func (identity serviceDeploymentIdentity) composeCommandPrefix() []string {
	return []string{"docker", "--context", identity.DockerContext, "compose", "-p", identity.ComposeProject}
}

func (identity serviceDeploymentIdentity) dockerCommandPrefix() []string {
	return []string{"docker", "--context", identity.DockerContext}
}
