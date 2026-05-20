package odn

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	root := t.TempDir()
	port := freeTCPPort(t)
	name := fmt.Sprintf("odn-docker-smoke-%d", time.Now().UnixNano())
	image := name + ":test"
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
		_ = exec.Command("docker", "rmi", "-f", image).Run()
	})

	command := buildDockerSmokeCommand(root, name, image, port)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":` + quoteJSONForGoCLITest(command) + `,"done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Docker app built, ran, passed health check, had clear logs, and was not in a restart loop."}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := RunStructuredCommandDecision(ctx, "Build and run a simple Docker application, then prove it is alive and healthy.", client, stdout, stderr)
	if err != nil {
		t.Fatalf("docker smoke failed: %v\ncommand=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			err, result.Command, stdout.String(), stderr.String(), result.Observations)
	}
	if client.calls != 2 {
		t.Fatalf("llm calls = %d, want 2", client.calls)
	}
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

func buildDockerSmokeCommand(root, name, image string, port int) string {
	return fmt.Sprintf(`set -e
root=%[1]q
name=%[2]q
image=%[3]q
port=%[4]d
mkdir -p "$root/app"
cat > "$root/app/main.go" <<'GO'
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
		fmt.Fprintln(w, "odn docker smoke alive")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "odn docker smoke")
	})
	log.Println("odn docker smoke server listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
GO
cat > "$root/app/Dockerfile" <<'DOCKER'
FROM scratch
COPY app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
DOCKER
cd "$root/app"
go mod init example.com/odn-docker-smoke
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o app .
docker rm -f "$name" >/dev/null 2>&1 || true
docker rmi -f "$image" >/dev/null 2>&1 || true
docker build -t "$image" .
docker run -d --name "$name" --restart=no -p 127.0.0.1:"$port":8080 "$image"
for i in 1 2 3 4 5 6 7 8 9 10; do
	curl -fsS "http://127.0.0.1:$port/health" && break
	sleep 1
done
health=$(curl -fsS "http://127.0.0.1:$port/health")
test "$health" = "odn docker smoke alive"
running=$(docker inspect -f '{{.State.Running}}' "$name")
restarting=$(docker inspect -f '{{.State.Restarting}}' "$name")
restart_count=$(docker inspect -f '{{.RestartCount}}' "$name")
test "$running" = "true"
test "$restarting" = "false"
test "$restart_count" = "0"
logs=$(docker logs "$name" 2>&1)
printf '%%s\n' "$logs" | grep -Eiq 'panic|fatal|error|traceback|exception' && { printf 'bad docker logs:\n%%s\n' "$logs" >&2; exit 1; }
printf 'DOCKER_SMOKE_OK container=%%s image=%%s port=%%s health=%%s running=%%s restarting=%%s restart_count=%%s\n' "$name" "$image" "$port" "$health" "$running" "$restarting" "$restart_count"
printf 'DOCKER_LOGS_CLEAR\n'`, root, name, image, port)
}

func validateDockerSmokeEvidence(t *testing.T, name, stdout, stderr, answer string) {
	t.Helper()
	combined := stdout + "\n" + stderr + "\n" + answer
	for _, want := range []string{
		"DOCKER_SMOKE_OK",
		"health=odn docker smoke alive",
		"running=true",
		"restarting=false",
		"restart_count=0",
		"DOCKER_LOGS_CLEAR",
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
	if !strings.Contains(logs, "odn docker smoke server listening") {
		t.Fatalf("docker logs missing startup evidence:\n%s", string(logBytes))
	}
}

func TestLiveOllamaBuildsRunsAndVerifiesDockerApp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live Docker app build in short mode")
	}
	if strings.TrimSpace(os.Getenv("ODN_RUN_DOCKER_LIVE")) == "" {
		t.Skip("set ODN_RUN_DOCKER_LIVE=1 to run live Docker app build test")
	}
	requireDockerDaemon(t)
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skipf("live Docker smoke test expects linux-amd64 host; current platform is %s-%s", runtime.GOOS, runtime.GOARCH)
	}

	root := t.TempDir()
	port := freeTCPPort(t)
	name := fmt.Sprintf("odn-live-docker-smoke-%d", time.Now().UnixNano())
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

func TestBuildDockerSmokeCommandMentionsRequiredLifecycleChecks(t *testing.T) {
	command := buildDockerSmokeCommand(filepath.Join(t.TempDir(), "x"), "odn-docker-contract", "odn-docker-contract:test", 8081)
	for _, want := range []string{"docker build", "docker run -d", "curl -fsS", "docker inspect", ".RestartCount", ".State.Restarting", "docker logs", "DOCKER_LOGS_CLEAR"} {
		if !strings.Contains(command, want) {
			t.Fatalf("docker smoke command missing %q\n%s", want, command)
		}
	}
}
