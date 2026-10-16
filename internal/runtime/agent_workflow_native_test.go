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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
)

// An ordinary request goes through production configuration, API, scheduler,
// intake, owner approval of the exact generated ledger, native agents and
// completion. No plan content or feedback is injected, and holdout evaluation
// starts only after the producer terminates.
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
	workspaceIdentity, err := projectroot.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	workers := make(chan error, 1)
	go func() { defer close(workers); workers <- runtime.worker.Start(runtime.context) }()
	defer func() { runtime.cancel(); <-workers }()
	request := map[string]any{"metadata": map[string]string{"client_cwd": root}, "instruction": "Build a Python module temperature.py with celsius_to_fahrenheit(value) and fahrenheit_to_celsius(value), using only the Python standard library for both the implementation and its automated tests. Each function accepts finite int or float values, returns a float, and raises ValueError for NaN/infinity or TypeError for booleans and nonnumbers. For finite inputs, return a finite float when the mathematical result is within finite float range; otherwise return infinity with the result's sign rather than raising an overflow error. Include automated tests and a short README showing usage."}
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
	approvedPlan := false
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
		if details.Job.Status == model.JobStatusWaiting && !approvedPlan {
			approveAndFreezeGeneratedLedger(t, ctx, server.URL, root, workspaceIdentity, receipt.Job.ID)
			approvedPlan = true
			continue
		}
		if details.Job.Status == model.JobStatusFailed || details.Job.Status == model.JobStatusWaiting || details.Job.Status == model.JobStatusCanceled {
			var assignment json.RawMessage
			_ = runtime.pool.QueryRow(ctx, `SELECT jsonb_build_object('role',payload_json->>'role','task',payload_json->'assignment'->'task') FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'phase'='call_opened' ORDER BY id DESC LIMIT 1`, receipt.Job.ID).Scan(&assignment)
			var rounds json.RawMessage
			_ = runtime.pool.QueryRow(ctx, `
				SELECT COALESCE(jsonb_agg(jsonb_build_object(
					'phase',payload_json->>'phase','task_id',payload_json->>'task_id',
					'round',payload_json->'round','findings',payload_json->'findings',
					'checks',payload_json->'checks'
				) ORDER BY id),'[]'::jsonb)
				FROM evidence
				WHERE job_id=$1 AND kind='coding_agent'
				  AND payload_json->>'phase' IN ('task_executing','task_reviewed','task_accepted')
			`, receipt.Job.ID).Scan(&rounds)
			t.Fatalf("unsteered producer stopped as %s: %s; steps=%+v; assigned responsibility=%s; task rounds=%s", details.Job.Status, details.Job.Error, details.Steps, assignment, rounds)
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
assert c(1e308)==math.inf and c(-1e308)==-math.inf
for fn in (c,f):
 for v in (10**309,-10**309):
  result=fn(v)
  assert type(result) is float and result==(math.inf if v>0 else -math.inf)
 for v in (True,False,"20",None,[],{}):
  try: fn(v)
  except TypeError: pass
  else: raise AssertionError("accepted a nonnumber")
 for v in (float("nan"),float("inf"),-float("inf")):
  try: fn(v)
  except ValueError: pass
  else: raise AssertionError("accepted a nonfinite number")
print("holdout passed")`
	holdoutCommand := exec.Command("python3", "-B", "-c", holdout)
	holdoutCommand.Dir = root
	if output, err := holdoutCommand.CombinedOutput(); err != nil {
		t.Fatalf("stopped producer failed independent holdout: %s %v", output, err)
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

func approveAndFreezeGeneratedLedger(
	t *testing.T,
	ctx context.Context,
	serverURL string,
	workspaceRoot string,
	workspaceIdentity string,
	jobID int64,
) {
	t.Helper()
	query := url.Values{"workspace_root": {workspaceRoot}, "workspace_identity": {workspaceIdentity}}
	var plan agentGauntletLedgerRevision
	agentGauntletJSON(t, ctx, http.MethodGet,
		fmt.Sprintf("%s/v1/jobs/%d/plan?%s", serverURL, jobID, query.Encode()), nil, &plan)
	items := agentGauntletLedgerItems(plan.Ledger)
	if plan.State != "proposed" || len(items) == 0 {
		t.Fatalf("generated ledger is not reviewable: %+v", plan)
	}
	for _, item := range items {
		if item.Disposition != model.LedgerDispositionActive || item.ReviewState != model.LedgerReviewProposed {
			continue
		}
		operationID, err := model.NewLifecycleOperationID()
		if err != nil {
			t.Fatal(err)
		}
		priorRevision := plan.Revision
		var revision agentGauntletLedgerRevision
		agentGauntletJSON(t, ctx, http.MethodPost, fmt.Sprintf("%s/v1/jobs/%d/plan/decisions", serverURL, jobID), map[string]any{
			"operation_id": string(operationID), "owner_turn_id": "gauntlet-" + string(operationID),
			"owner_turn_text": fmt.Sprintf("Set ledger item %s review state to agreed.", item.ID), "revision": priorRevision,
			"workspace_root": workspaceRoot, "workspace_identity": workspaceIdentity,
			"action": codingagent.DirectPlanningAction{Kind: model.LedgerMutationReview, TargetID: item.ID,
				ReviewState: model.LedgerReviewAgreed, Rationale: "explicit gauntlet owner approval"},
		}, &revision)
		if revision.BaseRevision != priorRevision || revision.Revision != priorRevision+1 || revision.State != "proposed" {
			t.Fatalf("ledger approval changed authority unexpectedly: %+v", revision)
		}
		plan.Revision = revision.Revision
	}
	operationID, err := model.NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	var frozen agentGauntletLedgerRevision
	agentGauntletJSON(t, ctx, http.MethodPost, fmt.Sprintf("%s/v1/jobs/%d/plan/freeze", serverURL, jobID), map[string]any{
		"operation_id": string(operationID), "owner_turn_id": "gauntlet-" + string(operationID),
		"owner_turn_text": "Sign off the exact displayed planning revision.", "revision": plan.Revision,
		"workspace_root": workspaceRoot, "workspace_identity": workspaceIdentity,
	}, &frozen)
	if frozen.BaseRevision != plan.Revision || frozen.Revision != plan.Revision+1 || frozen.State != "frozen" {
		t.Fatalf("ledger freeze changed authority unexpectedly: %+v", frozen)
	}
	t.Logf("approved %d generated ledger items and froze revision %d", len(items), frozen.Revision)
}

type agentGauntletLedgerRevision struct {
	JobID        int64                   `json:"job_id"`
	Revision     int64                   `json:"revision"`
	BaseRevision int64                   `json:"base_revision"`
	State        string                  `json:"state"`
	Ledger       model.EngineeringLedger `json:"ledger"`
}

func agentGauntletLedgerItems(ledger model.EngineeringLedger) []model.LedgerItem {
	items := make([]model.LedgerItem, 0)
	for _, value := range ledger.Objectives {
		items = append(items, value.LedgerItem)
	}
	for _, value := range ledger.Requirements {
		items = append(items, value.LedgerItem)
	}
	for _, value := range ledger.Constraints {
		items = append(items, value.LedgerItem)
	}
	for _, value := range ledger.Groups {
		items = append(items, value.LedgerItem)
	}
	for _, value := range ledger.Tasks {
		items = append(items, value.LedgerItem)
	}
	for _, value := range ledger.AcceptanceCriteria {
		items = append(items, value.LedgerItem)
	}
	for _, value := range ledger.Checks {
		items = append(items, value.LedgerItem)
	}
	for _, value := range ledger.Dependencies {
		items = append(items, value.LedgerItem)
	}
	return items
}

func agentGauntletJSON(t *testing.T, ctx context.Context, method, requestURL string, payload, destination any) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s returned %d: %s", method, requestURL, response.StatusCode, message)
	}
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatal(err)
	}
}
