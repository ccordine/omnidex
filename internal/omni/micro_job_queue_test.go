package omni

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteMicroJobQueueRunsTinyJobsSequentiallyWithPriorContext(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"jobs":[{"id":"job_1","objective":"create marker file","acceptance":"marker file exists"},{"id":"job_2","objective":"read marker file","acceptance":"stdout contains marker text"}]}`,
		`{"command":"printf 'marker created\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"marker created"}`,
		`{"command":"printf 'marker read\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"marker read"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := ExecuteMicroJobQueue(context.Background(), "build in tiny steps", "/tmp/workspace", client, stdout, stderr, MicroJobQueueConfig{MaxJobs: 4, DisableProjectProfile: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done {
		t.Fatalf("result not done: %#v", result)
	}
	if len(result.Results) != 2 {
		t.Fatalf("results = %#v, want 2", result.Results)
	}
	if client.calls != 5 {
		t.Fatalf("llm calls = %d, want plan + two command loops", client.calls)
	}
	if !strings.Contains(client.prompts[3], `job_1 done=true`) || !strings.Contains(client.prompts[3], `marker created`) {
		t.Fatalf("second job prompt missing prior job context: %s", client.prompts[3])
	}
	if stdout.String() != "marker created\nmarker read\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestExecuteMicroJobQueueStopsOnFailedMicroJob(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"jobs":[{"id":"job_1","objective":"attempt risky step","acceptance":"command succeeds"},{"id":"job_2","objective":"must not run","acceptance":"should not execute"}]}`,
		`{"command":"printf 'failed\n' >&2; exit 2","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"failed anyway"}`,
		`{"command":"","done":true,"answer":"still failed"}`,
		`{"command":"","done":true,"answer":"still failed"}`,
		`{"command":"","done":true,"answer":"still failed"}`,
		`{"command":"","done":true,"answer":"still failed"}`,
		`{"command":"","done":true,"answer":"still failed"}`,
		`{"command":"","done":true,"answer":"still failed"}`,
		`{"command":"","done":true,"answer":"still failed"}`,
		`{"command":"","done":true,"answer":"still failed"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := ExecuteMicroJobQueue(context.Background(), "stop on failure", "/tmp/workspace", client, stdout, stderr, MicroJobQueueConfig{MaxJobs: 4, DisableProjectProfile: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Done {
		t.Fatalf("result unexpectedly done: %#v", result)
	}
	if len(result.Results) != 1 {
		t.Fatalf("results = %#v, want only failed first job", result.Results)
	}
	if result.Results[0].Error == "" {
		t.Fatalf("failed job missing error: %#v", result.Results[0])
	}
	wantCalls := defaultCommandDecisionMaxSteps + 1
	if client.calls != wantCalls {
		t.Fatalf("llm calls = %d, want plan + exhausted first job only (%d)", client.calls, wantCalls)
	}
	if strings.Contains(client.prompts[len(client.prompts)-1], "Current micro job:\njob_2") {
		t.Fatalf("second job appears to have run: prompts=%#v", client.prompts)
	}
	if stderr.String() != "failed\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestParseMicroJobPlanRequiresTinyActionableJobs(t *testing.T) {
	_, err := parseMicroJobPlan(`{"jobs":[{"id":"job_1","objective":"","acceptance":"done"}]}`, 4)
	if err == nil {
		t.Fatal("expected empty objective to fail")
	}
	_, err = parseMicroJobPlan(`{"jobs":[{"id":"job_1","objective":"do thing","acceptance":""}]}`, 4)
	if err == nil {
		t.Fatal("expected empty acceptance to fail")
	}
}

func TestMicroQueueObjectiveParsesExplicitSlashCommands(t *testing.T) {
	for _, input := range []string{"/micro build the app", "/queue build the app"} {
		objective, ok := microQueueObjective(input)
		if !ok || objective != "build the app" {
			t.Fatalf("microQueueObjective(%q) = %q, %t", input, objective, ok)
		}
	}
	if objective, ok := microQueueObjective("build the app"); ok || objective != "" {
		t.Fatalf("natural prompt should not route to micro queue: %q %t", objective, ok)
	}
}

func TestExecuteMicroJobQueueBuildsProjectRunProfileBeforePlanning(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, workspace, "go.mod", "module example.test/app\n\ngo 1.24.1\n")
	writeTestFile(t, workspace, "main.go", "package main\nfunc main() {}\n")
	client := &fakeCommandDecisionClient{responses: []string{
		`{"summary":"Go app inferred from go.mod","languages":["Go"],"frameworks":[],"run_commands":["go run ."],"test_commands":["go test ./..."],"build_commands":["go build ./..."],"evidence":["go.mod","main.go"]}`,
		`{"jobs":[{"id":"job_1","objective":"run project tests using the profiled test command","acceptance":"go test succeeds"}]}`,
		`{"command":"printf 'tests passed\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"tests passed"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := ExecuteMicroJobQueue(context.Background(), "verify the project", workspace, client, stdout, stderr, MicroJobQueueConfig{MaxJobs: 4})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProjectProfile.Summary != "Go app inferred from go.mod" {
		t.Fatalf("profile = %#v", result.ProjectProfile)
	}
	if client.calls != 4 {
		t.Fatalf("llm calls = %d, want profile + plan + command loop", client.calls)
	}
	if !strings.Contains(client.prompts[1], "go test ./...") {
		t.Fatalf("planner prompt missing project profile: %s", client.prompts[1])
	}
	if !strings.Contains(client.prompts[2], "Project run profile:") || !strings.Contains(client.prompts[2], "go run .") {
		t.Fatalf("job prompt missing project profile: %s", client.prompts[2])
	}
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
