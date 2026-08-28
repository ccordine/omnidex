package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveV3DockerCommandExecutionUsesOnlyIdenticalMirrorAndUnixSocket(t *testing.T) {
	runtimeRoot := t.TempDir()
	hostRoot := runtimeRoot
	relative := filepath.Join("group", "fixture")
	runtimeCommandRoot := filepath.Join(runtimeRoot, relative)
	hostCommandRoot := filepath.Join(hostRoot, relative)
	for _, root := range []string{runtimeCommandRoot, hostCommandRoot} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	socketPath, closeSocket := openV3DockerTestSocket(t)
	defer closeSocket()
	t.Setenv("WORKSPACE_ROOT", runtimeRoot)
	t.Setenv("HOST_WORKSPACE_PATH", hostRoot)
	t.Setenv("DOCKER_HOST", "unix://"+socketPath)

	executionRoot, environment, err := resolveV3CommandExecution(
		context.Background(), runtimeCommandRoot, "docker",
	)
	if err != nil {
		t.Fatal(err)
	}
	if executionRoot != hostCommandRoot {
		t.Fatalf("Docker execution root=%q want host mirror %q", executionRoot, hostCommandRoot)
	}
	if len(environment) != 1 || environment[0] != "DOCKER_HOST=unix://"+socketPath {
		t.Fatalf("Docker execution environment=%v", environment)
	}
}

func TestResolveV3CommandExecutionLeavesNonDockerRootAndEnvironmentUntouched(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_ROOT", "")
	t.Setenv("HOST_WORKSPACE_PATH", "")
	t.Setenv("DOCKER_HOST", "")

	executionRoot, environment, err := resolveV3CommandExecution(context.Background(), root, "go")
	if err != nil {
		t.Fatal(err)
	}
	if executionRoot != root || len(environment) != 0 {
		t.Fatalf("non-Docker execution root/environment=%q/%v", executionRoot, environment)
	}
}

func TestResolveV3DockerCommandExecutionFailsWithoutEveryPhysicalAuthority(t *testing.T) {
	runtimeRoot := t.TempDir()
	hostRoot := runtimeRoot
	missingMirrorRoot := t.TempDir()
	commandRoot := filepath.Join(runtimeRoot, "fixture")
	if err := os.Mkdir(commandRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	socketPath, closeSocket := openV3DockerTestSocket(t)
	defer closeSocket()

	tests := []struct {
		name, runtime, host, dockerHost, want string
	}{
		{name: "runtime boundary", host: hostRoot, dockerHost: "unix://" + socketPath, want: "WORKSPACE_ROOT"},
		{name: "host boundary", runtime: runtimeRoot, dockerHost: "unix://" + socketPath, want: "HOST_WORKSPACE_PATH"},
		{name: "unnormalized runtime boundary", runtime: runtimeRoot + string(filepath.Separator), host: hostRoot, dockerHost: "unix://" + socketPath, want: "normalized absolute path"},
		{name: "host mirror", runtime: runtimeRoot, host: missingMirrorRoot, dockerHost: "unix://" + socketPath, want: "mirror is unavailable"},
		{name: "Docker host", runtime: runtimeRoot, host: hostRoot, want: "explicit Unix DOCKER_HOST"},
		{name: "whitespace Docker host", runtime: runtimeRoot, host: hostRoot, dockerHost: " unix://" + socketPath, want: "explicit Unix DOCKER_HOST"},
		{name: "TCP Docker host", runtime: runtimeRoot, host: hostRoot, dockerHost: "tcp://127.0.0.1:2375", want: "unix:///absolute/socket"},
		{name: "missing socket", runtime: runtimeRoot, host: hostRoot, dockerHost: "unix:///missing/docker.sock", want: "is unavailable"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("WORKSPACE_ROOT", testCase.runtime)
			t.Setenv("HOST_WORKSPACE_PATH", testCase.host)
			t.Setenv("DOCKER_HOST", testCase.dockerHost)
			_, _, err := resolveV3CommandExecution(context.Background(), commandRoot, "docker")
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("resolve error=%v want %q", err, testCase.want)
			}
		})
	}
}

func TestResolveV3DockerCommandExecutionRejectsDifferentMirrorDirectory(t *testing.T) {
	runtimeRoot := t.TempDir()
	hostRoot := t.TempDir()
	for _, root := range []string{runtimeRoot, hostRoot} {
		if err := os.Mkdir(filepath.Join(root, "fixture"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	socketPath, closeSocket := openV3DockerTestSocket(t)
	defer closeSocket()
	t.Setenv("WORKSPACE_ROOT", runtimeRoot)
	t.Setenv("HOST_WORKSPACE_PATH", hostRoot)
	t.Setenv("DOCKER_HOST", "unix://"+socketPath)

	_, _, err := resolveV3CommandExecution(
		context.Background(), filepath.Join(runtimeRoot, "fixture"), "docker",
	)
	if err == nil || !strings.Contains(err.Error(), "same mounted directory") {
		t.Fatalf("misconfigured Docker workspace mirror error=%v", err)
	}
}

func TestResolveV3DockerCommandExecutionAcceptsHealthyDefaultDaemon(t *testing.T) {
	root := t.TempDir()
	socketPath, closeSocket := openV3DockerTestSocket(t)
	defer closeSocket()
	t.Setenv("WORKSPACE_ROOT", root)
	t.Setenv("HOST_WORKSPACE_PATH", root)
	t.Setenv("DOCKER_HOST", "unix://"+socketPath)

	if _, _, err := resolveV3CommandExecution(context.Background(), root, "docker"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateV3DockerSocketRejectsRegularFileAndSymlink(t *testing.T) {
	root := t.TempDir()
	regular := filepath.Join(root, "regular")
	if err := os.WriteFile(regular, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	socketPath, closeSocket := openV3DockerTestSocket(t)
	defer closeSocket()
	alias := filepath.Join(root, "socket-link")
	if err := os.Symlink(socketPath, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{regular, alias} {
		if err := validateV3DockerSocket(path); err == nil {
			t.Fatalf("accepted non-exact Docker socket %s", path)
		}
	}
}

func TestDockerComposeConfigRunsFromTwoUnrelatedMappedFixtures(t *testing.T) {
	if os.Getenv("OMNIDEX_RUN_DOCKER_MIRROR_TEST") != "1" {
		t.Skip("set OMNIDEX_RUN_DOCKER_MIRROR_TEST=1 to run real Docker Compose config fixtures")
	}
	dockerHost := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_DOCKER_HOST"))
	if dockerHost == "" {
		t.Fatal("OMNIDEX_TEST_DOCKER_HOST must name one exact Unix socket")
	}
	runtimeWorkspace := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_RUNTIME_WORKSPACE_ROOT"))
	hostWorkspace := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_HOST_WORKSPACE_ROOT"))
	if runtimeWorkspace == "" || hostWorkspace == "" || runtimeWorkspace == hostWorkspace {
		t.Fatal("Docker mirror fixtures require distinct runtime and host workspace root paths")
	}
	runtimeInfo, err := os.Lstat(runtimeWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	hostInfo, err := os.Lstat(hostWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(runtimeInfo, hostInfo) {
		t.Fatal("Docker mirror fixture roots must identify the same mounted workspace directory")
	}
	fixtures := []struct {
		name, relative string
	}{
		{name: "bind source", relative: "bind_source"},
		{name: "declared config", relative: "declared_config"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			runtimeCommandRoot := filepath.Join(
				runtimeWorkspace, "internal", "worker", "testdata", "docker_mirror", fixture.relative,
			)
			relative, err := filepath.Rel(runtimeWorkspace, runtimeCommandRoot)
			if err != nil {
				t.Fatal(err)
			}
			hostCommandRoot := filepath.Join(hostWorkspace, relative)
			t.Setenv("WORKSPACE_ROOT", runtimeWorkspace)
			t.Setenv("HOST_WORKSPACE_PATH", hostWorkspace)
			t.Setenv("DOCKER_HOST", dockerHost)
			execution, err := runValidatedV3Command(context.Background(), runtimeCommandRoot, codeCommand{
				Program: "docker", Args: []string{"compose", "config", "--format", "json"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if execution.RunError != nil {
				t.Fatalf("Docker Compose config failed: %v\n%s", execution.RunError, execution.Stderr)
			}
			if !strings.Contains(execution.Stdout, hostCommandRoot) ||
				strings.Contains(execution.Stdout, runtimeCommandRoot) {
				t.Fatalf(
					"Docker Compose resolved paths outside host mirror %q: %s",
					hostCommandRoot, execution.Stdout,
				)
			}
		})
	}
}
