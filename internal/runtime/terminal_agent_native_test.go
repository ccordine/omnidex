package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

// This invokes the installed omni chat in a real PTY, updates existing source,
// observes its recap, then submits another task in the same terminal session.
func TestInstalledTerminalAgentsUpdateExistingProject(t *testing.T) {
	command, modelName, databaseURL := os.Getenv("OMNI_TEST_CODEX_COMMAND"), os.Getenv("OMNI_TEST_CODEX_MODEL"), os.Getenv("OMNI_TEST_DATABASE_URL")
	if command == "" || modelName == "" || databaseURL == "" {
		t.Skip("installed CLI/Codex and isolated PostgreSQL qualification not selected")
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Fatal("real PTY qualification requires script")
	}
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(repository, "bin", "cli-agent-qualification")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.MkdirTemp(base, "terminal-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(fixture) })
	root := filepath.Join(fixture, "project with spaces")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"greeting.py":      "def greeting():\n    return 'old'\n",
		"test_greeting.py": "import unittest\nfrom greeting import greeting\nclass GreetingTest(unittest.TestCase):\n    def test_greeting(self):\n        self.assertEqual(greeting(), 'old')\n",
		"keep.txt":         "existing unrelated owner data\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, field := range modelconfig.RegisteredFields() {
		for _, key := range field.EnvKeys {
			t.Setenv(key, modelName)
		}
	}
	for key, value := range map[string]string{"DATABASE_URL": databaseURL, "DATABASE_SCHEMA": fmt.Sprintf("terminal_agents_%d", time.Now().UnixNano()), "LLM_PROVIDER": "codex", "CODEX_COMMAND": command, "CODEX_DIRECTORY": repository, "HOST_DIRECTORY_ACCESS_ROOT": filepath.Join(fixture, "server-only"), "WORKER_POLL_INTERVAL": "50ms", "REDIS_URL": "", "LISTEN_ADDR": "127.0.0.1:0"} {
		t.Setenv(key, value)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime, err := New(ctx, cfg, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = runtime.pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+cfg.DatabaseSchema+" CASCADE")
		runtime.Close()
	}()
	server := httptest.NewServer(runtime.server.Handler())
	defer server.Close()
	workers := make(chan error, 1)
	go func() { workers <- runtime.worker.Start(runtime.context) }()
	defer func() { runtime.cancel(); <-workers }()
	process := exec.CommandContext(ctx, "script", "--quiet", "--return", "--flush", "--command", "stty rows 24 cols 100; exec omni chat", "/dev/null")
	process.Dir = root
	process.Env = append(os.Environ(), "CORE_URL="+server.URL, "TERM=xterm-256color")
	transcript := &terminalTranscript{}
	process.Stdout, process.Stderr = transcript, transcript
	input, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- process.Wait() }()
	defer func() { _ = input.Close(); cancel(); <-finished }()
	wait := func(check func() bool) {
		for !check() {
			select {
			case err := <-finished:
				finished <- err
				t.Fatalf("terminal exited before completion: %v\n%s", err, transcript.text())
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	wait(func() bool { return strings.Contains(transcript.text(), "mode: Agents") })
	var previousID int64
	for index, value := range []string{"hello", "goodbye"} {
		instruction := fmt.Sprintf("In this existing Python project, change greeting() in greeting.py to return %q. Update test_greeting.py to verify that result. Keep keep.txt unchanged. Run python3 -B -m unittest test_greeting.", value)
		if _, err := io.WriteString(input, instruction+"\n"); err != nil {
			t.Fatal(err)
		}
		var details model.JobDetails
		wait(func() bool {
			var id int64
			err := runtime.pool.QueryRow(ctx, `SELECT id FROM jobs WHERE metadata->>'client_cwd'=$1 AND id>$2 ORDER BY id DESC LIMIT 1`, root, previousID).Scan(&id)
			if err != nil {
				return false
			}
			response, err := http.Get(fmt.Sprintf("%s/v1/jobs/%d", server.URL, id))
			if err != nil {
				t.Fatal(err)
			}
			err = json.NewDecoder(response.Body).Decode(&details)
			_ = response.Body.Close()
			if err != nil || response.StatusCode != http.StatusOK {
				t.Fatalf("observe terminal job: status=%d error=%v", response.StatusCode, err)
			}
			switch details.Job.Status {
			case model.JobStatusFailed, model.JobStatusCanceled, model.JobStatusWaiting:
				questions := []string{}
				if details.AgentPause != nil {
					questions = details.AgentPause.Questions
				}
				for _, step := range details.Steps {
					if step.Status == model.StepStatusWaiting {
						questions = append(questions, step.Error)
					}
				}
				var assignment json.RawMessage
				_ = runtime.pool.QueryRow(ctx, `SELECT jsonb_build_object('role',payload_json->>'role','task',payload_json->'assignment'->'task') FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'phase'='call_opened' ORDER BY id DESC LIMIT 1`, details.Job.ID).Scan(&assignment)
				t.Fatalf("ordinary terminal job %d stopped as %s: %s; questions=%v; assignment=%s", details.Job.ID, details.Job.Status, details.Job.Error, questions, assignment)
			}
			return details.Job.Status == model.JobStatusCompleted
		})
		wait(func() bool { return strings.Count(transcript.text(), "Written files:") >= index+1 })
		verification := exec.Command("python3", "-B", "-c", fmt.Sprintf("from greeting import greeting; assert greeting()==%q", value))
		verification.Dir = root
		if output, err := verification.CombinedOutput(); err != nil {
			t.Fatalf("completed terminal output failed independent holdout: %s %v", output, err)
		}
		unchanged, err := os.ReadFile(filepath.Join(root, "keep.txt"))
		if err != nil || string(unchanged) != "existing unrelated owner data\n" {
			t.Fatal("terminal task changed unrelated existing data")
		}
		previousID = details.Job.ID
		t.Logf("terminal task %d completed as job %d; real existing files and recap verified", index+1, previousID)
	}
}

type terminalTranscript struct {
	mu    sync.Mutex
	bytes []byte
}

func (transcript *terminalTranscript) Write(value []byte) (int, error) {
	transcript.mu.Lock()
	defer transcript.mu.Unlock()
	transcript.bytes = append(transcript.bytes, value...)
	if len(transcript.bytes) > 256*1024 {
		transcript.bytes = append([]byte{}, transcript.bytes[len(transcript.bytes)-256*1024:]...)
	}
	return len(value), nil
}

func (transcript *terminalTranscript) text() string {
	transcript.mu.Lock()
	defer transcript.mu.Unlock()
	return string(transcript.bytes)
}
