package omni

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStructuredCommandDecisionBuildsRunsAndVerifiesDockerApp(t *testing.T) {
	requireDockerDaemon(t)
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skipf("scratch image smoke test builds linux-amd64 binary; current platform is %s-%s", runtime.GOOS, runtime.GOARCH)
	}

	appDir := t.TempDir()
	port := freeTCPPort(t)
	name := fmt.Sprintf("omni-docker-smoke-%d", time.Now().UnixNano())
	image := name + ":test"
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
		_ = exec.Command("docker", "rmi", "-f", image).Run()
	})

	commands := dockerSmokePlannerCommands(name, image, port)
	client := &fakeCommandDecisionClient{responses: fakePlannerCommandResponses(commands, "Docker app built, ran, passed its health check, and remained stable.")}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: dockerSmokeObjectives(commands),
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := runStructuredCommandDecisionWithConfig(
		ctx,
		"Create, build, and run a simple Docker application, then prove it is alive, stable, and free of error logs.",
		nil,
		client,
		stdout,
		stderr,
		nil,
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: appDir,
			PromptInterpreter:       interpreter,
		},
	)
	if err != nil {
		t.Fatalf("docker smoke failed: %v\ncommand=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			err, result.Command, stdout.String(), stderr.String(), result.Observations)
	}
	if client.calls < len(commands) || client.calls > len(commands)+1 {
		t.Fatalf("llm calls = %d, want %d command decisions and at most one done request", client.calls, len(commands))
	}
	assertSuccessfulPlannerCommandsObserved(t, commands, result.Observations)
	validateDockerSmokeEvidence(t, name, stdout.String(), stderr.String(), result.Answer)
}

func requireDockerDaemon(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker command not found: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("docker daemon unavailable: %v\n%s", err, string(out))
	}
}

func dockerSmokePlannerCommands(name, image string, port int) []string {
	return []string{`cat > main.go <<'GO'
package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "omni docker smoke alive")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "omni docker smoke")
	})
	log.Println("omni docker smoke server listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
GO
`, `cat > Dockerfile <<'DOCKER'
FROM scratch
COPY app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
DOCKER
`,
		"go mod init example.com/omni-docker-smoke",
		"CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags='-s -w' -o app .",
		fmt.Sprintf("docker build -t %s .", image),
		fmt.Sprintf("docker run -d --name %s --restart=no -p 127.0.0.1:%d:8080 %s", name, port, image),
		fmt.Sprintf("curl --retry 10 --retry-delay 1 --retry-connrefused -fsS http://127.0.0.1:%d/health", port),
		fmt.Sprintf("docker inspect -f 'running={{.State.Running}} restarting={{.State.Restarting}} restart_count={{.RestartCount}}' %s", name),
		fmt.Sprintf("docker logs %s", name),
	}
}

func dockerSmokeObjectives(commands []string) []StructuredObjective {
	definitions := []struct {
		id          string
		description string
		kind        WorkItemKind
	}{
		{"write_server", "Create the Go HTTP server source", WorkItemKindCreate},
		{"write_dockerfile", "Create the scratch-image Dockerfile", WorkItemKindCreate},
		{"initialize_module", "Initialize the Go module", WorkItemKindVerify},
		{"build_binary", "Build the static Go server binary", WorkItemKindVerify},
		{"build_image", "Build the Docker image", WorkItemKindVerify},
		{"run_container", "Run the requested container", WorkItemKindVerify},
		{"verify_health", "Verify the container health endpoint", WorkItemKindVerify},
		{"inspect_container", "Inspect running, restarting, and restart-count state", WorkItemKindVerify},
		{"inspect_logs", "Inspect the container logs", WorkItemKindVerify},
	}
	objectives := make([]StructuredObjective, 0, len(definitions))
	for i, definition := range definitions {
		objective := StructuredObjective{
			ID:          definition.id,
			Description: definition.description,
			Status:      "pending",
			Kind:        string(definition.kind),
			Source:      structuredObjectiveSourceUserExplicit,
			Required:    true,
		}
		if definition.kind == WorkItemKindVerify {
			objective.RequiredEvidence = []string{"command_passed:" + commands[i]}
		}
		objectives = append(objectives, objective)
	}
	return objectives
}

func fakePlannerCommandResponses(commands []string, answer string) []string {
	responses := make([]string, 0, len(commands)+1)
	for _, command := range commands {
		responses = append(responses, `{"command":`+quoteJSONForGoCLITest(command)+`,"done":false,"answer":""}`)
	}
	responses = append(responses, `{"command":"","done":true,"answer":`+quoteJSONForGoCLITest(answer)+`}`)
	return responses
}

func assertSuccessfulPlannerCommandsObserved(t *testing.T, commands []string, observations []StructuredCommandObservation) {
	t.Helper()
	for _, command := range commands {
		want := normalizeStructuredCommandForComparison(command)
		found := false
		for _, observation := range observations {
			if observation.ExitCode == 0 && normalizeStructuredCommandForComparison(observation.Command) == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("successful planner command was not observed: %q\nobservations=%#v", command, observations)
		}
	}
}

func TestDockerSmokePlannerCommandsAreBoundedPolicyUnits(t *testing.T) {
	commands := dockerSmokePlannerCommands("omni-docker-contract", "omni-docker-contract:test", 8081)
	for _, command := range commands {
		if err := validateUnsafeMutationCommandShape(command); err != nil {
			t.Fatalf("bounded Docker command rejected: %q: %v", command, err)
		}
	}
}

func validateDockerSmokeEvidence(t *testing.T, name, stdout, stderr, answer string) {
	t.Helper()
	combined := stdout + "\n" + stderr + "\n" + answer
	for _, want := range []string{
		"omni docker smoke alive",
		"running=true",
		"restarting=false",
		"restart_count=0",
		"omni docker smoke server listening",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("docker smoke evidence missing %q\nstdout=%s\nstderr=%s\nanswer=%s", want, stdout, stderr, answer)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	inspect := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}} {{.State.Restarting}} {{.RestartCount}}", name)
	out, err := inspect.CombinedOutput()
	if err != nil {
		t.Fatalf("docker inspect validation failed: %v\n%s", err, string(out))
	}
	if strings.TrimSpace(string(out)) != "true false 0" {
		t.Fatalf("unexpected docker state: %q", string(out))
	}

	logCmd := exec.CommandContext(ctx, "docker", "logs", name)
	logBytes, err := logCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker logs validation failed: %v\n%s", err, string(logBytes))
	}
	logs := strings.ToLower(string(logBytes))
	for _, bad := range []string{"panic", "fatal", "error", "traceback", "exception"} {
		if strings.Contains(logs, bad) {
			t.Fatalf("docker logs contain %q:\n%s", bad, string(logBytes))
		}
	}
	if !strings.Contains(logs, "omni docker smoke server listening") {
		t.Fatalf("docker logs missing startup evidence:\n%s", string(logBytes))
	}
}

func TestLiveOllamaBuildsRunsAndVerifiesDockerApp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live Docker app build in short mode")
	}
	if strings.TrimSpace(os.Getenv("OMNI_RUN_DOCKER_LIVE")) == "" {
		t.Skip("set OMNI_RUN_DOCKER_LIVE=1 to run live Docker app build test")
	}
	requireDockerDaemon(t)
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skipf("live Docker smoke test expects linux-amd64 host; current platform is %s-%s", runtime.GOOS, runtime.GOARCH)
	}

	root := t.TempDir()
	port := freeTCPPort(t)
	name := fmt.Sprintf("omni-live-docker-smoke-%d", time.Now().UnixNano())
	image := name + ":test"
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
		_ = exec.Command("docker", "rmi", "-f", image).Run()
	})

	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute
	prompt := fmt.Sprintf("Build a simple Docker web application in %s, run it as container %s from image %s on host port %d, confirm it is alive with curl, inspect Docker state to prove it is running and not restarting, verify restart count is zero, inspect docker logs, and report how to run/check it. Use a local static Go binary and FROM scratch if that avoids pulling base images. Do not install packages.", root, name, image, port)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	result, err := RunStructuredCommandDecision(ctx, prompt, client, stdout, stderr)
	if err != nil {
		if isOllamaRunnerStoppedError(err) || isLiveModelTimeoutError(err) {
			t.Skipf("live Docker model run unavailable: %v", err)
		}
		t.Fatalf("live Docker app build failed: %v\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			err, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations)
	}
	assertNoFalseCapabilityLimitation(t, client, result, stdout.String(), stderr.String())
	validateDockerSmokeEvidence(t, name, stdout.String(), stderr.String(), result.Answer)
}
