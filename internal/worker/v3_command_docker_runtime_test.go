package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveV3DockerCommandRootUsesOnlyIdenticalMirror(t *testing.T) {
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
	executionRoot, err := resolveV3DockerCommandRoot(
		runtimeCommandRoot, runtimeRoot, hostRoot,
	)
	if err != nil {
		t.Fatal(err)
	}
	if executionRoot != hostCommandRoot {
		t.Fatalf("Docker execution root=%q want host mirror %q", executionRoot, hostCommandRoot)
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

func TestResolveV3DockerCommandRootFailsWithoutEveryPhysicalAuthority(t *testing.T) {
	runtimeRoot := t.TempDir()
	hostRoot := runtimeRoot
	missingMirrorRoot := t.TempDir()
	commandRoot := filepath.Join(runtimeRoot, "fixture")
	if err := os.Mkdir(commandRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, runtime, host, want string
	}{
		{name: "runtime boundary", host: hostRoot, want: "WORKSPACE_ROOT"},
		{name: "host boundary", runtime: runtimeRoot, want: "HOST_WORKSPACE_PATH"},
		{name: "unnormalized runtime boundary", runtime: runtimeRoot + string(filepath.Separator), host: hostRoot, want: "normalized absolute path"},
		{name: "host mirror", runtime: runtimeRoot, host: missingMirrorRoot, want: "mirror is unavailable"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := resolveV3DockerCommandRoot(commandRoot, testCase.runtime, testCase.host)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("resolve error=%v want %q", err, testCase.want)
			}
		})
	}
}

func TestResolveV3DockerCommandRootRejectsDifferentMirrorDirectory(t *testing.T) {
	runtimeRoot := t.TempDir()
	hostRoot := t.TempDir()
	for _, root := range []string{runtimeRoot, hostRoot} {
		if err := os.Mkdir(filepath.Join(root, "fixture"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	_, err := resolveV3DockerCommandRoot(
		filepath.Join(runtimeRoot, "fixture"), runtimeRoot, hostRoot,
	)
	if err == nil || !strings.Contains(err.Error(), "same mounted directory") {
		t.Fatalf("misconfigured Docker workspace mirror error=%v", err)
	}
}

func TestResolveV3DockerHostAcceptsOnlyRootfulSystemSocket(t *testing.T) {
	host, socket, err := resolveV3DockerHost(v3RootfulDockerHost)
	if err != nil {
		t.Fatal(err)
	}
	if host != v3RootfulDockerHost || socket != v3RootfulDockerSocketPath {
		t.Fatalf("Docker authority=%q/%q", host, socket)
	}
	for _, candidate := range []string{
		"", " " + v3RootfulDockerHost, v3RootfulDockerHost + " ",
		"unix:///run/user/1000/docker.sock", "unix:///missing/docker.sock",
		"tcp://127.0.0.1:2375",
	} {
		if _, _, err := resolveV3DockerHost(candidate); err == nil ||
			!strings.Contains(err.Error(), v3RootfulDockerHost) {
			t.Fatalf("Docker host %q error=%v", candidate, err)
		}
	}
}

func TestV3DockerCLIArgumentsPinRootfulSystemSocket(t *testing.T) {
	input := []string{"compose", "config", "--quiet"}
	got := v3DockerCLIArguments(input)
	want := []string{"--host", v3RootfulDockerHost, "compose", "config", "--quiet"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Docker CLI arguments=%q want=%q", got, want)
	}
	got[2] = "changed"
	if input[0] != "compose" {
		t.Fatalf("Docker CLI argument projection mutated its input: %q", input)
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
		if err := validateV3UnixSocket(path); err == nil {
			t.Fatalf("accepted non-exact Docker socket %s", path)
		}
	}
}

func TestValidateV3DockerSocketRejectsAlternateUnixSocket(t *testing.T) {
	socketPath, closeSocket := openV3DockerTestSocket(t)
	defer closeSocket()
	if err := validateV3DockerSocket(socketPath); err == nil ||
		!strings.Contains(err.Error(), v3RootfulDockerSocketPath) {
		t.Fatalf("alternate Docker socket error=%v", err)
	}
}

func TestDockerComposeConfigRunsFromTwoUnrelatedMappedFixtures(t *testing.T) {
	if os.Getenv("OMNIDEX_RUN_DOCKER_MIRROR_TEST") != "1" {
		t.Skip("set OMNIDEX_RUN_DOCKER_MIRROR_TEST=1 to run real Docker Compose config fixtures")
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
			t.Setenv("DOCKER_HOST", v3RootfulDockerHost)
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
