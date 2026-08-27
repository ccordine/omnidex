package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestParseServiceCommandArgsMigrateFreshAction(t *testing.T) {
	opts, showHelp, err := parseServiceCommandArgs([]string{"--service", "core", "migrate:fresh", "--yes"}, "")
	if err != nil {
		t.Fatalf("parseServiceCommandArgs returned error: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help flag")
	}
	if opts.Service != "core" || opts.Action != "migrate:fresh" || !opts.AssumeYes {
		t.Fatalf("unexpected parse result: %+v", opts)
	}
}

func TestParseServiceCommandArgsYesFlagInvalidWithoutMigrateFresh(t *testing.T) {
	_, _, err := parseServiceCommandArgs([]string{"--service", "core", "up", "--yes"}, "")
	if err == nil {
		t.Fatalf("expected --yes with non-migrate action to fail parsing")
	}
}

func TestServiceRunsCoreMigrateFresh(t *testing.T) {
	run, err := serviceRunsCoreMigrateFresh(serviceCommandOptions{
		Service: "core",
		Action:  "migrate:fresh",
	})
	if err != nil {
		t.Fatalf("serviceRunsCoreMigrateFresh returned error: %v", err)
	}
	if !run {
		t.Fatalf("expected core migrate:fresh action to run via CLI migrate flow")
	}
}

func TestServiceRunsCoreMigrateFreshRejectsNonCore(t *testing.T) {
	_, err := serviceRunsCoreMigrateFresh(serviceCommandOptions{
		Service: "postgres",
		Action:  "migrate:fresh",
	})
	if err == nil {
		t.Fatalf("expected non-core migrate:fresh action to fail")
	}
}

func TestResolveComposeCommandPrefixRequiresDockerPlugin(t *testing.T) {
	fakeBin := t.TempDir()
	fakeDocker := "#!/bin/sh\n[ \"$*\" = \"--context rootless compose version\" ]\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "docker"), []byte(fakeDocker), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)
	got, err := resolveComposeCommandPrefix("rootless", os.Environ(), execServiceProcessRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if value := strings.Join(got, " "); value != "docker --context rootless compose" {
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
	t.Setenv("PATH", fakeBin)
	_, err := resolveComposeCommandPrefix("rootless", os.Environ(), execServiceProcessRunner{})
	if err == nil || !strings.Contains(err.Error(), "Docker Compose plugin is unavailable in explicit context") {
		t.Fatalf("standalone Compose error = %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("standalone docker-compose was invoked: %v", statErr)
	}
}

func TestComposeInvocationForServiceCoreDownStopsSingleService(t *testing.T) {
	opts := serviceCommandOptions{
		Service: "core",
		Action:  "down",
	}
	args, err := composeInvocationForService(opts, []string{"docker", "--context", "rootless", "compose", "-p", "omni-nxt"}, "/tmp/docker-compose.yml")
	if err != nil {
		t.Fatalf("composeInvocationForService returned error: %v", err)
	}
	got := strings.Join(args, " ")
	want := "docker --context rootless compose -p omni-nxt -f /tmp/docker-compose.yml stop core"
	if got != want {
		t.Fatalf("composeInvocationForService=%q, want %q", got, want)
	}
}

func TestComposeInvocationForServiceAllDownUsesComposeDown(t *testing.T) {
	opts := serviceCommandOptions{
		Service: "all",
		Action:  "down",
	}
	args, err := composeInvocationForService(opts, []string{"docker", "--context", "rootless", "compose", "-p", "omni-nxt"}, "/tmp/docker-compose.yml")
	if err != nil {
		t.Fatalf("composeInvocationForService returned error: %v", err)
	}
	got := strings.Join(args, " ")
	want := "docker --context rootless compose -p omni-nxt -f /tmp/docker-compose.yml down --remove-orphans"
	if got != want {
		t.Fatalf("composeInvocationForService=%q, want %q", got, want)
	}
}

func TestComposeInvocationForServiceCoreBuildTargetsSingleService(t *testing.T) {
	opts := serviceCommandOptions{
		Service: "core",
		Action:  "build",
	}
	args, err := composeInvocationForService(opts, []string{"docker", "--context", "rootless", "compose", "-p", "omni-nxt"}, "/tmp/docker-compose.yml")
	if err != nil {
		t.Fatalf("composeInvocationForService returned error: %v", err)
	}
	got := strings.Join(args, " ")
	want := "docker --context rootless compose -p omni-nxt -f /tmp/docker-compose.yml build core"
	if got != want {
		t.Fatalf("composeInvocationForService=%q, want %q", got, want)
	}
}

func TestComposeInvocationForServiceAllBuildTargetsStack(t *testing.T) {
	opts := serviceCommandOptions{
		Service: "all",
		Action:  "build",
	}
	args, err := composeInvocationForService(opts, []string{"docker", "--context", "rootless", "compose", "-p", "omni-nxt"}, "/tmp/docker-compose.yml")
	if err != nil {
		t.Fatalf("composeInvocationForService returned error: %v", err)
	}
	got := strings.Join(args, " ")
	want := "docker --context rootless compose -p omni-nxt -f /tmp/docker-compose.yml build"
	if got != want {
		t.Fatalf("composeInvocationForService=%q, want %q", got, want)
	}
}

func TestDockerLogsInvocationForServiceRequiresSpecificService(t *testing.T) {
	_, err := dockerLogsInvocationForService(serviceCommandOptions{
		Service: "all",
		Action:  "docker-logs",
	}, []string{"docker", "--context", "rootless", "compose", "-p", "omni-nxt"}, []string{"docker", "--context", "rootless"}, "/tmp/docker-compose.yml", "/tmp", os.Environ(), execServiceProcessRunner{})
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
	args, err := buildDockerLogsInvocation([]string{"docker", "--context", "rootless"}, "abc123", 75, true)
	if err != nil {
		t.Fatalf("buildDockerLogsInvocation returned error: %v", err)
	}
	got := strings.Join(args, " ")
	if got != "docker --context rootless logs --tail 75 -f abc123" {
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
