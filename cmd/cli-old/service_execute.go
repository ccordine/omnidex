package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/version"
)

func executeResolvedServiceCommand(opts serviceCommandOptions) error {
	root, composeFile, err := resolveServiceComposeTarget(opts.Prefix, opts.ComposeFile)
	if err != nil {
		return err
	}
	runner := execServiceProcessRunner{}
	environment, err := serviceChildEnvironment(os.Environ(), "")
	if err != nil {
		return err
	}
	releaseCommit, environment, err := prepareServiceReleaseEnvironment(
		opts, root, environment, runner,
	)
	if err != nil {
		return err
	}
	identity, err := readServiceDeploymentIdentity(root)
	if err != nil {
		return err
	}
	environment, err = serviceDeploymentChildEnvironment(
		environment, releaseCommit, identity.ComposeProject, identity.HostUID, identity.HostGID,
	)
	if err != nil {
		return err
	}
	fmt.Fprintf(
		os.Stderr,
		"[service] root=%s docker_context=%s compose_project=%s\n",
		root, identity.DockerContext, identity.ComposeProject,
	)
	composeCmd, err := resolveComposeCommandPrefix(identity.DockerContext, environment, runner)
	if err != nil {
		return err
	}
	composeCmd = append(composeCmd, "-p", identity.ComposeProject)
	if strings.EqualFold(strings.TrimSpace(opts.Action), "docker-logs") {
		invocation, err := dockerLogsInvocationForService(
			opts, composeCmd, identity.dockerCommandPrefix(), composeFile, root, environment, runner,
		)
		if err != nil {
			return err
		}
		return runner.Run(serviceProcessRequest{
			Invocation: invocation, Workdir: root, Environment: environment,
		})
	}
	invocation, err := composeInvocationForService(opts, composeCmd, composeFile)
	if err != nil {
		return err
	}
	if err := executeServiceOperation(
		runner, opts, invocation,
		composeCmd, identity.dockerCommandPrefix(), composeFile, root,
		environment, releaseCommit, identity.runtimeUser(),
	); err != nil {
		return err
	}
	if releaseCommit != "" {
		fmt.Fprintf(os.Stderr, "[service] verified release_commit=%s\n", releaseCommit)
	}
	return nil
}

func prepareServiceReleaseEnvironment(
	opts serviceCommandOptions,
	root string,
	cleanEnvironment []string,
	runner serviceProcessRunner,
) (string, []string, error) {
	if !serviceOperationRequiresReleaseIdentity(opts) {
		return "", cleanEnvironment, nil
	}
	if err := validateServiceReleaseTarget(opts); err != nil {
		return "", nil, err
	}
	embeddedCommit, err := version.BuildCommit()
	if err != nil {
		return "", nil, fmt.Errorf("resolve embedded service release commit: %w", err)
	}
	checkout, err := resolveServiceReleaseCheckout(root, embeddedCommit, cleanEnvironment, runner)
	if err != nil {
		return "", nil, err
	}
	environment, err := serviceChildEnvironment(cleanEnvironment, checkout.Commit)
	if err != nil {
		return "", nil, err
	}
	return checkout.Commit, environment, nil
}
