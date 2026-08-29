package worker

import (
	"fmt"
	"os"
	"slices"
	"time"
)

const plainTextWorkspaceVerificationFamily = "plain_text"

func newPlainTextProjectEnvironment(
	runtime *nativeRuntimeV3,
	root string,
	spec directCodingDockerEnvironmentSpec,
) (*directCodingDockerProjectEnvironment, error) {
	if runtime == nil || runtime.svc == nil {
		return nil, fmt.Errorf("plain-text project environment requires one active runtime")
	}
	if err := validatePlainTextProjectEnvironmentSpec(spec); err != nil {
		return nil, err
	}
	hostRoot, err := resolveV3DockerCommandRoot(
		root, runtime.svc.workspaceRoot, runtime.svc.workspaceHostRoot,
	)
	if err != nil {
		return nil, err
	}
	uid, gid := os.Getuid(), os.Getgid()
	if uid <= 0 || gid <= 0 {
		return nil, fmt.Errorf("plain-text project environment requires one non-root runtime identity")
	}
	return newDirectCodingDockerProjectEnvironment(
		spec,
		directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: hostRoot},
		uint32(uid), uint32(gid), invokeDirectCodingDocker,
	)
}

func validatePlainTextProjectEnvironmentSpec(spec directCodingDockerEnvironmentSpec) error {
	if err := spec.validate(); err != nil {
		return err
	}
	if spec.ID != directCodingPlainTextEnvironmentID ||
		!spec.WorkspaceReadOnly ||
		!slices.Equal(spec.Programs, []string{"sha256sum", "test"}) {
		return fmt.Errorf("plain-text project environment has invalid bounded authority")
	}
	return nil
}

func plainTextWorkspaceVerificationCommand(
	artifactPath string,
	authority *directCodingDockerEnvironmentAuthority,
) testCommand {
	return testCommand{
		Family: plainTextWorkspaceVerificationFamily,
		Name:   "sha256sum", Args: []string{artifactPath},
		Purpose: verificationTest, Timeout: 30 * time.Second,
		WorkspaceRole: workspaceVerificationPrimary,
		Environment:   cloneDirectCodingDockerEnvironmentAuthority(authority),
	}
}

func validatePlainTextWorkspaceVerificationCommand(command testCommand) error {
	if command.Family != plainTextWorkspaceVerificationFamily ||
		command.Name != "sha256sum" || len(command.Args) != 1 ||
		command.Purpose != verificationTest ||
		command.RepositoryProof != nil ||
		command.Environment == nil ||
		command.WorkspaceRole != "" && command.WorkspaceRole != workspaceVerificationPrimary {
		return fmt.Errorf("plain-text workspace verification command has invalid authority")
	}
	artifactPath, err := normalizeDirectCodingPath(command.Args[0])
	if err != nil || artifactPath != command.Args[0] {
		return fmt.Errorf("plain-text workspace verification requires one normalized artifact path")
	}
	adapter, _, err := directCodingArtifactAdapterForPath(artifactPath)
	if err != nil || adapter.ID != "plain_text" {
		return fmt.Errorf("plain-text workspace verification path lacks the plain-text adapter")
	}
	if err := command.Environment.validate(); err != nil {
		return fmt.Errorf("plain-text workspace verification environment: %w", err)
	}
	spec := command.Environment.spec()
	if err := validatePlainTextProjectEnvironmentSpec(spec); err != nil {
		return err
	}
	return (directCodingProjectEnvironmentCommand{
		Program: command.Name,
		Args:    append([]string(nil), command.Args...),
		Timeout: command.Timeout,
	}).validate(spec)
}

func plainTextProjectEnvironmentCommand(command testCommand) (
	directCodingProjectEnvironmentCommand,
	error,
) {
	if err := validatePlainTextWorkspaceVerificationCommand(command); err != nil {
		return directCodingProjectEnvironmentCommand{}, err
	}
	return directCodingProjectEnvironmentCommand{
		Program: command.Name,
		Args:    append([]string(nil), command.Args...),
		Timeout: command.Timeout,
	}, nil
}
