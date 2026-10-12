package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

// An ordinary request goes through production configuration, API, scheduler,
// intake, native agents and completion. No intermediate plan or feedback is
// injected, and holdout evaluation starts only after the producer terminates.
func TestInstalledAgentWorkflowOrdinaryRequest(t *testing.T) {
	command, modelName, databaseURL := os.Getenv("OMNI_TEST_CODEX_COMMAND"), os.Getenv("OMNI_TEST_CODEX_MODEL"), os.Getenv("OMNI_TEST_DATABASE_URL")
	if command == "" || modelName == "" || databaseURL == "" {
		t.Skip("isolated PostgreSQL and installed Codex qualification not selected")
	}
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(repository, "bin", "agent-qualification")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, "request-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	for _, field := range modelconfig.RegisteredFields() {
		for _, key := range field.EnvKeys {
			t.Setenv(key, modelName)
		}
	}
	for key, value := range map[string]string{"DATABASE_URL": databaseURL, "DATABASE_SCHEMA": fmt.Sprintf("agent_native_%d", time.Now().UnixNano()), "LLM_PROVIDER": "codex", "CODEX_COMMAND": command, "CODEX_DIRECTORY": repository, "HOST_DIRECTORY_ACCESS_ROOT": root, "WORKER_POLL_INTERVAL": "50ms", "REDIS_URL": "", "LISTEN_ADDR": "127.0.0.1:0"} {
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
	go func() { defer close(workers); workers <- runtime.worker.Start(runtime.context) }()
	defer func() { runtime.cancel(); <-workers }()
	request := map[string]any{"coding_mode": "agents", "metadata": map[string]string{"client_cwd": root}, "instruction": "Build a dependency-free Python module temperature.py with celsius_to_fahrenheit(value) and fahrenheit_to_celsius(value). Each accepts finite int or float values, returns a float, and raises ValueError for NaN/infinity or TypeError for booleans and nonnumbers. Include automated tests and a short README showing usage."}
	body, _ := json.Marshal(request)
	response, err := http.Post(server.URL+"/v1/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Job model.Job `json:"job"`
	}
	decodeErr := json.NewDecoder(response.Body).Decode(&receipt)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated || decodeErr != nil || receipt.Job.ID < 1 {
		t.Fatalf("ordinary request not accepted: status=%d error=%v", response.StatusCode, decodeErr)
	}
	t.Logf("accepted ordinary native request as job %d", receipt.Job.ID)
	var details model.JobDetails
	for ctx.Err() == nil {
		select {
		case err := <-workers:
			t.Fatalf("production worker stopped before completion: %v", err)
		default:
		}
		response, err := http.Get(fmt.Sprintf("%s/v1/jobs/%d", server.URL, receipt.Job.ID))
		if err != nil {
			t.Fatal(err)
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&details)
		_ = response.Body.Close()
		if response.StatusCode != 200 || decodeErr != nil {
			t.Fatalf("job observation failed: %d %v", response.StatusCode, decodeErr)
		}
		if details.Job.Status == model.JobStatusCompleted {
			break
		}
		if details.Job.Status == model.JobStatusFailed || details.Job.Status == model.JobStatusWaiting || details.Job.Status == model.JobStatusCanceled {
			var assignment json.RawMessage
			_ = runtime.pool.QueryRow(ctx, `SELECT jsonb_build_object('role',payload_json->>'role','task',payload_json->'assignment'->'task') FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'phase'='call_opened' ORDER BY id DESC LIMIT 1`, receipt.Job.ID).Scan(&assignment)
			t.Fatalf("unsteered producer stopped as %s: %s; steps=%+v; assigned responsibility=%s", details.Job.Status, details.Job.Error, details.Steps, assignment)
		}
		select {
		case <-ctx.Done():
		case <-time.After(250 * time.Millisecond):
		}
	}
	if details.Job.Status != model.JobStatusCompleted {
		t.Fatalf("native request did not terminate: %v", ctx.Err())
	}
	var calls int
	if err := runtime.pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'phase'='call_completed'`, receipt.Job.ID).Scan(&calls); err != nil || calls < 4 {
		t.Fatalf("native roles lack original retained receipts: calls=%d error=%v", calls, err)
	}
	_, client, err := agentProviders(cfg)()
	if err != nil {
		t.Fatal(err)
	}
	files := []string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	holdout := `import math,runpy
from fractions import Fraction
m=runpy.run_path("temperature.py"); c=m["celsius_to_fahrenheit"]; f=m["fahrenheit_to_celsius"]
assert type(c(0)) is float and type(f(32)) is float
for v in (-123.5,-40,0,37,100,500.25):
 assert math.isclose(c(v),v*9/5+32,rel_tol=1e-12,abs_tol=1e-10)
 assert math.isclose(f(v),(v-32)*5/9,rel_tol=1e-12,abs_tol=1e-10)
 assert math.isclose(f(c(v)),v,rel_tol=1e-12,abs_tol=1e-10)
assert math.isclose(f(2*10**308),float(Fraction(2*10**308-32)*Fraction(5,9)),rel_tol=1e-12)
assert math.isclose(f(1e308),1e308*(5/9),rel_tol=1e-12)
for fn in (c,f):
 for v in (True,False,"20",None,[],{}):
  try: fn(v)
  except TypeError: pass
  else: raise AssertionError("accepted a nonnumber")
 for v in (float("nan"),float("inf"),-float("inf")):
  try: fn(v)
  except ValueError: pass
  else: raise AssertionError("accepted a nonfinite number")
print("holdout passed")`
	checks, err := client.ExecuteChecks(ctx, codingagent.Assignment{JobID: receipt.Job.ID, Generation: details.Job.CurrentGeneration, Role: codingagent.OverallReview, Root: root, ReadFiles: files, Route: codingagent.Route{Provider: "codex", Model: modelName}}, []codingagent.Check{{ID: "independent-holdout", Argv: []string{"python3", "-B", "-c", holdout}}})
	if err != nil || len(checks) != 1 || checks[0].ExitCode != 0 {
		t.Fatalf("stopped producer failed holdout: %+v %v", checks, err)
	}
	t.Logf("ordinary production workflow completed with %d native calls; independent holdout passed", calls)
	// Restart reuses the same persisted schema and must not regenerate completion.
	second, err := New(ctx, cfg, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var retainedCalls int
	if err := second.pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'phase'='call_completed'`, receipt.Job.ID).Scan(&retainedCalls); err != nil || retainedCalls != calls {
		t.Fatalf("restart changed durable completion evidence: %d %v", retainedCalls, err)
	}
}
