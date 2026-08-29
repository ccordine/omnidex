package worker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	directCodingProjectEnvironmentBuildTimeout = 5 * time.Minute
)

type directCodingDockerInvoke func(
	context.Context,
	[]string,
	[]byte,
) (v3CommandExecution, error)

// directCodingDockerProjectEnvironment is the only project-environment
// implementation. The injected function is only the Docker CLI transport and
// cannot select another executable.
type directCodingDockerProjectEnvironment struct {
	mu           sync.Mutex
	spec         directCodingDockerEnvironmentSpec
	workspace    directCodingDockerWorkspaceMapping
	uid          uint32
	gid          uint32
	invokeDocker directCodingDockerInvoke
	buildContext []byte
	imageID      string
	built        bool
	closed       bool
}

func newDirectCodingDockerProjectEnvironment(
	spec directCodingDockerEnvironmentSpec,
	workspace directCodingDockerWorkspaceMapping,
	uid, gid uint32,
	invokeDocker directCodingDockerInvoke,
) (*directCodingDockerProjectEnvironment, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	if err := workspace.validate(); err != nil {
		return nil, err
	}
	if uid == 0 || gid == 0 {
		return nil, fmt.Errorf("project Docker environment requires one non-root uid:gid")
	}
	if invokeDocker == nil {
		return nil, fmt.Errorf("project Docker environment requires the Docker CLI transport")
	}
	buildContext, err := directCodingDockerBuildContext(spec.Dockerfile)
	if err != nil {
		return nil, err
	}
	spec.Programs = append([]string(nil), spec.Programs...)
	return &directCodingDockerProjectEnvironment{
		spec: spec, workspace: workspace, uid: uid, gid: gid,
		invokeDocker: invokeDocker, buildContext: buildContext,
	}, nil
}

func (environment *directCodingDockerProjectEnvironment) ImageID() string {
	if environment == nil {
		return ""
	}
	environment.mu.Lock()
	defer environment.mu.Unlock()
	return environment.imageID
}

func (environment *directCodingDockerProjectEnvironment) Authority() (
	*directCodingDockerEnvironmentAuthority,
	error,
) {
	if environment == nil {
		return nil, fmt.Errorf("project Docker environment is nil")
	}
	environment.mu.Lock()
	defer environment.mu.Unlock()
	if !environment.built || environment.imageID == "" {
		return nil, fmt.Errorf("project Docker environment %s has not been built", environment.spec.ID)
	}
	return newDirectCodingDockerEnvironmentAuthority(environment.spec, environment.imageID)
}

func (environment *directCodingDockerProjectEnvironment) RequireAuthority(
	authority *directCodingDockerEnvironmentAuthority,
) error {
	if environment == nil {
		return fmt.Errorf("project Docker environment is nil")
	}
	if err := authority.validate(); err != nil {
		return err
	}
	environment.mu.Lock()
	defer environment.mu.Unlock()
	if !environment.built || environment.imageID == "" {
		return fmt.Errorf("project Docker environment %s has not been built", environment.spec.ID)
	}
	current, err := newDirectCodingDockerEnvironmentAuthority(environment.spec, environment.imageID)
	if err != nil {
		return err
	}
	if current.AuthoritySHA256 != authority.AuthoritySHA256 ||
		current.ImageID != authority.ImageID {
		return fmt.Errorf("rebuilt project Docker environment differs from persisted authority")
	}
	return nil
}

func (environment *directCodingDockerProjectEnvironment) Build(parent context.Context) error {
	if environment == nil {
		return fmt.Errorf("project Docker environment is nil")
	}
	environment.mu.Lock()
	defer environment.mu.Unlock()
	if environment.closed {
		return fmt.Errorf("project Docker environment %s is closed", environment.spec.ID)
	}
	ctx, cancel, err := boundedDirectCodingDockerContext(
		parent, directCodingProjectEnvironmentBuildTimeout,
	)
	if err != nil {
		return err
	}
	defer cancel()
	result, err := environment.invokeDocker(ctx, []string{
		"build", "--quiet", "--file", "Dockerfile", "-",
	}, environment.buildContext)
	if err := validateDirectCodingDockerResult("build project development image", result, err); err != nil {
		return err
	}
	imageID, err := directCodingDockerBuildImageID(result)
	if err != nil {
		return err
	}
	if environment.imageID != "" && environment.imageID != imageID {
		return fmt.Errorf(
			"repeated project development image build changed immutable ID from %s to %s",
			environment.imageID, imageID,
		)
	}
	environment.imageID = imageID
	environment.built = true
	return nil
}

func directCodingDockerBuildImageID(result v3CommandExecution) (string, error) {
	if result.StdoutTruncated {
		return "", fmt.Errorf("project development image ID output was truncated")
	}
	imageID := result.Stdout
	if strings.HasSuffix(imageID, "\n") {
		imageID = strings.TrimSuffix(imageID, "\n")
	}
	if !directCodingEnvironmentImageIDPattern.MatchString(imageID) {
		return "", fmt.Errorf("project development image build returned invalid immutable image ID %q", result.Stdout)
	}
	return imageID, nil
}

func (environment *directCodingDockerProjectEnvironment) Run(
	parent context.Context,
	command directCodingProjectEnvironmentCommand,
) (v3CommandExecution, error) {
	var zero v3CommandExecution
	if environment == nil {
		return zero, fmt.Errorf("project Docker environment is nil")
	}
	environment.mu.Lock()
	defer environment.mu.Unlock()
	if environment.closed {
		return zero, fmt.Errorf("project Docker environment %s is closed", environment.spec.ID)
	}
	if !environment.built {
		return zero, fmt.Errorf("project Docker environment %s has not been built", environment.spec.ID)
	}
	if err := command.validate(environment.spec); err != nil {
		return zero, err
	}
	ctx, cancel, err := boundedDirectCodingDockerContext(parent, command.Timeout)
	if err != nil {
		return zero, err
	}
	defer cancel()
	args := []string{
		"run", "--rm",
		"--read-only",
		"--security-opt", "no-new-privileges=true",
		"--cap-drop", "ALL",
		"--network", "none",
		"--user", strconv.FormatUint(uint64(environment.uid), 10) + ":" + strconv.FormatUint(uint64(environment.gid), 10),
		"--mount", "type=bind,src=" + environment.workspace.HostRoot + ",dst=" + directCodingProjectEnvironmentWorkdir + ",readonly",
		"--tmpfs", "/tmp:rw,nosuid,nodev,noexec",
		"--env", "HOME=/tmp",
		"--workdir", directCodingProjectEnvironmentWorkdir,
		environment.imageID,
		command.Program,
	}
	args = append(args, command.Args...)
	result, invokeErr := environment.invokeDocker(ctx, args, nil)
	if err := validateDirectCodingDockerResult("run project development command", result, invokeErr); err != nil {
		return result, err
	}
	return result, nil
}

func (environment *directCodingDockerProjectEnvironment) Close(parent context.Context) error {
	if environment == nil {
		return nil
	}
	environment.mu.Lock()
	defer environment.mu.Unlock()
	if environment.closed {
		return nil
	}
	if parent == nil {
		return fmt.Errorf("project Docker environment requires a context")
	}
	environment.closed = true
	return nil
}

func boundedDirectCodingDockerContext(
	parent context.Context,
	limit time.Duration,
) (context.Context, context.CancelFunc, error) {
	if parent == nil {
		return nil, nil, fmt.Errorf("project Docker environment requires a context")
	}
	ctx, cancel := context.WithTimeout(parent, limit)
	return ctx, cancel, nil
}

func directCodingDockerBuildContext(dockerfile string) ([]byte, error) {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	header := &tar.Header{
		Name: "Dockerfile", Mode: 0o444, Size: int64(len(dockerfile)),
		Uid: 0, Gid: 0, ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR,
	}
	if err := writer.WriteHeader(header); err != nil {
		return nil, fmt.Errorf("write project Dockerfile header: %w", err)
	}
	if _, err := writer.Write([]byte(dockerfile)); err != nil {
		return nil, fmt.Errorf("write project Dockerfile: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close project Docker build context: %w", err)
	}
	return buffer.Bytes(), nil
}

func validateDirectCodingDockerResult(
	action string,
	result v3CommandExecution,
	err error,
) error {
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	if result.ContextError != nil {
		return fmt.Errorf("%s: %w", action, result.ContextError)
	}
	if result.RunError != nil || result.ExitCode != 0 {
		return fmt.Errorf(
			"%s exited %d: %v: %s", action, result.ExitCode, result.RunError,
			trimForBudget(renderV3CommandOutput(result), 1000),
		)
	}
	return nil
}

func invokeDirectCodingDocker(
	ctx context.Context,
	args []string,
	stdin []byte,
) (v3CommandExecution, error) {
	var zero v3CommandExecution
	if ctx == nil {
		return zero, fmt.Errorf("Docker CLI invocation requires a context")
	}
	_, _, err := resolveV3DockerHost(os.Getenv("DOCKER_HOST"))
	if err != nil {
		return zero, err
	}
	if err := requireV3RootfulDockerAuthority(ctx); err != nil {
		return zero, err
	}
	stdout, err := newBoundedCommandOutput(maxV3CommandOutput)
	if err != nil {
		return zero, err
	}
	stderr, err := newBoundedCommandOutput(maxV3CommandOutput)
	if err != nil {
		return zero, err
	}
	started := time.Now()
	process := exec.CommandContext(ctx, "docker", v3DockerCLIArguments(args)...)
	process.Env = directCodingDockerProcessEnvironment(os.Environ())
	if stdin != nil {
		process.Stdin = bytes.NewReader(stdin)
	}
	process.Stdout = stdout
	process.Stderr = stderr
	runErr := process.Run()
	result := v3CommandExecution{Duration: time.Since(started), RunError: runErr, ContextError: ctx.Err()}
	result.Stdout, result.StdoutBytes, result.StdoutTruncated = stdout.Result()
	result.Stderr, result.StderrBytes, result.StderrTruncated = stderr.Result()
	if runErr != nil {
		result.ExitCode = -1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
	}
	return result, nil
}

func directCodingDockerProcessEnvironment(current []string) []string {
	environment := make([]string, 0, len(current))
	for _, value := range current {
		name, _, found := strings.Cut(value, "=")
		if found && directCodingDockerRoutingEnvironment(name) {
			continue
		}
		environment = append(environment, value)
	}
	return environment
}

func directCodingDockerRoutingEnvironment(name string) bool {
	switch name {
	case "DOCKER_CONTEXT", "DOCKER_HOST", "DOCKER_CONFIG",
		"DOCKER_CERT_PATH", "DOCKER_TLS", "DOCKER_TLS_VERIFY",
		"BUILDKIT_HOST", "BUILDKIT_TLS_SERVER_NAME", "BUILDKIT_TLS_CACERT",
		"BUILDKIT_TLS_CERT", "BUILDKIT_TLS_KEY",
		"BUILDX_BUILDER", "BUILDX_CONFIG":
		return true
	default:
		return false
	}
}
