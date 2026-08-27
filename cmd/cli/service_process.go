package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const serviceReleaseCommitEnvironmentKey = "OMNIDEX_COMMIT"

type serviceProcessRequest struct {
	Invocation  []string
	Workdir     string
	Environment []string
}

type serviceProcessRunner interface {
	Run(serviceProcessRequest) error
	Output(serviceProcessRequest) (string, error)
}

type execServiceProcessRunner struct{}

func (execServiceProcessRunner) Run(request serviceProcessRequest) error {
	command, err := newServiceExecCommand(request)
	if err != nil {
		return err
	}
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("service command failed: %w", err)
	}
	return nil
}

func (execServiceProcessRunner) Output(request serviceProcessRequest) (string, error) {
	command, err := newServiceExecCommand(request)
	if err != nil {
		return "", err
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = err.Error()
		}
		return "", fmt.Errorf("service command failed: %s", reason)
	}
	return stdout.String(), nil
}

func newServiceExecCommand(request serviceProcessRequest) (*exec.Cmd, error) {
	if len(request.Invocation) == 0 {
		return nil, errors.New("service invocation is empty")
	}
	command := exec.Command(request.Invocation[0], request.Invocation[1:]...)
	command.Dir = strings.TrimSpace(request.Workdir)
	command.Env = append([]string(nil), request.Environment...)
	return command, nil
}

func serviceChildEnvironment(base []string, commit string) ([]string, error) {
	if commit != "" && !serviceReleaseCommitPattern.MatchString(commit) {
		return nil, fmt.Errorf("service release commit must be an exact lowercase Git object identity")
	}
	environment := make([]string, 0, len(base)+1)
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if found && key == serviceReleaseCommitEnvironmentKey {
			continue
		}
		environment = append(environment, entry)
	}
	if commit != "" {
		environment = append(environment, serviceReleaseCommitEnvironmentKey+"="+commit)
	}
	return environment, nil
}

func serviceDeploymentChildEnvironment(
	base []string,
	commit string,
	composeProject string,
	hostUID string,
	hostGID string,
) ([]string, error) {
	if err := validateServiceDeploymentIdentifier(composeProjectEnvironmentKey, composeProject); err != nil {
		return nil, err
	}
	if err := validateServiceHostID(hostUIDEnvironmentKey, hostUID); err != nil {
		return nil, err
	}
	if err := validateServiceHostID(hostGIDEnvironmentKey, hostGID); err != nil {
		return nil, err
	}
	environment, err := serviceChildEnvironment(base, commit)
	if err != nil {
		return nil, err
	}
	bound := make([]string, 0, len(environment)+2)
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if found && (key == composeProjectEnvironmentKey || key == hostUIDEnvironmentKey || key == hostGIDEnvironmentKey) {
			continue
		}
		bound = append(bound, entry)
	}
	bound = append(
		bound,
		composeProjectEnvironmentKey+"="+composeProject,
		hostUIDEnvironmentKey+"="+hostUID,
		hostGIDEnvironmentKey+"="+hostGID,
	)
	return bound, nil
}

func exitServiceProcessError(err error) {
	if err == nil {
		return
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	die(err.Error())
}
