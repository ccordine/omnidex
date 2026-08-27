package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type directCodingSandboxExecutionAuthority struct {
	root         string
	environment  []string
	dockerSocket string
}

func (sandbox *directCodingWorkspaceSandbox) invocation(
	ctx context.Context,
	command testCommand,
) ([]string, []*os.File, *os.File, error) {
	if sandbox == nil || sandbox.root == "" {
		return nil, nil, nil, fmt.Errorf("direct-coding verification sandbox is incomplete")
	}
	if err := exactRepositorySandboxExecutable(repositoryBubblewrapPath, "bubblewrap"); err != nil {
		return nil, nil, nil, err
	}
	execution, err := sandbox.executionAuthority(ctx, command)
	if err != nil {
		return nil, nil, nil, err
	}
	rootHandle, err := openRepositorySandboxDirectory(
		sandbox.root, "direct-coding writable verification root",
	)
	if err != nil {
		return nil, nil, nil, err
	}
	handles := []*os.File{rootHandle}
	closeHandles := func() {
		for _, handle := range handles {
			_ = handle.Close()
		}
	}
	sourceHandle, err := openRepositorySandboxDirectory(
		sandbox.projection.source.Root, "direct-coding projection source",
	)
	if err != nil {
		closeHandles()
		return nil, nil, nil, err
	}
	handles = append(handles, sourceHandle)
	deltaFD := -1
	if sandbox.projection.deltaRoot != "" {
		deltaHandle, err := openRepositorySandboxDirectory(
			sandbox.projection.deltaRoot, "direct-coding projection delta",
		)
		if err != nil {
			closeHandles()
			return nil, nil, nil, err
		}
		deltaFD = 3 + len(handles)
		handles = append(handles, deltaHandle)
	}
	infoReader, infoWriter, err := os.Pipe()
	if err != nil {
		closeHandles()
		return nil, nil, nil, fmt.Errorf("create direct-coding sandbox status pipe: %w", err)
	}
	infoFD := 3 + len(handles)
	handles = append(handles, infoWriter)
	fail := func(err error) ([]string, []*os.File, *os.File, error) {
		_ = infoReader.Close()
		closeHandles()
		return nil, nil, nil, err
	}
	arguments := []string{
		"--unshare-all", "--share-net", "--unshare-user",
		"--disable-userns", "--assert-userns-disabled",
		"--die-with-parent", "--new-session", "--cap-drop", "ALL",
		"--hostname", "omnidex-verification", "--clearenv",
	}
	for _, system := range []string{"/usr", "/bin", "/lib", "/lib64", "/sbin"} {
		arguments = append(arguments, "--dir", system, "--ro-bind-try", system, system)
	}
	arguments = append(arguments,
		"--dir", "/etc", "--ro-bind", "/etc", "/etc",
		"--dir", "/var",
		"--proc", "/proc", "--dev", "/dev",
		"--tmpfs", "/tmp", "--tmpfs", "/home", "--dir", "/home/omnidex",
	)
	arguments = append(arguments, directCodingSandboxDirectoryArguments(execution.root)...)
	arguments = append(arguments, "--bind-fd", "3", execution.root)
	if execution.dockerSocket != "" {
		arguments = append(
			arguments,
			directCodingSandboxDirectoryArguments(filepath.Dir(execution.dockerSocket))...,
		)
		arguments = append(
			arguments, "--bind", execution.dockerSocket, execution.dockerSocket,
		)
	}
	sourceFD := 4
	for _, mount := range sandbox.mounts {
		if mount.Source == repositoryWorkspaceProjectionSymlink {
			continue
		}
		fd := sourceFD
		if mount.Source == repositoryWorkspaceProjectionDelta {
			if deltaFD < 3 {
				return fail(fmt.Errorf("direct-coding delta mount %q has no descriptor", mount.Path))
			}
			fd = deltaFD
		}
		source := repositorySandboxDescriptorPath(fd, mount.Path)
		destination := filepath.Join(execution.root, filepath.FromSlash(mount.Path))
		arguments = append(arguments, "--ro-bind", source, destination)
	}
	environment, err := v3CommandEnvironment(sandbox.root)
	if err != nil {
		return fail(err)
	}
	for _, item := range environment {
		name, value, found := strings.Cut(item, "=")
		if !found || name == "HOME" {
			continue
		}
		arguments = append(arguments, "--setenv", name, value)
	}
	for _, item := range execution.environment {
		name, value, found := strings.Cut(item, "=")
		if !found {
			return fail(fmt.Errorf("direct-coding command environment is invalid"))
		}
		arguments = append(arguments, "--setenv", name, value)
	}
	arguments = append(arguments,
		"--setenv", "HOME", "/home/omnidex",
		"--setenv", "TMPDIR", "/tmp",
		"--setenv", "COMPOSE_PROJECT_NAME", sandbox.namespace,
		"--setenv", "COMPOSE_DISABLE_ENV_FILE", "1",
		"--chdir", execution.root,
		"--info-fd", fmt.Sprint(infoFD),
		"--", command.Name,
	)
	arguments = append(arguments, command.Args...)
	if err := validateDirectCodingSandboxInvocation(arguments); err != nil {
		return fail(err)
	}
	return arguments, handles, infoReader, nil
}

func (sandbox *directCodingWorkspaceSandbox) executionAuthority(
	ctx context.Context,
	command testCommand,
) (directCodingSandboxExecutionAuthority, error) {
	authority := directCodingSandboxExecutionAuthority{root: repositorySandboxRoot}
	if command.Name != "docker" {
		return authority, nil
	}
	root, environment, err := resolveV3CommandExecution(
		ctx, sandbox.projection.source.Root, command.Name,
	)
	if err != nil {
		return directCodingSandboxExecutionAuthority{}, err
	}
	if err := validateDirectCodingDockerSandboxRoot(root); err != nil {
		return directCodingSandboxExecutionAuthority{}, err
	}
	_, socket, err := resolveV3DockerHost(os.Getenv("DOCKER_HOST"))
	if err != nil {
		return directCodingSandboxExecutionAuthority{}, err
	}
	authority.root = root
	authority.environment = append([]string(nil), environment...)
	authority.dockerSocket = socket
	return authority, nil
}

func validateDirectCodingDockerSandboxRoot(root string) error {
	if root == "" || root == string(filepath.Separator) || !filepath.IsAbs(root) ||
		root != filepath.Clean(root) || strings.ContainsAny(root, "\x00\r\n") {
		return fmt.Errorf("direct-coding Docker sandbox requires one exact host-identical workspace root")
	}
	for _, protected := range []string{
		"/bin", "/dev", "/etc", "/gomodcache", "/lib", "/lib64", "/proc",
		"/sbin", "/toolchain", "/usr", "/var", "/home/omnidex",
	} {
		if root == protected || strings.HasPrefix(root, protected+string(filepath.Separator)) {
			return fmt.Errorf(
				"direct-coding Docker sandbox root %q overlaps protected runtime authority",
				root,
			)
		}
	}
	return nil
}

func directCodingSandboxDirectoryArguments(absolute string) []string {
	clean := filepath.Clean(absolute)
	if clean == string(filepath.Separator) {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator))
	arguments := make([]string, 0, len(parts)*2)
	current := string(filepath.Separator)
	for _, part := range parts {
		current = filepath.Join(current, part)
		arguments = append(arguments, "--dir", current)
	}
	return arguments
}

func validateDirectCodingSandboxInvocation(arguments []string) error {
	if err := validateRepositoryWorkspaceProjectionArguments(arguments); err != nil {
		return err
	}
	for _, argument := range arguments {
		if strings.ContainsRune(argument, '\x00') || strings.ContainsAny(argument, "\r\n") {
			return fmt.Errorf("direct-coding verification sandbox argument is invalid")
		}
	}
	return nil
}
