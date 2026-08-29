package main

import (
	"strings"
	"testing"
)

type releaseIdentityTestRunner struct {
	requests          []serviceProcessRequest
	runCount          int
	imageID           string
	imageRevision     string
	imageUser         string
	containerID       string
	runningImageID    string
	containerRevision string
	containerUser     string
	healthCommit      string
}

func (runner *releaseIdentityTestRunner) Run(request serviceProcessRequest) error {
	runner.requests = append(runner.requests, request)
	runner.runCount++
	return nil
}

func (runner *releaseIdentityTestRunner) Output(request serviceProcessRequest) (string, error) {
	runner.requests = append(runner.requests, request)
	command := strings.Join(request.Invocation, " ")
	switch {
	case strings.Contains(command, " images -q core"):
		return runner.imageID + "\n", nil
	case strings.Contains(command, " image inspect --format {{.Config.User}} "):
		return runner.imageUser + "\n", nil
	case strings.Contains(command, " image inspect --format "):
		return runner.imageRevision + "\n", nil
	case strings.Contains(command, " ps -q core"):
		return runner.containerID + "\n", nil
	case strings.Contains(command, " inspect --type container --format {{ index "):
		return runner.containerRevision + "\n", nil
	case strings.Contains(command, " inspect --type container --format {{.Config.User}} "):
		return runner.containerUser + "\n", nil
	case strings.Contains(command, " inspect --type container --format "):
		return runner.runningImageID + "\n", nil
	case strings.Contains(command, " exec ") && strings.Contains(command, " release:verify-running-health "):
		return runner.healthCommit + "\n", nil
	default:
		return "", nil
	}
}

func TestExecuteServiceUpVerifiesOneExactReleaseChain(t *testing.T) {
	commit := strings.Repeat("a", 40)
	imageID := "sha256:" + strings.Repeat("b", 64)
	expectedUser := "1000:1001"
	runner := &releaseIdentityTestRunner{
		imageID:           imageID,
		imageRevision:     commit,
		imageUser:         expectedUser,
		containerID:       strings.Repeat("c", 64),
		runningImageID:    imageID,
		containerRevision: commit,
		containerUser:     expectedUser,
		healthCommit:      commit,
	}
	environment, err := serviceDeploymentChildEnvironment(
		[]string{
			"PATH=/bin",
			"OMNIDEX_COMMIT=ambient",
			"HOST_UID=9000",
			"HOST_GID=9001",
			"HOST_UID=9002",
			"COMPOSE_PROJECT_NAME=ambient-project",
		},
		commit, "omnidex", "1000", "1001",
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := serviceCommandOptions{Service: "core", Action: "up"}
	composeCmd := []string{"docker", "--context", "default", "compose", "-p", "omnidex"}
	invocation, err := composeInvocationForService(opts, composeCmd, "/release/docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	if err := executeServiceOperation(
		runner, opts, invocation,
		composeCmd, []string{"docker", "--context", "default"},
		"/release/docker-compose.yml", "/release", environment,
		commit, expectedUser,
	); err != nil {
		t.Fatal(err)
	}
	if runner.runCount != 1 {
		t.Fatalf("Compose up run count = %d", runner.runCount)
	}
	if command := strings.Join(runner.requests[0].Invocation, " "); !strings.Contains(command, " up -d --remove-orphans --wait --wait-timeout 180 core") {
		t.Fatalf("Compose up invocation lacks bounded wait semantics: %s", command)
	}
	for _, request := range runner.requests {
		values := serviceEnvironmentValues(request.Environment, serviceReleaseCommitEnvironmentKey)
		if len(values) != 1 || values[0] != commit {
			t.Fatalf("child environment release commit values = %v", values)
		}
		if values := serviceEnvironmentValues(request.Environment, hostUIDEnvironmentKey); len(values) != 1 || values[0] != "1000" {
			t.Fatalf("child environment host UID values = %v", values)
		}
		if values := serviceEnvironmentValues(request.Environment, hostGIDEnvironmentKey); len(values) != 1 || values[0] != "1001" {
			t.Fatalf("child environment host GID values = %v", values)
		}
		if values := serviceEnvironmentValues(request.Environment, composeProjectEnvironmentKey); len(values) != 1 || values[0] != "omnidex" {
			t.Fatalf("child environment Compose project values = %v", values)
		}
	}
	healthCommand := "docker --context default exec " + runner.containerID + " " +
		serviceCoreBinaryPath + " release:verify-running-health " + commit
	if !serviceTestRequestsContain(runner.requests, healthCommand) {
		t.Fatalf("exact-container running health command was not invoked: %v", runner.requests)
	}
}

func TestExecuteServiceStartAndRestartRejectStaleRunningRelease(t *testing.T) {
	expected := strings.Repeat("a", 40)
	stale := strings.Repeat("f", 40)
	imageID := "sha256:" + strings.Repeat("b", 64)
	expectedUser := "1000:1001"
	for _, test := range []struct {
		action  string
		service string
	}{
		{action: "start", service: "core"},
		{action: "restart", service: "all"},
	} {
		t.Run(test.action+"_"+test.service, func(t *testing.T) {
			runner := &releaseIdentityTestRunner{
				imageID:           imageID,
				imageRevision:     expected,
				imageUser:         expectedUser,
				containerID:       strings.Repeat("c", 64),
				runningImageID:    imageID,
				containerRevision: expected,
				containerUser:     expectedUser,
				healthCommit:      stale,
			}
			opts := serviceCommandOptions{Service: test.service, Action: test.action}
			err := executeServiceOperation(
				runner, opts, []string{"docker", "compose", test.action},
				[]string{"docker", "compose"}, []string{"docker"},
				"compose.yml", "/release", []string{"PATH=/bin"}, expected, expectedUser,
			)
			if err == nil || !strings.Contains(err.Error(), "does not equal target release commit") {
				t.Fatalf("stale %s release error = %v", test.action, err)
			}
			if runner.runCount != 1 {
				t.Fatalf("%s run count = %d", test.action, runner.runCount)
			}
			if !serviceTestRequestsContain(
				runner.requests,
				"docker exec "+runner.containerID+" "+serviceCoreBinaryPath+
					" release:verify-running-health "+expected,
			) {
				t.Fatalf("%s did not verify exact-container health: %v", test.action, runner.requests)
			}
		})
	}
}

func TestExecuteServiceBuildVerifiesBuiltImageRevision(t *testing.T) {
	commit := strings.Repeat("d", 64)
	expectedUser := "1000:1001"
	runner := &releaseIdentityTestRunner{
		imageID:       "sha256:" + strings.Repeat("e", 64),
		imageRevision: commit,
		imageUser:     expectedUser,
	}
	opts := serviceCommandOptions{Service: "all", Action: "build"}
	environment, err := serviceDeploymentChildEnvironment(
		[]string{"PATH=/bin", "COMPOSE_PROJECT_NAME=ambient", "HOST_UID=9000", "HOST_GID=9001"},
		commit, "omnidex", "1000", "1001",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := executeServiceOperation(
		runner, opts, []string{"docker", "compose", "build"},
		[]string{"docker", "compose"}, []string{"docker"}, "compose.yml", "/release",
		environment, commit, expectedUser,
	); err != nil {
		t.Fatal(err)
	}
	if runner.runCount != 1 {
		t.Fatalf("Compose build run count = %d", runner.runCount)
	}
}

func TestVerifyBuiltServiceReleaseRejectsStaleOCIRevision(t *testing.T) {
	expected := strings.Repeat("a", 40)
	stale := strings.Repeat("b", 40)
	runner := &releaseIdentityTestRunner{
		imageID:       "sha256:" + strings.Repeat("c", 64),
		imageRevision: stale,
	}
	_, err := verifyBuiltServiceRelease(
		runner, []string{"docker", "compose"}, []string{"docker"},
		"compose.yml", "/release", []string{"PATH=/bin"}, expected,
		"1000:1001",
	)
	if err == nil || !strings.Contains(err.Error(), "does not equal target release commit") {
		t.Fatalf("stale OCI revision error = %v", err)
	}
}

func TestVerifyBuiltServiceReleaseRejectsStaleRuntimeUser(t *testing.T) {
	expected := strings.Repeat("a", 40)
	runner := &releaseIdentityTestRunner{
		imageID:       "sha256:" + strings.Repeat("c", 64),
		imageRevision: expected,
		imageUser:     "1002:1003",
	}
	_, err := verifyBuiltServiceRelease(
		runner, []string{"docker", "compose"}, []string{"docker"},
		"compose.yml", "/release", []string{"PATH=/bin"}, expected, "1000:1001",
	)
	if err == nil || !strings.Contains(err.Error(), "does not equal managed host identity") {
		t.Fatalf("stale image runtime user error = %v", err)
	}
}

func TestVerifyRunningServiceReleaseRejectsStaleExactContainerHealth(t *testing.T) {
	expected := strings.Repeat("a", 40)
	stale := strings.Repeat("f", 40)
	imageID := "sha256:" + strings.Repeat("b", 64)
	expectedUser := "1000:1001"
	runner := &releaseIdentityTestRunner{
		imageID:           imageID,
		imageRevision:     expected,
		imageUser:         expectedUser,
		containerID:       strings.Repeat("c", 64),
		runningImageID:    imageID,
		containerRevision: expected,
		containerUser:     expectedUser,
		healthCommit:      stale,
	}
	err := verifyRunningServiceRelease(
		runner, []string{"docker", "compose"}, []string{"docker"},
		"compose.yml", "/release", []string{"PATH=/bin"}, expected, expectedUser,
	)
	if err == nil || !strings.Contains(err.Error(), "does not equal target release commit") {
		t.Fatalf("stale exact-container health commit error = %v", err)
	}
}

func TestVerifyRunningServiceReleaseRejectsRunningImageMismatchBeforeHealth(t *testing.T) {
	expected := strings.Repeat("a", 40)
	expectedImage := "sha256:" + strings.Repeat("b", 64)
	expectedUser := "1000:1001"
	runner := &releaseIdentityTestRunner{
		imageID:           expectedImage,
		imageRevision:     expected,
		imageUser:         expectedUser,
		containerID:       strings.Repeat("c", 64),
		runningImageID:    "sha256:" + strings.Repeat("d", 64),
		containerRevision: expected,
		containerUser:     expectedUser,
		healthCommit:      expected,
	}
	err := verifyRunningServiceRelease(
		runner, []string{"docker", "compose"}, []string{"docker"},
		"compose.yml", "/release", []string{"PATH=/bin"}, expected, expectedUser,
	)
	if err == nil || !strings.Contains(err.Error(), "does not equal configured core image") {
		t.Fatalf("running image mismatch error = %v", err)
	}
	for _, request := range runner.requests {
		if strings.Contains(strings.Join(request.Invocation, " "), "release:verify-running-health") {
			t.Fatal("running health was checked after image identity mismatch")
		}
	}
}

func TestVerifyRunningServiceReleaseRejectsStaleContainerRevisionBeforeHealth(t *testing.T) {
	expected := strings.Repeat("a", 40)
	stale := strings.Repeat("f", 40)
	imageID := "sha256:" + strings.Repeat("b", 64)
	expectedUser := "1000:1001"
	runner := &releaseIdentityTestRunner{
		imageID:           imageID,
		imageRevision:     expected,
		imageUser:         expectedUser,
		containerID:       strings.Repeat("c", 64),
		runningImageID:    imageID,
		containerRevision: stale,
		containerUser:     expectedUser,
		healthCommit:      expected,
	}
	err := verifyRunningServiceRelease(
		runner, []string{"docker", "compose"}, []string{"docker"},
		"compose.yml", "/release", []string{"PATH=/bin"}, expected, expectedUser,
	)
	if err == nil || !strings.Contains(err.Error(), "container revision") {
		t.Fatalf("stale running container revision error = %v", err)
	}
	for _, request := range runner.requests {
		if strings.Contains(strings.Join(request.Invocation, " "), "release:verify-running-health") {
			t.Fatal("running health was checked after container revision mismatch")
		}
	}
}

func TestVerifyRunningServiceReleaseRejectsStaleContainerUserBeforeHealth(t *testing.T) {
	expected := strings.Repeat("a", 40)
	imageID := "sha256:" + strings.Repeat("b", 64)
	runner := &releaseIdentityTestRunner{
		imageID:           imageID,
		imageRevision:     expected,
		imageUser:         "1000:1001",
		containerID:       strings.Repeat("c", 64),
		runningImageID:    imageID,
		containerRevision: expected,
		containerUser:     "1002:1003",
		healthCommit:      expected,
	}
	err := verifyRunningServiceRelease(
		runner, []string{"docker", "compose"}, []string{"docker"},
		"compose.yml", "/release", []string{"PATH=/bin"}, expected, "1000:1001",
	)
	if err == nil || !strings.Contains(err.Error(), "does not equal managed host identity") {
		t.Fatalf("stale running container user error = %v", err)
	}
	for _, request := range runner.requests {
		if strings.Contains(strings.Join(request.Invocation, " "), "release:verify-running-health") {
			t.Fatal("running health was checked after container runtime user mismatch")
		}
	}
}

func TestValidateServiceReleaseTargetRejectsUnrelatedService(t *testing.T) {
	err := validateServiceReleaseTarget(serviceCommandOptions{Service: "postgres", Action: "up"})
	if err == nil || !strings.Contains(err.Error(), "requires --service core or --all") {
		t.Fatalf("non-core release target error = %v", err)
	}
}

func serviceTestRequestsContain(requests []serviceProcessRequest, command string) bool {
	for _, request := range requests {
		if strings.Join(request.Invocation, " ") == command {
			return true
		}
	}
	return false
}
