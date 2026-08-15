package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func normalizeServiceName(value string) string {
	clean := strings.ToLower(strings.TrimSpace(value))
	if clean == "" {
		return defaultServiceName
	}
	return clean
}

func serviceTargetsAll(service string) bool {
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "", "*", "all", "stack":
		return true
	default:
		return false
	}
}

func runServiceInvocationOrExit(invocation []string, workdir string) {
	if len(invocation) == 0 {
		die("service invocation is empty")
	}

	cmd := exec.Command(invocation[0], invocation[1:]...)
	cmd.Dir = strings.TrimSpace(workdir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		die(fmt.Sprintf("service command failed: %v", err))
	}
}

func dockerLogsInvocationForService(opts serviceCommandOptions, composeCmd, dockerCmd []string, composeFile string, workdir string) ([]string, error) {
	serviceName := normalizeServiceName(opts.Service)
	if serviceTargetsAll(serviceName) {
		return nil, errors.New("docker-logs requires a specific service (example: --service core)")
	}

	containerID, err := resolveComposeServiceContainerID(composeCmd, composeFile, serviceName, workdir)
	if err != nil {
		return nil, err
	}

	return buildDockerLogsInvocation(dockerCmd, containerID, opts.Tail, opts.Follow)
}

func resolveComposeServiceContainerID(composeCmd []string, composeFile string, serviceName string, workdir string) (string, error) {
	if len(composeCmd) == 0 {
		return "", errors.New("compose command prefix is required")
	}
	composeFile = strings.TrimSpace(composeFile)
	if composeFile == "" {
		return "", errors.New("compose file is required")
	}
	serviceName = normalizeServiceName(serviceName)
	if serviceName == "" || serviceTargetsAll(serviceName) {
		return "", errors.New("service name is required for docker-logs")
	}

	invocation := append([]string{}, composeCmd...)
	invocation = append(invocation, "-f", composeFile, "ps", "-q", serviceName)
	cmd := exec.Command(invocation[0], invocation[1:]...)
	cmd.Dir = strings.TrimSpace(workdir)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = err.Error()
		}
		return "", fmt.Errorf("failed resolving container for service %q: %s", serviceName, reason)
	}

	containerID := firstNonEmptyLine(stdout.String())
	if containerID == "" {
		return "", fmt.Errorf("no running container found for service %q", serviceName)
	}
	return containerID, nil
}

func buildDockerLogsInvocation(dockerCmd []string, containerID string, tail int, follow bool) ([]string, error) {
	if len(dockerCmd) == 0 {
		return nil, errors.New("docker command prefix is required")
	}
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return nil, errors.New("container id is required for docker logs")
	}

	invocation := append([]string{}, dockerCmd...)
	invocation = append(invocation, "logs", "--tail", strconv.Itoa(maxInt(tail, 0)))
	if follow {
		invocation = append(invocation, "-f")
	}
	invocation = append(invocation, containerID)
	return invocation, nil
}

func firstNonEmptyLine(raw string) string {
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		clean := strings.TrimSpace(line)
		if clean != "" {
			return clean
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func expandHomePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if raw == "~" {
		return os.Getenv("HOME")
	}
	if strings.HasPrefix(raw, "~/") {
		return filepath.Join(os.Getenv("HOME"), strings.TrimPrefix(raw, "~/"))
	}
	return raw
}
