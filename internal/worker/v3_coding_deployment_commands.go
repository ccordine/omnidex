package worker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/operation"
)

type directCodingDeploymentCommandKind string

const (
	directCodingDeploymentConfig   directCodingDeploymentCommandKind = "config"
	directCodingDeploymentBuild    directCodingDeploymentCommandKind = "build"
	directCodingDeploymentStart    directCodingDeploymentCommandKind = "start"
	directCodingDeploymentObserve  directCodingDeploymentCommandKind = "observe"
	directCodingDeploymentMigrate  directCodingDeploymentCommandKind = "migrate"
	directCodingDeploymentWrite    directCodingDeploymentCommandKind = "state_write"
	directCodingDeploymentRead     directCodingDeploymentCommandKind = "state_read"
	directCodingDeploymentRestart  directCodingDeploymentCommandKind = "restart"
	directCodingDeploymentRollback directCodingDeploymentCommandKind = "rollback"
)

func directCodingDeploymentCommand(
	kind directCodingDeploymentCommandKind,
	project string,
	descriptor directCodingDeploymentDescriptor,
	environment map[string]string,
) (codeCommand, error) {
	var command []string
	timeout := defaultV3CommandLimit
	switch kind {
	case directCodingDeploymentConfig:
		command = []string{"config", "--hash=*"}
	case directCodingDeploymentBuild:
		if err := descriptor.validate(); err != nil {
			return codeCommand{}, err
		}
		command = append([]string{"build"}, descriptor.BaseServices...)
		timeout = maxV3CommandLimit
	case directCodingDeploymentStart:
		command = []string{"up", "--detach", "--wait", "--remove-orphans"}
		timeout = maxV3CommandLimit
	case directCodingDeploymentObserve:
		command = []string{"ps", "--all", "--orphans", "--format", "json"}
	case directCodingDeploymentRestart:
		command = []string{"restart", "app"}
	case directCodingDeploymentMigrate:
		if descriptor.MigrationScript == "" {
			return codeCommand{}, fmt.Errorf("deployment stack has no registered migration operation")
		}
		command = []string{"run", "--rm", "--no-deps", "app", "php", descriptor.MigrationScript}
	case directCodingDeploymentWrite, directCodingDeploymentRead:
		mode := map[directCodingDeploymentCommandKind]string{
			directCodingDeploymentWrite: "write",
			directCodingDeploymentRead:  "read",
		}[kind]
		command = []string{
			"run", "--rm", "--no-deps", "app", "php", phpServiceStateVerificationPath, mode,
		}
	case directCodingDeploymentRollback:
		command = []string{"down", "--volumes", "--remove-orphans"}
		timeout = maxV3CommandLimit
	default:
		return codeCommand{}, fmt.Errorf("deployment command kind %q is unsupported", kind)
	}
	args, err := directCodingDeploymentComposeArgs(project, command...)
	if err != nil {
		return codeCommand{}, err
	}
	return codeCommand{
		Program: "docker", Args: args, Timeout: timeout,
		Profile: codeCommandProfileDeployment, Environment: cloneDirectCodingDeploymentEnvironment(environment),
	}, nil
}

func cloneDirectCodingDeploymentEnvironment(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for name, value := range values {
		cloned[name] = value
	}
	return cloned
}

func (s *directCodingSession) executeDirectCodingDeploymentCommand(
	root string,
	kind directCodingDeploymentCommandKind,
	profileID string,
	project string,
	descriptor directCodingDeploymentDescriptor,
	environment map[string]string,
) (operation.Result, error) {
	command, err := directCodingDeploymentCommand(kind, project, descriptor, environment)
	if err != nil {
		return operation.Result{}, err
	}
	result, err := executeDirectCodingDeploymentAfterRuntimeQualification(
		profileID,
		directCodingSessionVersionProbe(s.runtime.ctx, root),
		func() (operation.Result, error) {
			return executeCodeCommandAtRoot(s.runtime.ctx, root, command)
		},
	)
	if err != nil {
		return operation.Result{}, fmt.Errorf("execute deployment %s: %w", kind, err)
	}
	return validateDirectCodingDeploymentCommandResult(kind, result, environment)
}

func (s *directCodingSession) executeProtectedDirectCodingDeploymentCommand(
	root string,
	kind directCodingDeploymentCommandKind,
	profileID string,
	project string,
	descriptor directCodingDeploymentDescriptor,
	environment map[string]string,
	namespaceGate func() error,
) (operation.Result, error) {
	if kind != directCodingDeploymentBuild && kind != directCodingDeploymentStart {
		return operation.Result{}, fmt.Errorf("deployment command %s has no protected namespace authority", kind)
	}
	if namespaceGate == nil {
		return operation.Result{}, fmt.Errorf("protected deployment command requires an exact namespace gate")
	}
	command, err := directCodingDeploymentCommand(kind, project, descriptor, environment)
	if err != nil {
		return operation.Result{}, err
	}
	result, err := executeDirectCodingDeploymentAfterRuntimeQualificationAndGate(
		profileID, directCodingSessionVersionProbe(s.runtime.ctx, root), namespaceGate,
		func() (operation.Result, error) {
			return executeCodeCommandAtRoot(s.runtime.ctx, root, command)
		},
	)
	if err != nil {
		return operation.Result{}, fmt.Errorf("execute protected deployment %s: %w", kind, err)
	}
	return validateDirectCodingDeploymentCommandResult(kind, result, environment)
}

func validateDirectCodingDeploymentCommandResult(
	kind directCodingDeploymentCommandKind,
	result operation.Result,
	environment map[string]string,
) (operation.Result, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return operation.Result{}, fmt.Errorf("encode deployment command result: %w", err)
	}
	if err := validateDirectCodingDeploymentEnvironmentAbsentFromText(
		string(raw), environment,
	); err != nil {
		return operation.Result{}, err
	}
	if len(result.Evidence) != 1 {
		return operation.Result{}, fmt.Errorf("deployment %s produced %d evidence rows, expected one", kind, len(result.Evidence))
	}
	return result, nil
}

func (s *directCodingSession) qualifyDirectCodingDeploymentRuntime(
	root, profileID string,
) error {
	if s == nil || s.runtime == nil || s.runtime.ctx == nil {
		return fmt.Errorf("deployment runtime qualification requires one active session")
	}
	return validateDirectCodingDeploymentRuntimeAuthority(
		profileID, directCodingSessionVersionProbe(s.runtime.ctx, root),
	)
}

func executeDirectCodingDeploymentAfterRuntimeQualification(
	profileID string,
	probe directCodingVersionProbe,
	execute func() (operation.Result, error),
) (operation.Result, error) {
	if execute == nil {
		return operation.Result{}, fmt.Errorf("deployment command executor is required")
	}
	if err := validateDirectCodingDeploymentRuntimeAuthority(profileID, probe); err != nil {
		return operation.Result{}, err
	}
	return execute()
}

func executeDirectCodingDeploymentAfterRuntimeQualificationAndGate(
	profileID string,
	probe directCodingVersionProbe,
	gate func() error,
	execute func() (operation.Result, error),
) (operation.Result, error) {
	if gate == nil || execute == nil {
		return operation.Result{}, fmt.Errorf("protected deployment gate and executor are required")
	}
	if err := validateDirectCodingDeploymentRuntimeAuthority(profileID, probe); err != nil {
		return operation.Result{}, err
	}
	if err := gate(); err != nil {
		return operation.Result{}, err
	}
	return execute()
}

func validateDirectCodingDeploymentRuntimeAuthority(
	profileID string,
	probe directCodingVersionProbe,
) error {
	profile, err := directCodingProjectVersionProfileByID(profileID)
	if err != nil {
		return fmt.Errorf("deployment runtime profile is unavailable: %w", err)
	}
	stack, err := directCodingProjectStackByID(profile.StackID)
	if err != nil || stack.Deployment == nil {
		return fmt.Errorf("deployment runtime profile %s lacks a registered deployment stack", profileID)
	}
	engine, err := directCodingVersionComponent(profile, "docker_engine")
	if err != nil {
		return fmt.Errorf("deployment runtime profile lacks Docker Engine authority: %w", err)
	}
	if err := validateRuntimeCommandConstraint(
		probe, "docker", []string{"version", "--format", "{{.Server.Version}}"}, engine,
	); err != nil {
		return fmt.Errorf("deployment Docker Engine authority drifted: %w", err)
	}
	compose, err := directCodingVersionComponent(profile, "docker_compose")
	if err != nil {
		return fmt.Errorf("deployment runtime profile lacks Docker Compose authority: %w", err)
	}
	if err := validateRuntimeCommandConstraint(
		probe, "docker", []string{"compose", "version", "--short"}, compose,
	); err != nil {
		return fmt.Errorf("deployment Docker Compose authority drifted: %w", err)
	}
	return nil
}

func directCodingDeploymentCommandSucceeded(
	kind directCodingDeploymentCommandKind,
	result operation.Result,
) error {
	if directCodingCommandSucceeded(result) {
		return nil
	}
	return fmt.Errorf("deployment %s failed: %s", kind, directCodingCommandResult(result))
}

func directCodingDeploymentCommandTimeout(kind directCodingDeploymentCommandKind) time.Duration {
	if kind == directCodingDeploymentBuild || kind == directCodingDeploymentStart ||
		kind == directCodingDeploymentRollback {
		return maxV3CommandLimit
	}
	return defaultV3CommandLimit
}
