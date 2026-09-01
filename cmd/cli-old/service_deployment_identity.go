package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/envfile"
)

const dockerContextEnvironmentKey = "DOCKER_CONTEXT"
const rootfulDockerContextName = "default"
const rootfulDockerSocketURL = "unix:///var/run/docker.sock"
const composeProjectEnvironmentKey = "COMPOSE_PROJECT_NAME"
const hostUIDEnvironmentKey = "HOST_UID"
const hostGIDEnvironmentKey = "HOST_GID"

const maxServiceHostID = uint64(4294967294)

type serviceDeploymentIdentity struct {
	DockerContext  string
	ComposeProject string
	HostUID        string
	HostGID        string
}

func readServiceDeploymentIdentity(root string) (serviceDeploymentIdentity, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return serviceDeploymentIdentity{}, fmt.Errorf("service deployment root is required")
	}
	path := filepath.Join(root, ".env")
	values, raw, err := readExactServiceDeploymentEnvironment(path)
	if err != nil {
		return serviceDeploymentIdentity{}, fmt.Errorf("read service deployment identity from %s: %w", path, err)
	}
	if err := rejectManagedDockerRoutingEnvironment(values); err != nil {
		return serviceDeploymentIdentity{}, err
	}
	contextName, found := values[dockerContextEnvironmentKey]
	if !found {
		return serviceDeploymentIdentity{}, fmt.Errorf("managed environment %s does not define %s", path, dockerContextEnvironmentKey)
	}
	projectName, found := values[composeProjectEnvironmentKey]
	if !found {
		return serviceDeploymentIdentity{}, fmt.Errorf("managed environment %s does not define %s", path, composeProjectEnvironmentKey)
	}
	if err := validateServiceRootfulDockerContext(contextName); err != nil {
		return serviceDeploymentIdentity{}, err
	}
	if err := validateServiceDeploymentIdentifier(composeProjectEnvironmentKey, projectName); err != nil {
		return serviceDeploymentIdentity{}, err
	}
	hostUID, err := readServiceHostID(values, raw, path, hostUIDEnvironmentKey)
	if err != nil {
		return serviceDeploymentIdentity{}, err
	}
	hostGID, err := readServiceHostID(values, raw, path, hostGIDEnvironmentKey)
	if err != nil {
		return serviceDeploymentIdentity{}, err
	}
	return serviceDeploymentIdentity{
		DockerContext:  contextName,
		ComposeProject: projectName,
		HostUID:        hostUID,
		HostGID:        hostGID,
	}, nil
}

func rejectManagedDockerRoutingEnvironment(values map[string]string) error {
	for _, key := range []string{
		"DOCKER_SOCKET_PATH", "DOCKER_HOST", "DOCKER_CONFIG", "DOCKER_CERT_PATH",
		"DOCKER_TLS", "DOCKER_TLS_VERIFY", "BUILDKIT_HOST",
		"BUILDKIT_TLS_SERVER_NAME", "BUILDKIT_TLS_CACERT", "BUILDKIT_TLS_CERT",
		"BUILDKIT_TLS_KEY", "BUILDX_BUILDER", "BUILDX_CONFIG",
	} {
		if _, found := values[key]; found {
			return fmt.Errorf(
				"managed environment must not define %s; Docker authority is invariant",
				key,
			)
		}
	}
	return nil
}

func validateServiceRootfulDockerContext(value string) error {
	if value != rootfulDockerContextName {
		return fmt.Errorf(
			"%s must be %q; rootless Docker is unsupported",
			dockerContextEnvironmentKey, rootfulDockerContextName,
		)
	}
	return nil
}

func readExactServiceDeploymentEnvironment(path string) (map[string]string, []byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("environment file must be a regular file: %s", path)
	}
	if info.Size() > envfile.MaxBytes {
		return nil, nil, fmt.Errorf("environment file exceeds %d bytes", envfile.MaxBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, envfile.MaxBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > envfile.MaxBytes {
		return nil, nil, fmt.Errorf("environment file exceeds %d bytes", envfile.MaxBytes)
	}
	values, err := envfile.Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	return values, raw, nil
}

func readServiceHostID(values map[string]string, raw []byte, path, key string) (string, error) {
	value, found := values[key]
	if !found {
		return "", fmt.Errorf("managed environment %s does not define %s", path, key)
	}
	exact := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, key+"=") {
			exact = strings.TrimPrefix(line, key+"=")
			break
		}
	}
	if exact != value {
		return "", fmt.Errorf("%s must be one exact positive numeric host identity", key)
	}
	if err := validateServiceHostID(key, value); err != nil {
		return "", err
	}
	return value, nil
}

func validateServiceHostID(key, value string) error {
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil || parsed == 0 || parsed > maxServiceHostID || strconv.FormatUint(parsed, 10) != value {
		return fmt.Errorf("%s must be one exact positive numeric host identity", key)
	}
	return nil
}

func validateServiceRuntimeUser(value string) error {
	uid, gid, found := strings.Cut(value, ":")
	if !found || strings.Contains(gid, ":") {
		return fmt.Errorf("service runtime user must be one exact positive numeric UID:GID")
	}
	if err := validateServiceHostID(hostUIDEnvironmentKey, uid); err != nil {
		return fmt.Errorf("service runtime user must be one exact positive numeric UID:GID")
	}
	if err := validateServiceHostID(hostGIDEnvironmentKey, gid); err != nil {
		return fmt.Errorf("service runtime user must be one exact positive numeric UID:GID")
	}
	return nil
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

func (identity serviceDeploymentIdentity) runtimeUser() string {
	return identity.HostUID + ":" + identity.HostGID
}
