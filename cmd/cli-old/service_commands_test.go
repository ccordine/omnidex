package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type serviceOutputTestRunner map[string]string

func (serviceOutputTestRunner) Run(serviceProcessRequest) error { return nil }

func (runner serviceOutputTestRunner) Output(request serviceProcessRequest) (string, error) {
	command := strings.Join(request.Invocation, " ")
	output, found := runner[command]
	if !found {
		return "", fmt.Errorf("unexpected service command %q", command)
	}
	return output, nil
}

func TestParseServiceCommandArgsShortcutStyle(t *testing.T) {
	opts, showHelp, err := parseServiceCommandArgs([]string{"--service", "core", "up", "--build"}, "")
	if err != nil {
		t.Fatalf("parseServiceCommandArgs returned error: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help flag")
	}
	if opts.Service != "core" || opts.Action != "up" || !opts.Build {
		t.Fatalf("unexpected parse result: %+v", opts)
	}
}

func TestParseServiceCommandArgsPresetService(t *testing.T) {
	opts, showHelp, err := parseServiceCommandArgs([]string{"logs", "--tail", "25", "--follow"}, "core")
	if err != nil {
		t.Fatalf("parseServiceCommandArgs returned error: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help flag")
	}
	if opts.Service != "core" || opts.Action != "logs" || opts.Tail != 25 || !opts.Follow {
		t.Fatalf("unexpected parse result: %+v", opts)
	}
}

func TestParseServiceCommandArgsDockerLogsTwoTokenAction(t *testing.T) {
	opts, showHelp, err := parseServiceCommandArgs([]string{"--service", "core", "docker", "logs", "--tail", "10"}, "")
	if err != nil {
		t.Fatalf("parseServiceCommandArgs returned error: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help flag")
	}
	if opts.Service != "core" || opts.Action != "docker-logs" || opts.Tail != 10 {
		t.Fatalf("unexpected parse result: %+v", opts)
	}
}

func TestParseServiceCommandArgsCoreShorthand(t *testing.T) {
	opts, showHelp, err := parseServiceCommandArgs([]string{"--core", "up"}, "")
	if err != nil {
		t.Fatalf("parseServiceCommandArgs returned error: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help flag")
	}
	if opts.Service != "core" || opts.Action != "up" {
		t.Fatalf("unexpected parse result: %+v", opts)
	}
}

func TestParseServiceCommandArgsInvalidAction(t *testing.T) {
	_, _, err := parseServiceCommandArgs([]string{"deploy"}, "")
	if err == nil {
		t.Fatalf("expected invalid action to fail parsing")
	}
}

func TestParseServiceCommandArgsBuildAction(t *testing.T) {
	opts, showHelp, err := parseServiceCommandArgs([]string{"--service", "core", "build"}, "")
	if err != nil {
		t.Fatalf("parseServiceCommandArgs returned error: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help flag")
	}
	if opts.Service != "core" || opts.Action != "build" {
		t.Fatalf("unexpected parse result: %+v", opts)
	}
}

func TestResolveComposeCommandPrefixRequiresDockerPlugin(t *testing.T) {
	fakeBin := t.TempDir()
	fakeDocker := `#!/bin/sh
case "$*" in
  'context inspect default --format {{(index .Endpoints "docker").Host}}') printf '%s\n' 'unix:///var/run/docker.sock' ;;
  '--context default info --format {{json .SecurityOptions}}') printf '%s\n' '["name=seccomp,profile=builtin"]' ;;
  '--context default compose version') exit 0 ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(fakeBin, "docker"), []byte(fakeDocker), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)
	got, err := resolveComposeCommandPrefix("default", os.Environ(), execServiceProcessRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if value := strings.Join(got, " "); value != "docker --context default compose" {
		t.Fatalf("Compose command = %q", value)
	}
}

func TestResolveComposeCommandPrefixRejectsStandaloneFallback(t *testing.T) {
	fakeBin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "standalone-invoked")
	fakeStandalone := "#!/bin/sh\n: > \"" + marker + "\"\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "docker-compose"), []byte(fakeStandalone), 0o700); err != nil {
		t.Fatal(err)
	}
	fakeDocker := `#!/bin/sh
case "$*" in
  'context inspect default --format {{(index .Endpoints "docker").Host}}') printf '%s\n' 'unix:///var/run/docker.sock' ;;
  '--context default info --format {{json .SecurityOptions}}') printf '%s\n' '["name=seccomp,profile=builtin"]' ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(fakeBin, "docker"), []byte(fakeDocker), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)
	_, err := resolveComposeCommandPrefix("default", os.Environ(), execServiceProcessRunner{})
	if err == nil || !strings.Contains(err.Error(), "Docker Compose plugin is unavailable in explicit context") {
		t.Fatalf("standalone Compose error = %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("standalone docker-compose was invoked: %v", statErr)
	}
}

func TestResolveComposeCommandPrefixRejectsNonRootfulDockerAuthority(t *testing.T) {
	endpointCommand := `docker context inspect default --format {{(index .Endpoints "docker").Host}}`
	securityCommand := `docker --context default info --format {{json .SecurityOptions}}`
	composeCommand := `docker --context default compose version`
	for _, test := range []struct {
		name     string
		endpoint string
		security string
		want     string
	}{
		{name: "wrong socket", endpoint: "unix:///run/user/1000/docker.sock\n", security: `["name=seccomp"]`, want: rootfulDockerSocketURL},
		{name: "rootless daemon", endpoint: rootfulDockerSocketURL, security: `["name=rootless"]`, want: "rootless execution authority"},
		{name: "invalid security authority", endpoint: rootfulDockerSocketURL, security: `null`, want: "invalid security authority"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := serviceOutputTestRunner{
				endpointCommand: test.endpoint,
				securityCommand: test.security,
				composeCommand:  "",
			}
			_, err := resolveComposeCommandPrefix("default", nil, runner)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Docker authority error=%v want %q", err, test.want)
			}
		})
	}
}

func TestComposeInvocationForServiceCoreDownStopsSingleService(t *testing.T) {
	opts := serviceCommandOptions{
		Service: "core",
		Action:  "down",
	}
	args, err := composeInvocationForService(opts, []string{"docker", "--context", "default", "compose", "-p", "omni-nxt"}, "/tmp/docker-compose.yml")
	if err != nil {
		t.Fatalf("composeInvocationForService returned error: %v", err)
	}
	got := strings.Join(args, " ")
	want := "docker --context default compose -p omni-nxt -f /tmp/docker-compose.yml stop core"
	if got != want {
		t.Fatalf("composeInvocationForService=%q, want %q", got, want)
	}
}

func TestComposeInvocationForServiceAllDownUsesComposeDown(t *testing.T) {
	opts := serviceCommandOptions{
		Service: "all",
		Action:  "down",
	}
	args, err := composeInvocationForService(opts, []string{"docker", "--context", "default", "compose", "-p", "omni-nxt"}, "/tmp/docker-compose.yml")
	if err != nil {
		t.Fatalf("composeInvocationForService returned error: %v", err)
	}
	got := strings.Join(args, " ")
	want := "docker --context default compose -p omni-nxt -f /tmp/docker-compose.yml down --remove-orphans"
	if got != want {
		t.Fatalf("composeInvocationForService=%q, want %q", got, want)
	}
}

func TestComposeInvocationForServiceCoreBuildTargetsSingleService(t *testing.T) {
	opts := serviceCommandOptions{
		Service: "core",
		Action:  "build",
	}
	args, err := composeInvocationForService(opts, []string{"docker", "--context", "default", "compose", "-p", "omni-nxt"}, "/tmp/docker-compose.yml")
	if err != nil {
		t.Fatalf("composeInvocationForService returned error: %v", err)
	}
	got := strings.Join(args, " ")
	want := "docker --context default compose -p omni-nxt -f /tmp/docker-compose.yml build core"
	if got != want {
		t.Fatalf("composeInvocationForService=%q, want %q", got, want)
	}
}

func TestComposeInvocationForServiceAllBuildTargetsStack(t *testing.T) {
	opts := serviceCommandOptions{
		Service: "all",
		Action:  "build",
	}
	args, err := composeInvocationForService(opts, []string{"docker", "--context", "default", "compose", "-p", "omni-nxt"}, "/tmp/docker-compose.yml")
	if err != nil {
		t.Fatalf("composeInvocationForService returned error: %v", err)
	}
	got := strings.Join(args, " ")
	want := "docker --context default compose -p omni-nxt -f /tmp/docker-compose.yml build"
	if got != want {
		t.Fatalf("composeInvocationForService=%q, want %q", got, want)
	}
}

func TestDockerLogsInvocationForServiceRequiresSpecificService(t *testing.T) {
	_, err := dockerLogsInvocationForService(serviceCommandOptions{
		Service: "all",
		Action:  "docker-logs",
	}, []string{"docker", "--context", "default", "compose", "-p", "omni-nxt"}, []string{"docker", "--context", "default"}, "/tmp/docker-compose.yml", "/tmp", os.Environ(), execServiceProcessRunner{})
	if err == nil {
		t.Fatalf("expected docker-logs all-service invocation to fail")
	}
	if !strings.Contains(err.Error(), "specific service") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildDockerLogsInvocation(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available in PATH")
	}
	args, err := buildDockerLogsInvocation([]string{"docker", "--context", "default"}, "abc123", 75, true)
	if err != nil {
		t.Fatalf("buildDockerLogsInvocation returned error: %v", err)
	}
	got := strings.Join(args, " ")
	if got != "docker --context default logs --tail 75 -f abc123" {
		t.Fatalf("buildDockerLogsInvocation=%q, expected docker logs args", got)
	}
}

func TestFirstNonEmptyLine(t *testing.T) {
	got := firstNonEmptyLine("\n \ncontainer-id-1\ncontainer-id-2\n")
	if got != "container-id-1" {
		t.Fatalf("firstNonEmptyLine()=%q, want %q", got, "container-id-1")
	}
}

func TestResolveServiceComposeTargetFromPrefix(t *testing.T) {
	root := t.TempDir()
	composePath := filepath.Join(root, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	gotRoot, gotCompose, err := resolveServiceComposeTarget(root, "")
	if err != nil {
		t.Fatalf("resolveServiceComposeTarget returned error: %v", err)
	}
	if gotRoot != root {
		t.Fatalf("resolveServiceComposeTarget root=%q, want %q", gotRoot, root)
	}
	if gotCompose != composePath {
		t.Fatalf("resolveServiceComposeTarget compose=%q, want %q", gotCompose, composePath)
	}
}
