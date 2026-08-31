package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const serviceComposeWaitTimeoutSeconds = 180

func normalizeServiceAction(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "up":
		return "up", true
	case "down":
		return "down", true
	case "restart":
		return "restart", true
	case "status", "ps":
		return "status", true
	case "logs", "log":
		return "logs", true
	case "docker-logs", "docker:logs", "dlogs":
		return "docker-logs", true
	case "start":
		return "start", true
	case "stop":
		return "stop", true
	case "build":
		return "build", true
	default:
		return "", false
	}
}

func resolveComposeCommandPrefix(
	contextName string,
	environment []string,
	runner serviceProcessRunner,
) ([]string, error) {
	if err := validateServiceRootfulDockerContext(contextName); err != nil {
		return nil, err
	}
	if runner == nil {
		return nil, errors.New("service process runner is required")
	}
	if err := requireServiceRootfulDockerDaemon(environment, runner); err != nil {
		return nil, err
	}
	invocation := []string{"docker", "--context", contextName, "compose", "version"}
	if _, err := runner.Output(serviceProcessRequest{
		Invocation: invocation, Environment: environment,
	}); err == nil {
		return []string{"docker", "--context", contextName, "compose"}, nil
	}
	return nil, fmt.Errorf("the Docker Compose plugin is unavailable in explicit context %q", contextName)
}

func requireServiceRootfulDockerDaemon(
	environment []string,
	runner serviceProcessRunner,
) error {
	endpoint, err := runner.Output(serviceProcessRequest{
		Invocation: []string{
			"docker", "context", "inspect", rootfulDockerContextName,
			"--format", `{{(index .Endpoints "docker").Host}}`,
		},
		Environment: environment,
	})
	if err != nil {
		return fmt.Errorf("qualify default rootful Docker context: %w", err)
	}
	if strings.TrimSpace(endpoint) != rootfulDockerSocketURL {
		return fmt.Errorf(
			"default Docker context must resolve to %s; rootless Docker is unsupported",
			rootfulDockerSocketURL,
		)
	}
	securityRaw, err := runner.Output(serviceProcessRequest{
		Invocation: []string{
			"docker", "--context", rootfulDockerContextName,
			"info", "--format", `{{json .SecurityOptions}}`,
		},
		Environment: environment,
	})
	if err != nil {
		return fmt.Errorf("qualify default rootful Docker daemon: %w", err)
	}
	var securityOptions []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(securityRaw)), &securityOptions); err != nil || securityOptions == nil {
		return fmt.Errorf("default Docker daemon returned invalid security authority")
	}
	for _, option := range securityOptions {
		if option == "name=rootless" || strings.HasPrefix(option, "name=rootless,") {
			return fmt.Errorf("default Docker daemon reported rootless execution authority")
		}
	}
	return nil
}

func resolveServiceComposeTarget(prefix, composeFile string) (string, string, error) {
	cleanPrefix := expandHomePath(strings.TrimSpace(prefix))
	if cleanPrefix != "" {
		if abs, err := filepath.Abs(cleanPrefix); err == nil {
			cleanPrefix = abs
		}
		cleanPrefix = filepath.Clean(cleanPrefix)
	}

	cleanComposeFile := expandHomePath(strings.TrimSpace(composeFile))
	if cleanComposeFile != "" {
		if !filepath.IsAbs(cleanComposeFile) {
			base := cleanPrefix
			if base == "" {
				base = currentWorkingDirectory()
			}
			cleanComposeFile = filepath.Join(base, cleanComposeFile)
		}
		abs, err := filepath.Abs(cleanComposeFile)
		if err != nil {
			return "", "", err
		}
		cleanComposeFile = filepath.Clean(abs)
		if !scriptFileExists(cleanComposeFile) {
			return "", "", fmt.Errorf("compose file not found: %s", cleanComposeFile)
		}
		return filepath.Dir(cleanComposeFile), cleanComposeFile, nil
	}

	searchRoots := []string{}
	if cleanPrefix != "" {
		searchRoots = append(searchRoots, cleanPrefix)
	} else {
		searchRoots = serviceRuntimeRootCandidates(
			strings.TrimSpace(os.Getenv(omniRuntimeDirEnv)),
			currentWorkingDirectory(),
			currentExecutablePath(),
		)
	}

	for _, root := range searchRoots {
		for _, name := range []string{"docker-compose.yml", "docker-compose.yaml"} {
			candidate := filepath.Join(root, name)
			if scriptFileExists(candidate) {
				return root, candidate, nil
			}
		}
	}

	if cleanPrefix != "" {
		return "", "", fmt.Errorf("no docker-compose.yml found under %s", cleanPrefix)
	}
	return "", "", errors.New("unable to locate docker-compose.yml; pass --prefix or --compose-file")
}

func serviceRuntimeRootCandidates(envRoot, cwd, executablePath string) []string {
	raw := []string{}
	if executablePath != "" {
		executableDirectory := filepath.Dir(executablePath)
		raw = append(raw, executableDirectory, filepath.Dir(executableDirectory))
	}
	raw = append(raw, envRoot, cwd)
	return dedupeAbsolutePaths(raw)
}

func composeInvocationForService(opts serviceCommandOptions, composeCmd []string, composeFile string) ([]string, error) {
	if len(composeCmd) == 0 {
		return nil, errors.New("compose command prefix is required")
	}
	composeFile = strings.TrimSpace(composeFile)
	if composeFile == "" {
		return nil, errors.New("compose file is required")
	}

	action, ok := normalizeServiceAction(opts.Action)
	if !ok {
		return nil, fmt.Errorf("unsupported service action: %s", opts.Action)
	}
	serviceName := normalizeServiceName(opts.Service)
	targetAll := serviceTargetsAll(serviceName)

	invocation := append([]string{}, composeCmd...)
	invocation = append(invocation, "-f", composeFile)
	switch action {
	case "up":
		invocation = append(
			invocation,
			"up", "-d", "--remove-orphans", "--wait", "--wait-timeout",
			strconv.Itoa(serviceComposeWaitTimeoutSeconds),
		)
		if opts.Build {
			invocation = append(invocation, "--build")
		}
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "down":
		if targetAll {
			invocation = append(invocation, "down", "--remove-orphans")
		} else {
			invocation = append(invocation, "stop", serviceName)
		}
	case "restart":
		invocation = append(invocation, "restart")
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "status":
		invocation = append(invocation, "ps")
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "logs":
		invocation = append(invocation, "logs", "--tail", strconv.Itoa(maxInt(opts.Tail, 0)))
		if opts.Follow {
			invocation = append(invocation, "-f")
		}
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "start":
		invocation = append(invocation, "start")
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "stop":
		invocation = append(invocation, "stop")
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "build":
		invocation = append(invocation, "build")
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	default:
		return nil, fmt.Errorf("unsupported service action: %s", action)
	}

	return invocation, nil
}
