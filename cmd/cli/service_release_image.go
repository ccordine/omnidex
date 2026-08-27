package main

import (
	"fmt"
	"regexp"
)

const serviceOCIRevisionLabel = "org.opencontainers.image.revision"
const serviceCoreBinaryPath = "/usr/local/bin/agent-core"

var (
	serviceImageIDPattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	serviceContainerIDPattern = regexp.MustCompile(`^[0-9a-f]{12,64}$`)
)

func verifyBuiltServiceRelease(
	runner serviceProcessRunner,
	composeCmd []string,
	dockerCmd []string,
	composeFile string,
	workdir string,
	environment []string,
	expectedCommit string,
	expectedUser string,
) (string, error) {
	imageID, err := resolveServiceImageID(
		runner, composeCmd, composeFile, defaultServiceName, workdir, environment,
	)
	if err != nil {
		return "", err
	}
	if err := verifyServiceImageRevision(
		runner, dockerCmd, imageID, workdir, environment, expectedCommit,
	); err != nil {
		return "", err
	}
	if err := verifyServiceRuntimeUser(
		runner, dockerCmd, "image", imageID, workdir, environment, expectedUser,
	); err != nil {
		return "", err
	}
	return imageID, nil
}

func verifyRunningServiceRelease(
	runner serviceProcessRunner,
	composeCmd []string,
	dockerCmd []string,
	composeFile string,
	workdir string,
	environment []string,
	expectedCommit string,
	expectedUser string,
) error {
	expectedImage, err := verifyBuiltServiceRelease(
		runner, composeCmd, dockerCmd, composeFile, workdir, environment, expectedCommit, expectedUser,
	)
	if err != nil {
		return err
	}
	containerID, err := resolveRunningServiceContainerID(
		runner, composeCmd, composeFile, defaultServiceName, workdir, environment,
	)
	if err != nil {
		return err
	}
	invocation := append([]string{}, dockerCmd...)
	invocation = append(invocation, "inspect", "--type", "container", "--format", "{{.Image}}", containerID)
	runningImageOutput, err := runner.Output(serviceProcessRequest{
		Invocation: invocation, Workdir: workdir, Environment: environment,
	})
	if err != nil {
		return fmt.Errorf("inspect running core container image: %w", err)
	}
	runningImage, err := exactServiceOutputLine(runningImageOutput)
	if err != nil || !serviceImageIDPattern.MatchString(runningImage) {
		return fmt.Errorf("running core container did not resolve to one exact immutable image identity")
	}
	if runningImage != expectedImage {
		return fmt.Errorf("running core image %s does not equal configured core image %s", runningImage, expectedImage)
	}
	revisionInvocation := append([]string{}, dockerCmd...)
	revisionInvocation = append(
		revisionInvocation,
		"inspect", "--type", "container", "--format",
		`{{ index .Config.Labels "org.opencontainers.image.revision" }}`,
		containerID,
	)
	revisionOutput, err := runner.Output(serviceProcessRequest{
		Invocation: revisionInvocation, Workdir: workdir, Environment: environment,
	})
	if err != nil {
		return fmt.Errorf("inspect running core container revision: %w", err)
	}
	runningRevision, err := exactServiceOutputLine(revisionOutput)
	if err != nil || !serviceReleaseCommitPattern.MatchString(runningRevision) {
		return fmt.Errorf("running core container has no exact %s label", serviceOCIRevisionLabel)
	}
	if runningRevision != expectedCommit {
		return fmt.Errorf(
			"running core container revision %s does not equal target release commit %s",
			runningRevision,
			expectedCommit,
		)
	}
	if err := verifyServiceRuntimeUser(
		runner, dockerCmd, "container", containerID, workdir, environment, expectedUser,
	); err != nil {
		return err
	}
	healthInvocation := append([]string{}, dockerCmd...)
	healthInvocation = append(
		healthInvocation,
		"exec", containerID, serviceCoreBinaryPath,
		"release:verify-running-health", expectedCommit,
	)
	healthOutput, err := runner.Output(serviceProcessRequest{
		Invocation: healthInvocation, Workdir: workdir, Environment: environment,
	})
	if err != nil {
		return fmt.Errorf("verify exact running core health release: %w", err)
	}
	healthCommit, err := exactServiceOutputLine(healthOutput)
	if err != nil || !serviceReleaseCommitPattern.MatchString(healthCommit) {
		return fmt.Errorf("exact running core health did not return one release commit")
	}
	if healthCommit != expectedCommit {
		return fmt.Errorf(
			"exact running core health commit %s does not equal target release commit %s",
			healthCommit,
			expectedCommit,
		)
	}
	return nil
}

func verifyServiceRuntimeUser(
	runner serviceProcessRunner,
	dockerCmd []string,
	objectType string,
	objectID string,
	workdir string,
	environment []string,
	expectedUser string,
) error {
	if runner == nil || len(dockerCmd) == 0 {
		return fmt.Errorf("service runtime user verification requires a runner and Docker command")
	}
	if err := validateServiceRuntimeUser(expectedUser); err != nil {
		return err
	}
	invocation := append([]string{}, dockerCmd...)
	if objectType == "image" {
		invocation = append(invocation, "image", "inspect", "--format", "{{.Config.User}}", objectID)
	} else if objectType == "container" {
		invocation = append(invocation, "inspect", "--type", "container", "--format", "{{.Config.User}}", objectID)
	} else {
		return fmt.Errorf("unsupported service runtime user object type %q", objectType)
	}
	output, err := runner.Output(serviceProcessRequest{
		Invocation: invocation, Workdir: workdir, Environment: environment,
	})
	if err != nil {
		return fmt.Errorf("inspect core %s runtime user: %w", objectType, err)
	}
	runtimeUser, err := exactServiceOutputLine(output)
	if err != nil || validateServiceRuntimeUser(runtimeUser) != nil {
		return fmt.Errorf("core %s has no exact positive numeric runtime UID:GID", objectType)
	}
	if runtimeUser != expectedUser {
		return fmt.Errorf("core %s runtime user %s does not equal managed host identity %s", objectType, runtimeUser, expectedUser)
	}
	return nil
}

func resolveServiceImageID(
	runner serviceProcessRunner,
	composeCmd []string,
	composeFile string,
	serviceName string,
	workdir string,
	environment []string,
) (string, error) {
	if runner == nil || len(composeCmd) == 0 {
		return "", fmt.Errorf("service image resolution requires a runner and Compose command")
	}
	invocation := append([]string{}, composeCmd...)
	invocation = append(invocation, "-f", composeFile, "images", "-q", serviceName)
	output, err := runner.Output(serviceProcessRequest{
		Invocation: invocation, Workdir: workdir, Environment: environment,
	})
	if err != nil {
		return "", fmt.Errorf("resolve built image for service %q: %w", serviceName, err)
	}
	imageID, err := exactServiceOutputLine(output)
	if err != nil || !serviceImageIDPattern.MatchString(imageID) {
		return "", fmt.Errorf("Compose service %q did not resolve to one exact immutable image identity", serviceName)
	}
	return imageID, nil
}

func verifyServiceImageRevision(
	runner serviceProcessRunner,
	dockerCmd []string,
	imageID string,
	workdir string,
	environment []string,
	expectedCommit string,
) error {
	if runner == nil || len(dockerCmd) == 0 {
		return fmt.Errorf("service image verification requires a runner and Docker command")
	}
	invocation := append([]string{}, dockerCmd...)
	invocation = append(
		invocation,
		"image", "inspect", "--format",
		`{{ index .Config.Labels "org.opencontainers.image.revision" }}`,
		imageID,
	)
	output, err := runner.Output(serviceProcessRequest{
		Invocation: invocation, Workdir: workdir, Environment: environment,
	})
	if err != nil {
		return fmt.Errorf("inspect built core image revision: %w", err)
	}
	revision, err := exactServiceOutputLine(output)
	if err != nil || !serviceReleaseCommitPattern.MatchString(revision) {
		return fmt.Errorf("built core image has no exact %s label", serviceOCIRevisionLabel)
	}
	if revision != expectedCommit {
		return fmt.Errorf("built core image revision %s does not equal target release commit %s", revision, expectedCommit)
	}
	return nil
}

func resolveRunningServiceContainerID(
	runner serviceProcessRunner,
	composeCmd []string,
	composeFile string,
	serviceName string,
	workdir string,
	environment []string,
) (string, error) {
	invocation := append([]string{}, composeCmd...)
	invocation = append(invocation, "-f", composeFile, "ps", "-q", serviceName)
	output, err := runner.Output(serviceProcessRequest{
		Invocation: invocation, Workdir: workdir, Environment: environment,
	})
	if err != nil {
		return "", fmt.Errorf("resolve running container for service %q: %w", serviceName, err)
	}
	containerID, err := exactServiceOutputLine(output)
	if err != nil || !serviceContainerIDPattern.MatchString(containerID) {
		return "", fmt.Errorf("Compose service %q did not resolve to one exact running container", serviceName)
	}
	return containerID, nil
}
