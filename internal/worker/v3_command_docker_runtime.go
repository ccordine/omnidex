package worker

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	dockerHost, socketPath, err := resolveV3DockerHost(os.Getenv("DOCKER_HOST"))
	if err != nil {
		return "", nil, err
	}
	if err := validateV3DockerSocket(socketPath); err != nil {
		return "", nil, err
	}
	if err := validateV3DockerDaemon(ctx, socketPath); err != nil {
		return "", nil, err
	}
	return executionRoot, []string{"DOCKER_HOST=" + dockerHost}, nil
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
	if raw == "" || strings.TrimSpace(raw) != raw {
		return "", "", fmt.Errorf("Docker command execution requires an explicit Unix DOCKER_HOST")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "unix" || parsed.Host != "" || parsed.Opaque != "" ||
		parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("Docker command execution requires DOCKER_HOST=unix:///absolute/socket")
	}
	socketPath := parsed.Path
	if socketPath == "" || !filepath.IsAbs(socketPath) || filepath.Clean(socketPath) != socketPath ||
		strings.ContainsAny(socketPath, "\x00\r\n") || raw != "unix://"+socketPath {
		return "", "", fmt.Errorf("Docker command execution requires DOCKER_HOST=unix:///absolute/socket")
	}
	return raw, socketPath, nil
}

func validateV3DockerSocket(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("Docker Unix socket %s is unavailable: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("Docker command execution requires an exact non-symlink Unix socket at %s", path)
	}
	return nil
}
