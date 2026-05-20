package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStructuredCommandDecisionBuildsGoCLIDemoWithDownloadedLatestGo(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skipf("test downloads linux-amd64 Go tarball; current platform is %s-%s", runtime.GOOS, runtime.GOARCH)
	}
	latest := latestStableGoVersion(t)
	workspace := t.TempDir()
	command := buildGoCLIDemoCommand(workspace)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":` + quoteJSONForGoCLITest(command) + `,"done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Built demo-go-cli. Run it with ./demo-go-cli from the project directory."}`,
	}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Confidence: 99, Feedback: "command builds and verifies the requested Go CLI project"},
		{Confidence: 95, Feedback: "final answer follows successful evidence"},
	}}
	events := []StructuredCommandEvent{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	result, err := runStructuredCommandDecisionWithConfig(
		ctx,
		"build me a demo go application",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{
			Evaluator:          evaluator,
			EvaluatorThreshold: 70,
		},
	)
	if err != nil {
		t.Fatalf("Go CLI deterministic chain failed: %v\ncommand=%q\nstdout=%s\nstderr=%s", err, result.Command, stdout.String(), stderr.String())
	}
	if len(evaluator.inputs) != 2 {
		t.Fatalf("evaluator calls = %d, want command and done responses evaluated", len(evaluator.inputs))
	}
	if !structuredEventsContain(events, "structured_response_evaluated") {
		t.Fatalf("missing evaluator event: %#v", events)
	}
	validateGoCLIDemo(t, workspace, latest, stdout.String(), result.Answer)
}

func TestLiveOllamaBuildsGoCLIDemoFromSimplePrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live Go CLI demo build in short mode")
	}
	skipUnlessHeavyLiveEnabled(t)
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skipf("test expects linux-amd64 Go tarball; current platform is %s-%s", runtime.GOOS, runtime.GOARCH)
	}
	latest := latestStableGoVersion(t)
	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute

	workspace := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := runStructuredCommandDecisionWithConfig(
		ctx,
		"build me a demo go application",
		nil,
		client,
		stdout,
		stderr,
		nil,
		func(ctx context.Context, question string) (string, error) {
			return "Proceed. Install Go only inside this temporary workspace, then build, test, and run the demo CLI.", nil
		},
		structuredCommandDecisionRunConfig{
			ShellSpecialist: NewOllamaShellCommandSpecialist(client),
		},
	)
	if err != nil {
		if isOllamaRunnerStoppedError(err) {
			t.Skipf("Ollama runner stopped during live Go CLI demo test: %v", err)
		}
		if isLiveModelTimeoutError(err) {
			if validateGoCLIDemoArtifacts(workspace, latest) == nil {
				return
			}
			t.Fatalf("Live model timed out before completing Go CLI chain and artifacts were invalid: %v\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
				err, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations)
		}
		t.Fatalf("Go CLI live build failed: %v\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			err, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations)
	}
	if err := validateGoCLIDemoArtifacts(workspace, latest); err != nil {
		t.Fatalf("Go CLI live artifacts invalid: %v\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			err, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations)
	}
	assertNoFalseCapabilityLimitation(t, client, result, stdout.String(), stderr.String())
	validateGoCLIDemo(t, workspace, latest, stdout.String(), result.Answer)
}

func latestStableGoVersion(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://go.dev/dl/?mode=json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("Go release feed unavailable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Skipf("Go release feed returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		t.Fatalf("parse Go release feed: %v", err)
	}
	for _, release := range releases {
		if release.Stable && strings.TrimSpace(release.Version) != "" {
			return release.Version
		}
	}
	t.Skip("Go release feed contained no stable release")
	return ""
}

func buildGoCLIDemoCommand(workspace string) string {
	return fmt.Sprintf(`set -e
workspace=%[1]q
mkdir -p "$workspace/toolchain" "$workspace/demo-go-cli"
latest=$(curl -fsSL 'https://go.dev/dl/?mode=json' | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\(go[0-9][^"]*\)".*/\1/p' | head -1)
test -n "$latest"
curl -fsSL "https://go.dev/dl/${latest}.linux-amd64.tar.gz" -o "$workspace/go.tar.gz"
rm -rf "$workspace/toolchain/go"
tar -C "$workspace/toolchain" -xzf "$workspace/go.tar.gz"
cd "$workspace/demo-go-cli"
"$workspace/toolchain/go/bin/go" mod init example.com/demo-go-cli
cat > main.go <<'GO'
package main

import "fmt"

func Message() string {
	return "hello from demo go application"
}

func main() {
	fmt.Println(Message())
}
GO
cat > main_test.go <<'GO'
package main

import "testing"

func TestMessage(t *testing.T) {
	if Message() != "hello from demo go application" {
		t.Fatalf("unexpected message: %%s", Message())
	}
}
GO
"$workspace/toolchain/go/bin/go" test ./...
"$workspace/toolchain/go/bin/go" build -o demo-go-cli .
./demo-go-cli
"$workspace/toolchain/go/bin/go" version
printf 'RUN_GUIDE cd %%s/demo-go-cli && ./demo-go-cli\n' "$workspace"`, workspace)
}

func validateGoCLIDemo(t *testing.T, workspace, latest, stdout, answer string) {
	t.Helper()
	if err := validateGoCLIDemoArtifacts(workspace, latest); err != nil {
		t.Fatal(err)
	}
	combined := strings.ToLower(stdout + "\n" + answer)
	for _, want := range []string{"hello from demo go application", strings.ToLower(latest)} {
		if !strings.Contains(combined, want) {
			t.Fatalf("Go CLI output missing %q\nstdout=%s\nanswer=%s", want, stdout, answer)
		}
	}
	if !strings.Contains(combined, "./demo-go-cli") {
		t.Fatalf("Go CLI final guidance missing run command\nstdout=%s\nanswer=%s", stdout, answer)
	}
}

func validateGoCLIDemoArtifacts(workspace, latest string) error {
	for _, rel := range []string{
		"toolchain/go/bin/go",
		"demo-go-cli/go.mod",
		"demo-go-cli/main.go",
		"demo-go-cli/main_test.go",
		"demo-go-cli/demo-go-cli",
	} {
		if _, err := os.Stat(filepath.Join(workspace, rel)); err != nil {
			return err
		}
	}
	versionOut, err := runGoCLIValidationCommand(workspace, filepath.Join(workspace, "toolchain/go/bin/go"), "version")
	if err != nil {
		return err
	}
	if !strings.Contains(versionOut, latest) {
		return fmt.Errorf("downloaded Go version %q did not include latest %q", versionOut, latest)
	}
	testOut, err := runGoCLIValidationCommand(filepath.Join(workspace, "demo-go-cli"), filepath.Join(workspace, "toolchain/go/bin/go"), "test", "./...")
	if err != nil {
		return fmt.Errorf("go test failed: %w\n%s", err, testOut)
	}
	runOut, err := runGoCLIValidationCommand(filepath.Join(workspace, "demo-go-cli"), "./demo-go-cli")
	if err != nil {
		return fmt.Errorf("demo CLI failed: %w\n%s", err, runOut)
	}
	if !strings.Contains(runOut, "hello from demo go application") {
		return fmt.Errorf("demo CLI output missing greeting: %s", runOut)
	}
	return nil
}

func runGoCLIValidationCommand(dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func quoteJSONForGoCLITest(value string) string {
	blob, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(blob)
}
