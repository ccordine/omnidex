package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Opt-in native qualification uses installed sign-in, not synthetic RPC success.
// TempDir under bin/agent-qualification owns <=1MiB source fixtures; its parent
// is retired when empty. bin/agent-runtime owns disposable native tool caches.
func installedAgentClient(t *testing.T) (*Client, string) {
	t.Helper()
	command, model := os.Getenv("OMNI_TEST_CODEX_COMMAND"), os.Getenv("OMNI_TEST_CODEX_MODEL")
	if command == "" || model == "" {
		t.Skip("installed Codex agent qualification not selected")
	}
	directory, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(directory, "bin", "agent-qualification")
	_, before := os.Stat(base)
	created := os.IsNotExist(before)
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	if created {
		t.Cleanup(func() {
			if err := os.Remove(base); err != nil && !os.IsNotExist(err) {
				t.Errorf("retire empty native source fixture parent: %v", err)
			}
		})
	}
	t.Setenv("TMPDIR", base)
	client, err := New(command, directory, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return client, model
}

func nativeAgentFixture(t *testing.T, model string) codingagent.Assignment {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("native qualification requires the installed Python verifier")
	}
	root := t.TempDir()
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for file, content := range map[string]string{
		"allowed.py":     "def greet(name):\n    return 'broken'\n",
		"contract.txt":   "greet(name) returns Hello, followed by one space, the exact name, and an exclamation point.\n",
		"undeclared.txt": "keep this exact fixture unchanged\n",
	} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	task := codingagent.Task{ID: "greeting", Goal: "Implement greet according to the granted contract.txt.",
		Acceptance: []string{`greet("Ada") == "Hello, Ada!" and greet("") == "Hello, !"`},
		ReadFiles:  []string{"contract.txt"}, WriteFiles: []string{"allowed.py"}, Dependencies: []string{},
		RequirementIDs: []string{"greeting-result"}, Checks: []codingagent.Check{{ID: "greeting-behavior", Argv: []string{
			python, "-B", "-c", `import runpy; greet=runpy.run_path("allowed.py")["greet"]; assert greet("Ada")=="Hello, Ada!"; assert greet("")=="Hello, !"; print("greeting-behavior verified")`,
		}}}}
	return codingagent.Assignment{JobID: 17, Generation: 1, Role: codingagent.Execute, Task: &task,
		Root: root, ReadFiles: []string{"allowed.py", "contract.txt"}, WriteFiles: []string{"allowed.py"},
		Route: codingagent.Route{Provider: "codex", Model: model}}
}

func TestInstalledCodexAgentExecutionAndIndependentReview(t *testing.T) {
	client, model := installedAgentClient(t)
	scope := nativeAgentFixture(t, model)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	frames, effects := 0, 0
	writer := &agentFixtureWriter{scope: scope}
	activityProof := nativeActivityQualification{call: scope}
	result, err := client.Run(ctx, scope, writer, func(event codingagent.NativeEvent) error {
		if err := activityProof.native(event); err != nil {
			return err
		}
		if len(event.Frame) == 0 {
			return nil
		}
		frames++
		var frame struct {
			Method string `json:"method"`
			Params struct {
				Item struct {
					Type string `json:"type"`
				} `json:"item"`
			} `json:"params"`
		}
		if err := json.Unmarshal(event.Frame, &frame); err != nil {
			return err
		}
		if frame.Method == "item/completed" && (frame.Params.Item.Type == "commandExecution" || frame.Params.Item.Type == "dynamicToolCall") {
			effects++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed || result.ThreadID == "" || result.TurnID == "" || frames == 0 || effects == 0 || writer.writes == 0 {
		t.Fatalf("native execution lacks exact completion and original environment-effect receipts: completed=%v frames=%d effects=%d result=%s", result.Completed, frames, effects, result.Content)
	}
	var execution codingagent.ExecutionResult
	if err := json.Unmarshal(result.Content, &execution); err != nil || execution.Questions == nil || len(execution.Questions) != 0 {
		t.Fatalf("native execution did not fulfill its complete supplied contract: decode=%v", err)
	}
	checks, err := client.ExecuteChecks(ctx, scope, scope.Task.Checks, activityProof.verification)
	if err != nil || len(checks) != 1 || checks[0].ExitCode != 0 {
		t.Fatalf("actual native behavior verification failed: checks=%+v error=%v", checks, err)
	}
	activityProof.verify(t, writer.writes)
	reviewScope := scope
	reviewScope.Role, reviewScope.WriteFiles, reviewScope.Checks = codingagent.Review, nil, checks
	reviewActivityProof := nativeActivityQualification{call: reviewScope}
	review, err := client.Run(ctx, reviewScope, nil, reviewActivityProof.native)
	if err != nil {
		t.Fatal(err)
	}
	if !review.Completed || review.ThreadID == result.ThreadID {
		t.Fatal("review reused the executor's retained conversation")
	}
	var reviewed codingagent.ReviewResult
	if err := json.Unmarshal(review.Content, &reviewed); err != nil || reviewed.Findings == nil || reviewed.Questions == nil || len(reviewed.Findings) != 0 || len(reviewed.Questions) != 0 {
		t.Fatalf("independent reviewer did not verify the actual fulfilled task: decode=%v findings=%+v questions=%+v", err, reviewed.Findings, reviewed.Questions)
	}
	unchanged, err := os.ReadFile(filepath.Join(scope.Root, "undeclared.txt"))
	if err != nil || string(unchanged) != "keep this exact fixture unchanged\n" {
		t.Fatal("native execution changed an undeclared adjacent file")
	}
	t.Logf("native execution thread=%s turn=%s frames=%d effects=%d; independent review thread=%s turn=%s", result.ThreadID, result.TurnID, frames, effects, review.ThreadID, review.TurnID)
}

func TestInstalledCodexAgentSandboxAndServiceEnvironment(t *testing.T) {
	client, model := installedAgentClient(t)
	scope := nativeAgentFixture(t, model)
	t.Setenv("OMNI_AGENT_SERVICE_SECRET", "qualification-only-value")
	python := scope.Task.Checks[0].Argv[0]
	denied := `import errno, os
assert "OMNI_AGENT_SERVICE_SECRET" not in os.environ
for path, mode in [("undeclared.txt", "r"), ("allowed.py", "w")]:
    try:
        stream=open(path,mode)
    except OSError as failure:
        assert failure.errno in (errno.EACCES, errno.EPERM, errno.ENOENT, errno.EROFS)
        continue
    stream.close()
    raise AssertionError("ungranted file operation succeeded: " + path + " " + mode)
# A private mount shadow may accept a new file; it confers no host authority.
# The Go host independently verifies that both canonical files remain exact.
try:
    with open("undeclared.txt", "w") as stream:
        stream.write("disposable sandbox shadow")
except OSError as failure:
    assert failure.errno in (errno.EACCES, errno.EPERM, errno.ENOENT, errno.EROFS)
assert os.path.realpath(os.environ["HOME"])==os.path.realpath(os.environ["TMPDIR"])
`
	results, err := client.ExecuteChecks(context.Background(), scope, []codingagent.Check{{ID: "actual-sandbox-denials", Argv: []string{python, "-B", "-c", denied}}}, nil)
	for file, expected := range map[string]string{"allowed.py": "def greet(name):\n    return 'broken'\n", "undeclared.txt": "keep this exact fixture unchanged\n"} {
		content, err := os.ReadFile(filepath.Join(scope.Root, file))
		if err != nil || string(content) != expected {
			t.Fatalf("read-only native verification changed %s", file)
		}
	}
	if err != nil || len(results) != 1 || results[0].ExitCode != 0 {
		t.Fatalf("actual native host files remained unchanged, but sandbox denial semantics differ: checks=%+v error=%v", results, err)
	}
}

func TestInstalledCodexAgentRunningCommandCancellation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("actual child lifetime inspection requires Linux /proc")
	}
	client, model := installedAgentClient(t)
	scope := nativeAgentFixture(t, model)
	client.timeout = 45 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	marker := fmt.Sprintf("cancel-%d.pid", time.Now().UnixNano())
	script := `import os, pathlib, sys, time; pathlib.Path(os.environ["TMPDIR"],sys.argv[1]).write_text(str(os.getpid())); time.sleep(120)`
	finished := make(chan error, 1)
	go func() {
		_, err := client.ExecuteChecks(ctx, scope, []codingagent.Check{{ID: "actual-running-command", Argv: []string{scope.Task.Checks[0].Argv[0], "-B", "-c", script, marker}}}, nil)
		finished <- err
	}()
	var pid int
	t.Cleanup(func() {
		cancel()
		if pid > 0 {
			if command, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil && strings.Contains(string(command), marker) {
				process, err := os.FindProcess(pid)
				if err == nil {
					_ = process.Kill()
				}
			}
		}
	})
	deadline := time.Now().Add(15 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		pid = markedNativeChild(scope.Task.Checks[0].Argv[0], marker)
		select {
		case err := <-finished:
			t.Fatalf("native command ended before it was observed running: %v", err)
		default:
		}
		if pid == 0 {
			time.Sleep(25 * time.Millisecond)
		}
	}
	if pid == 0 {
		t.Fatal("native command was never observed running")
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("canceled native command reported a completed verification")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("canceled native verification did not release its process")
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		command, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if os.IsNotExist(err) || err == nil && !strings.Contains(string(command), marker) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("cancellation left the actual native command running")
}

// Sandbox PID namespaces make an in-command getpid unsuitable for host cleanup.
// Match only the exact frozen executable and this test's unique argument.
func markedNativeChild(executable, marker string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid < 1 {
			continue
		}
		command, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(strings.TrimRight(string(command), "\x00"), "\x00")
		if len(args) > 1 && args[0] == executable && args[len(args)-1] == marker {
			return pid
		}
	}
	return 0
}
