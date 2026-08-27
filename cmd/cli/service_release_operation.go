package main

import (
	"fmt"
	"strings"
)

func serviceOperationRequiresReleaseIdentity(opts serviceCommandOptions) bool {
	action := strings.ToLower(strings.TrimSpace(opts.Action))
	if action == "build" || action == "up" {
		return true
	}
	if action != "start" && action != "restart" {
		return false
	}
	serviceName := normalizeServiceName(opts.Service)
	return serviceName == defaultServiceName || serviceTargetsAll(serviceName)
}

func validateServiceReleaseTarget(opts serviceCommandOptions) error {
	if !serviceOperationRequiresReleaseIdentity(opts) {
		return nil
	}
	serviceName := normalizeServiceName(opts.Service)
	if serviceName == defaultServiceName || serviceTargetsAll(serviceName) {
		return nil
	}
	return fmt.Errorf(
		"service %s release identity requires --service core or --all (got %q)",
		strings.ToLower(strings.TrimSpace(opts.Action)),
		serviceName,
	)
}

func executeServiceOperation(
	runner serviceProcessRunner,
	opts serviceCommandOptions,
	invocation []string,
	composeCmd []string,
	dockerCmd []string,
	composeFile string,
	workdir string,
	environment []string,
	releaseCommit string,
	expectedUser string,
) error {
	if runner == nil {
		return fmt.Errorf("service process runner is required")
	}
	if err := validateServiceReleaseTarget(opts); err != nil {
		return err
	}
	if err := runner.Run(serviceProcessRequest{
		Invocation: invocation, Workdir: workdir, Environment: environment,
	}); err != nil {
		return err
	}

	action := strings.ToLower(strings.TrimSpace(opts.Action))
	if !serviceOperationRequiresReleaseIdentity(opts) {
		return nil
	}
	if !serviceReleaseCommitPattern.MatchString(releaseCommit) {
		return fmt.Errorf("service release operation requires an exact target commit")
	}
	switch action {
	case "build":
		_, err := verifyBuiltServiceRelease(
			runner, composeCmd, dockerCmd, composeFile, workdir, environment, releaseCommit, expectedUser,
		)
		return err
	case "up", "start", "restart":
		if err := verifyRunningServiceRelease(
			runner, composeCmd, dockerCmd, composeFile, workdir, environment, releaseCommit, expectedUser,
		); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported release service action %q", action)
	}
}
