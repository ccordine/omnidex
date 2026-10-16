package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
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
	_, before := os.Stat(base)
	if os.IsNotExist(before) {
		t.Cleanup(func() {
			if err := os.Remove(base); err != nil && !os.IsNotExist(err) {
				t.Errorf("retire terminal fixture parent: %v", err)
			}
		})
	}
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
	// The server and its local terminal share one isolated installation identity.
	// A child-only config root would correctly look like a foreign environment.
	clientConfigRoot := filepath.Join(fixture, "client-config")
	t.Setenv("XDG_CONFIG_HOME", clientConfigRoot)
	for _, field := range modelconfig.RegisteredFields() {
		for _, key := range field.EnvKeys {
			t.Setenv(key, modelName)
		}
	}
	// Acceptance exercises the normal low-effort/default-tier profile. Public
	// native summaries remain optional; the observer never changes inference.
	for _, role := range []string{"PLANNER", "EXECUTOR", "REVIEWER"} {
		t.Setenv("CODING_"+role+"_EFFORT", "low")
		t.Setenv("CODING_"+role+"_SERVICE_TIER", "default")
	}
	for key, value := range map[string]string{"DATABASE_URL": databaseURL, "DATABASE_SCHEMA": fmt.Sprintf("terminal_agents_%d", time.Now().UnixNano()), "LLM_PROVIDER": "codex", "CODEX_COMMAND": command, "CODEX_DIRECTORY": repository, "HOST_DIRECTORY_ACCESS_ROOT": filepath.Join(fixture, "server-only"), "WORKER_POLL_INTERVAL": "50ms", "REDIS_URL": "", "LISTEN_ADDR": "127.0.0.1:0"} {
		t.Setenv(key, value)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []codingagent.Route{cfg.CodingAgents.Planner, cfg.CodingAgents.Executor, cfg.CodingAgents.Reviewer} {
		if route.Effort != "low" || route.ServiceTier != "default" {
			t.Fatalf("terminal acceptance changed the default inference profile: %+v", route)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
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
	cli := os.Getenv("OMNI_TEST_CLI_COMMAND")
	if cli == "" {
		cli = "omni"
	}
	quotedCLI := "'" + strings.ReplaceAll(cli, "'", "'\\''") + "'"
	process := exec.CommandContext(ctx, "script", "--quiet", "--return", "--flush", "--command", "stty rows 24 cols 100; exec "+quotedCLI+" chat", "/dev/null")
	process.Dir = root
	process.Env = append(os.Environ(), "CORE_URL="+server.URL, "TERM=xterm-256color", "XDG_CONFIG_HOME="+clientConfigRoot)
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
	wait(func() bool { return strings.Contains(transcript.text(), "Omnidex · Agents") })
	clientID, err := projectroot.ReadClientInstallationIdentity(clientConfigRoot)
	if err != nil {
		t.Fatal(err)
	}
	directoryID, err := projectroot.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	workspaceIdentity, err := projectroot.ClientWorkspaceIdentity(clientID, directoryID)
	if err != nil {
		t.Fatal(err)
	}
	var previousID int64
	for index, value := range []string{"hello", "goodbye"} {
		if index == 1 {
			if _, err := io.WriteString(input, "/verbosity verbose\n"); err != nil {
				t.Fatal(err)
			}
			wait(func() bool { return strings.Contains(transcript.text(), "Verbosity: verbose") })
		}
		cursor := transcript.position()
		activity := terminalWorkflowActivity{requireOutput: index == 1}
		planReviewDisplayed, approvedPlan := false, false
		var planReviewWaitingSince time.Time
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
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/jobs/%d", server.URL, id), nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			err = json.NewDecoder(response.Body).Decode(&details)
			_ = response.Body.Close()
			if err != nil || response.StatusCode != http.StatusOK {
				t.Fatalf("observe terminal job: status=%d error=%v", response.StatusCode, err)
			}
			currentText, next, readErr := transcript.since(cursor)
			if readErr != nil {
				t.Fatal(readErr)
			}
			cursor = next
			activity.observe(t, currentText)
			planReviewDisplayed = planReviewDisplayed || strings.Contains(currentText, "PLAN REVIEW")
			switch details.Job.Status {
			case model.JobStatusFailed, model.JobStatusCanceled:
				questions := []string{}
				for _, step := range details.Steps {
					if step.Status == model.StepStatusWaiting {
						questions = append(questions, step.Error)
					}
				}
				var assignment json.RawMessage
				_ = runtime.pool.QueryRow(ctx, `SELECT jsonb_build_object('role',payload_json->>'role','task',payload_json->'assignment'->'task') FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'phase'='call_opened' ORDER BY id DESC LIMIT 1`, details.Job.ID).Scan(&assignment)
				t.Fatalf("ordinary terminal job %d stopped as %s: %s; questions=%v; assignment=%s", details.Job.ID, details.Job.Status, details.Job.Error, questions, assignment)
			case model.JobStatusWaiting:
				if !terminalWaitingPlanReview(details) {
					questions := []string{}
					for _, step := range details.Steps {
						if step.Status == model.StepStatusWaiting {
							questions = append(questions, step.Error)
						}
					}
					t.Fatalf("ordinary terminal job %d stopped outside owner ledger review: %s; questions=%v", details.Job.ID, details.Job.Error, questions)
				}
				if planReviewWaitingSince.IsZero() {
					planReviewWaitingSince = time.Now()
				}
				if !planReviewDisplayed && time.Since(planReviewWaitingSince) > 15*time.Second {
					t.Fatalf("terminal did not display authoritative ledger review:\n%s", terminalTranscriptTail(transcript.text(), 16*1024))
				}
				if !approvedPlan && planReviewDisplayed {
					approveAndFreezeTerminalLedger(t, ctx, input, transcript, server.URL, root, workspaceIdentity, details.Job.ID)
					approvedPlan = true
				}
			}
			return details.Job.Status == model.JobStatusCompleted
		})
		catchupDeadline := time.Now().Add(10 * time.Second)
		wait(func() bool {
			if time.Now().After(catchupDeadline) {
				t.Fatal("completed terminal job did not display its retained planning facts and written-files recap within the catch-up budget")
			}
			currentText, next, err := transcript.since(cursor)
			if err != nil {
				t.Fatal(err)
			}
			cursor = next
			activity.observe(t, currentText)
			if !activity.seen["Written files:"] {
				return false
			}
			return true
		})
		activity.assert(t)
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

func terminalTranscriptTail(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}

func terminalWaitingPlanReview(details model.JobDetails) bool {
	waiting := 0
	for _, step := range details.Steps {
		if step.Generation == details.Job.CurrentGeneration && step.SupersededAtGeneration == nil &&
			step.Action == "v3_coding_plan" && step.Status == model.StepStatusWaiting {
			waiting++
		}
	}
	return waiting == 1
}

func approveAndFreezeTerminalLedger(
	t *testing.T,
	ctx context.Context,
	input io.Writer,
	transcript *terminalTranscript,
	serverURL, workspaceRoot, workspaceIdentity string,
	jobID int64,
) {
	t.Helper()
	query := url.Values{"workspace_root": {workspaceRoot}, "workspace_identity": {workspaceIdentity}}
	readLedger := func() agentGauntletLedgerRevision {
		var result agentGauntletLedgerRevision
		agentGauntletJSON(t, ctx, http.MethodGet,
			fmt.Sprintf("%s/v1/jobs/%d/plan?%s", serverURL, jobID, query.Encode()), nil, &result)
		return result
	}
	plan := readLedger()
	items := make([]model.LedgerItem, 0)
	for _, item := range agentGauntletLedgerItems(plan.Ledger) {
		if item.Disposition == model.LedgerDispositionActive {
			items = append(items, item)
		}
	}
	if plan.State != "proposed" || len(items) == 0 {
		t.Fatalf("terminal ledger is not reviewable: %+v", plan)
	}
	for index, item := range items {
		if item.ReviewState != model.LedgerReviewAgreed {
			priorRevision := plan.Revision
			if _, err := io.WriteString(input, " "); err != nil {
				t.Fatal(err)
			}
			plan = waitForTerminalLedger(t, ctx, 15*time.Second, readLedger, func(current agentGauntletLedgerRevision) bool {
				if current.Revision != priorRevision+1 {
					return false
				}
				for _, currentItem := range agentGauntletLedgerItems(current.Ledger) {
					if currentItem.ID == item.ID {
						return currentItem.ReviewState == model.LedgerReviewAgreed
					}
				}
				return false
			})
		}
		if index+1 < len(items) {
			cursor := transcript.position()
			if _, err := io.WriteString(input, "\x1b[B"); err != nil {
				t.Fatal(err)
			}
			waitForTerminalText(t, ctx, transcript, cursor, fmt.Sprintf("> %d. [", index+2))
		}
	}
	confirmationCursor := transcript.position()
	if _, err := io.WriteString(input, "\r"); err != nil {
		t.Fatal(err)
	}
	waitForTerminalText(t, ctx, transcript, confirmationCursor, "Freeze this work set:")
	priorRevision := plan.Revision
	freezeCursor := transcript.position()
	if _, err := io.WriteString(input, "\r"); err != nil {
		t.Fatal(err)
	}
	frozen := waitForTerminalLedger(t, ctx, 6*time.Minute, readLedger, func(current agentGauntletLedgerRevision) bool {
		return current.State == "frozen" && current.Revision == priorRevision+1
	})
	waitForTerminalText(t, ctx, transcript, freezeCursor, "plan frozen; coding started")
	t.Logf("approved %d displayed ledger items through the PTY and froze revision %d", len(items), frozen.Revision)
}

func waitForTerminalLedger(
	t *testing.T,
	ctx context.Context,
	timeout time.Duration,
	read func() agentGauntletLedgerRevision,
	done func(agentGauntletLedgerRevision) bool,
) agentGauntletLedgerRevision {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		current := read()
		if done(current) {
			return current
		}
		select {
		case <-ctx.Done():
			t.Fatalf("terminal ledger transition did not complete: %v", ctx.Err())
		case <-deadline.C:
			t.Fatalf("terminal ledger transition did not complete within %s", timeout)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func waitForTerminalText(
	t *testing.T,
	ctx context.Context,
	transcript *terminalTranscript,
	cursor int64,
	needle string,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		text, _, err := transcript.since(cursor)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text, needle) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("terminal did not render %q:\n%s", needle, terminalTranscriptTail(text, 16*1024))
		}
		select {
		case <-ctx.Done():
			t.Fatalf("terminal did not render %q: %v", needle, ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
}

type terminalTranscript struct {
	mu    sync.Mutex
	bytes []byte
	total int64
}

func (transcript *terminalTranscript) Write(value []byte) (int, error) {
	transcript.mu.Lock()
	defer transcript.mu.Unlock()
	transcript.bytes = append(transcript.bytes, value...)
	transcript.total += int64(len(value))
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

func (transcript *terminalTranscript) position() int64 {
	transcript.mu.Lock()
	defer transcript.mu.Unlock()
	return transcript.total
}

func (transcript *terminalTranscript) since(cursor int64) (string, int64, error) {
	transcript.mu.Lock()
	defer transcript.mu.Unlock()
	start := transcript.total - int64(len(transcript.bytes))
	if cursor < start || cursor > transcript.total {
		return "", transcript.total, fmt.Errorf("terminal activity cursor lost bounded evidence")
	}
	return string(transcript.bytes[cursor-start:]), transcript.total, nil
}
