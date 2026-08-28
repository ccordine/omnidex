package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	v3RootfulDockerHost       = "unix:///var/run/docker.sock"
	v3RootfulDockerSocketPath = "/var/run/docker.sock"
)

func resolveV3CommandExecution(
	ctx context.Context,
	root, program string,
) (string, []string, error) {
	if program != "docker" {
		return root, nil, nil
	}
	executionRoot, err := resolveV3DockerCommandRoot(
		root, os.Getenv("WORKSPACE_ROOT"), os.Getenv("HOST_WORKSPACE_PATH"),
	)
	if err != nil {
		return "", nil, err
	}
	_, _, err = resolveV3DockerHost(os.Getenv("DOCKER_HOST"))
	if err != nil {
		return "", nil, err
	}
	if err := requireV3RootfulDockerAuthority(ctx); err != nil {
		return "", nil, err
	}
	return executionRoot, nil, nil
}

func requireV3RootfulDockerAuthority(ctx context.Context) error {
	if err := validateV3DockerSocket(v3RootfulDockerSocketPath); err != nil {
		return err
	}
	return validateV3DockerDaemon(ctx, v3RootfulDockerSocketPath)
}

func v3DockerCLIArguments(args []string) []string {
	arguments := make([]string, 0, len(args)+2)
	arguments = append(arguments, "--host", v3RootfulDockerHost)
	return append(arguments, args...)
}

func resolveV3DockerCommandRoot(commandRoot, runtimeRoot, hostRoot string) (string, error) {
	if commandRoot == "" || runtimeRoot == "" || hostRoot == "" {
		return "", fmt.Errorf(
			"Docker command execution requires command, WORKSPACE_ROOT, and HOST_WORKSPACE_PATH roots",
		)
	}
	for _, candidate := range []struct {
		name string
		root string
	}{
		{name: "command", root: commandRoot},
		{name: "runtime", root: runtimeRoot},
		{name: "host", root: hostRoot},
	} {
		if strings.TrimSpace(candidate.root) != candidate.root ||
			!filepath.IsAbs(candidate.root) || filepath.Clean(candidate.root) != candidate.root {
			return "", fmt.Errorf(
				"Docker command %s root must be one normalized absolute path", candidate.name,
			)
		}
	}
	relative, inside := relativeWithin(runtimeRoot, commandRoot)
	if !inside {
		return "", fmt.Errorf("Docker command root is outside WORKSPACE_ROOT")
	}
	if err := validateV3CommandRoot(commandRoot); err != nil {
		return "", fmt.Errorf("Docker runtime workspace path is unavailable: %w", err)
	}
	executionRoot := filepath.Join(hostRoot, relative)
	if err := validateV3CommandRoot(executionRoot); err != nil {
		return "", fmt.Errorf("Docker host-identical workspace mirror is unavailable: %w", err)
	}
	runtimeInfo, err := os.Lstat(commandRoot)
	if err != nil {
		return "", fmt.Errorf("inspect Docker runtime workspace path: %w", err)
	}
	hostInfo, err := os.Lstat(executionRoot)
	if err != nil {
		return "", fmt.Errorf("inspect Docker host workspace path: %w", err)
	}
	if !os.SameFile(runtimeInfo, hostInfo) {
		return "", fmt.Errorf(
			"Docker host workspace path does not identify the same mounted directory as the runtime path",
		)
	}
	return executionRoot, nil
}

func resolveV3DockerHost(raw string) (string, string, error) {
	if raw != v3RootfulDockerHost {
		return "", "", fmt.Errorf(
			"Docker command execution requires DOCKER_HOST=%s; every other Docker authority is unsupported",
			v3RootfulDockerHost,
		)
	}
	return v3RootfulDockerHost, v3RootfulDockerSocketPath, nil
}

func validateV3DockerSocket(path string) error {
	if path != v3RootfulDockerSocketPath {
		return fmt.Errorf(
			"Docker command execution supports only the rootful Unix socket at %s",
			v3RootfulDockerSocketPath,
		)
	}
	return validateV3UnixSocket(path)
}

func validateV3UnixSocket(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("Docker Unix socket %s is unavailable: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("Docker command execution requires an exact non-symlink Unix socket at %s", path)
	}
	return nil
}
