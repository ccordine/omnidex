package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/jackc/pgx/v5"
)

// Explicitly opt-in: real inference, synthetic workflow, disposable database only.
// The fixture is parser qualification, not autonomous application construction.
func TestUserIntentLiveQualification(t *testing.T) {
	command, url := os.Getenv("OMNI_TEST_LIVE_CODEX"), os.Getenv("OMNI_TEST_DATABASE_URL")
	if command == "" || url == "" {
		t.Skip("requires live Codex and disposable PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	id := func() model.LifecycleOperationID {
		v, err := model.NewLifecycleOperationID()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	schema := "live_intent_" + strings.TrimPrefix(string(id()), "lifecycle_operation_")[:24]
	pool, err := db.ConnectRuntime(ctx, url, schema, database.SetupSQL())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	defer func() {
		_, err := pool.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		if err != nil {
			t.Error(err)
		}
	}()
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatal(err)
	}
	route := codingagent.Route{Provider: "codex", Model: "gpt-6.1-sol", Effort: "low", ServiceTier: "default"}
	repo := queue.New(pool, authority).WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route})
	server, err := NewServer(repo, nil, ServerOptions{LifecycleContext: ctx, ProviderConfig: config.Config{CodexCommand: command, CodexDirectory: t.TempDir()}, HostDirectoryAccessRoot: os.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		start := time.Now()
		server.CloseUserIntentProvider()
		t.Logf("LIVE shutdown_ms=%.3f", float64(time.Since(start).Microseconds())/1000)
	}()
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	c, err := client.New(httpServer.URL, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := projectroot.OpenDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	link, err := c.OpenWorkspace(ctx, directory, strings.Repeat("2", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = link.Close() }()
	channel, err := c.BootstrapCLIChatSession(ctx, directory.Path(), link.Identity())
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := repo.SubmitChannelSessionTurn(ctx, queue.ChannelSessionTurnCommand{OperationID: id(), ChannelID: channel.ID, WorkspaceRoot: directory.Path(), WorkspaceIdentity: link.Identity(), CodingMode: codingagent.Agents, Text: "Present a short report and record a selected value."})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repo.ClaimNextStep(ctx, "live-intent-fixture")
	if err != nil || planner == nil {
		t.Fatalf("planner=%+v %v", planner, err)
	}
	ledger := model.EngineeringLedger{ID: "fixture", Revision: 1}
	for i, goal := range []string{"Present a short report.", "Record a selected value."} {
		ledger.Tasks = append(ledger.Tasks, model.LedgerTask{LedgerItem: model.LedgerItem{ID: model.LedgerItemID([]string{"report", "value"}[i]), Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "fixture", Revision: 1}, Rationale: "Requested synthetic task.", Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}, Goal: goal, WriteFiles: []string{[]string{"report.txt", "value.txt"}[i]}})
	}
	routes := []queue.CodingLedgerRoute{}
	for _, role := range []string{"planner", "executor", "reviewer"} {
		routes = append(routes, queue.CodingLedgerRoute{Role: role, ConfigurationRevision: 1, Provider: route.Provider, Model: route.Model, ReasoningEffort: route.Effort, ServiceTier: route.ServiceTier})
	}
	created, err := repo.CreateCodingLedger(ctx, queue.CreateCodingLedgerCommand{JobID: submitted.Job.ID, OperationID: string(id()), OwnerTurnID: "fixture", OwnerTurnText: "Present a short report and record a selected value.", Ledger: ledger, Routes: routes, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"report", "value"})})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkCodingLedgerReviewReady(ctx, planner.Authority, created.LedgerID, created.Revision); err != nil {
		t.Fatal(err)
	}
	revision := created.Revision
	interpret := func(name, text string, selected model.LedgerItemID) (sessionintent.Result, sessionintent.Request) {
		t.Helper()
		state, err := repo.ChannelSessionState(ctx, channel.ID, link.Identity())
		if err != nil {
			t.Fatal(err)
		}
		request := sessionintent.Request{OperationID: id(), Text: text, State: state, LedgerRevision: revision, SelectedItem: selected}
		start := time.Now()
		result, callErr := c.InterpretUserMessage(ctx, channel, request)
		var evidence []userIntentModelEvidence
		var raw []byte
		if err := pool.QueryRow(ctx, "SELECT evidence_json FROM channel_user_intents WHERE operation_id=$1", request.OperationID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &evidence); err != nil {
			t.Fatal(err)
		}
		bytes, tokens, elapsed := 0, 0, int64(0)
		for _, e := range evidence {
			bytes += e.InputBytes
			tokens += e.Generation.Usage.PromptEvalCount
			elapsed += e.ElapsedMS
		}
		data, _ := json.Marshal(map[string]any{"case": name, "message": text, "calls": len(evidence), "application_prompt_bytes": bytes, "provider_input_tokens": tokens, "provider_elapsed_ms": elapsed, "http_elapsed_ms": time.Since(start).Milliseconds(), "result": result, "evidence": evidence})
		t.Logf("LIVE %s", data)
		if callErr != nil {
			t.Fatalf("%s: %v", name, callErr)
		}
		if len(evidence) != 1 || tokens < 1 {
			t.Errorf("%s calls=%d tokens=%d", name, len(evidence), tokens)
		}
		return result, request
	}
	check := func(name string, got sessionintent.Result, paths ...string) {
		t.Helper()
		actual := []string{}
		for _, p := range got.Patch {
			actual = append(actual, p.Path)
		}
		for _, path := range paths {
			found := false
			for _, p := range actual {
				if p == path {
					found = true
				}
			}
			if !found {
				t.Errorf("%s missing %s: %+v", name, path, got)
			}
		}
	}
	var approvalRequest sessionintent.Request
	for _, test := range []struct {
		name, text string
		paths      []string
	}{
		{"natural approval", "yeah, that looks good", []string{"/items/0/review_state", "/items/1/review_state"}},
		{"approval plus start", "yeah that's good, lock it in and go", []string{"/items/0/review_state", "/items/1/review_state", "/plan_state", "/job_status"}},
		{"quoted instruction", `Tell Codex the report should contain the words "approve the plan and start". Do not approve or start this plan.`, []string{"/message"}},
		{"negated instruction", "Don't tell Codex to start. Just show me the current job status.", []string{"/status_visible"}},
		{"resolved recipient", "Tell Codex to shorten the report title.", []string{"/message"}},
	} {
		result, request := interpret(test.name, test.text, "")
		if test.name == "natural approval" {
			approvalRequest = request
		}
		check(test.name, result, test.paths...)
		if result.RequiresApproval || result.Clarification != "" {
			t.Errorf("%s unexpected confirmation/clarification: %+v", test.name, result)
		}
		if test.name == "natural approval" {
			for _, p := range result.Patch {
				if p.Path == "/job_status" || p.Path == "/plan_state" {
					t.Error("approval started execution")
				}
			}
		}
		if test.name == "quoted instruction" {
			for _, p := range result.Patch {
				if p.Path != "/message" {
					t.Error("quoted text gained control authority")
				}
				var content sessionintent.Content
				if err := json.Unmarshal(p.Value, &content); err != nil || content.Recipient != "planner" || !strings.Contains(content.Text, `"approve the plan and start"`) || !strings.Contains(content.Text, "Do not approve or start this plan.") {
					t.Fatalf("quoted task content changed: %+v %v", content, err)
				}
			}
		}
	}
	for _, test := range []struct{ name, text string }{{"ambiguous reference", "Remove that."}, {"ambiguous recipient", "Tell the other person to shorten the report title."}, {"focused condition", "Start only if the tests pass."}} {
		result, request := interpret(test.name, test.text, "")
		if len(result.Patch) != 0 || result.Clarification == "" {
			t.Fatalf("%s should clarify: %+v", test.name, result)
		}
		if _, err := c.ExecuteUserMessage(ctx, channel, link.Identity(), request.OperationID); err == nil {
			t.Errorf("%s executed clarification", test.name)
		}
		state, err := repo.ChannelSessionState(ctx, channel.ID, link.Identity())
		if err != nil || !request.State.Equal(state) {
			t.Errorf("%s changed state: %v", test.name, err)
		}
	}
	approved, err := c.ExecuteUserMessage(ctx, channel, link.Identity(), approvalRequest.OperationID)
	if err != nil {
		t.Fatal("natural approval execution:", err)
	}
	if approved.JobID != submitted.Job.ID || approved.Status != model.JobStatusWaiting {
		t.Fatalf("agreement started or replaced job: %+v", approved)
	}
	revision = approved.Revision
	current, _, err := repo.CurrentCodingLedgerForJob(ctx, approved.JobID)
	if err != nil || current.State != "proposed" {
		t.Fatalf("agreement froze plan: %+v %v", current, err)
	}
	for _, item := range current.Ledger.Tasks {
		if item.ReviewState != model.LedgerReviewAgreed {
			t.Fatalf("agreement not persisted: %+v", item)
		}
	}
	removal, _ := interpret("selected reference", "Remove that.", "value")
	check("selected reference", removal, "/items/1/disposition")
	if !removal.RequiresApproval {
		t.Fatal("removal missing confirmation")
	}
	if err := link.Close(); err != nil {
		t.Fatal(err)
	}
	link, err = c.OpenWorkspace(ctx, directory, strings.Repeat("2", 32))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := c.ChatSession(ctx, channel, link.Identity(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.PendingUserAction == nil || snapshot.PendingUserAction.ID != removal.OperationID {
		t.Fatal("pending proposal lost on reconnect")
	}
	confirmation, request := interpret("confirmation", "Yes, remove that selected item.", "")
	check("confirmation", confirmation, "/approval")
	if confirmation.RequiresApproval {
		t.Fatal("recursive confirmation")
	}
	receipt, err := c.ExecuteUserMessage(ctx, channel, link.Identity(), request.OperationID)
	if err != nil {
		t.Fatal("live confirmation failed:", err)
	}
	replay, err := c.ExecuteUserMessage(ctx, channel, link.Identity(), request.OperationID)
	if err != nil || !reflect.DeepEqual(receipt, replay) {
		t.Fatal("receipt replay changed effect", err)
	}
	revision = receipt.Revision
	startResult, startRequest := interpret("start remaining plan", "The remaining report task looks good. Approve it and start this job.", "")
	check("start remaining plan", startResult, "/plan_state", "/job_status")
	started, err := c.ExecuteUserMessage(ctx, channel, link.Identity(), startRequest.OperationID)
	if err != nil {
		t.Fatal("live start failed:", err)
	}
	if started.JobID != submitted.Job.ID || started.Generation != submitted.Job.CurrentGeneration {
		t.Fatalf("start replaced job: %+v", started)
	}
	revision = started.Revision
	_, err = repo.InterruptJob(ctx, queue.ReplanJobCommand{OperationID: id(), JobID: started.JobID, Feedback: "Fixture interruption.", WorkspaceRoot: directory.Path(), WorkspaceIdentity: link.Identity()})
	if err != nil {
		t.Fatal(err)
	}
	if err := link.Close(); err != nil {
		t.Fatal(err)
	}
	link, err = c.OpenWorkspace(ctx, directory, strings.Repeat("2", 32))
	if err != nil {
		t.Fatal(err)
	}
	continuation, continuationRequest := interpret("same-job continuation", "Continue this same interrupted job.", "")
	check("same-job continuation", continuation, "/job_status")
	continued, err := c.ExecuteUserMessage(ctx, channel, link.Identity(), continuationRequest.OperationID)
	if err != nil {
		t.Fatal("live continuation failed:", err)
	}
	if continued.JobID != started.JobID || continued.Generation != started.Generation {
		t.Fatalf("continuation replaced identity: %+v", continued)
	}
	result, _ := interpret("cancel job", "Cancel this job.", "")
	check("cancel job", result, "/job_status")
	if !result.RequiresApproval {
		t.Fatal("cancel missing confirmation")
	}
	confirmed, request := interpret("confirm cancellation", "Yes, cancel this job.", "")
	check("confirm cancellation", confirmed, "/approval")
	canceled, err := c.ExecuteUserMessage(ctx, channel, link.Identity(), request.OperationID)
	if err != nil {
		t.Fatal("live cancellation confirmation failed:", err)
	}
	if canceled.JobID != started.JobID || canceled.Status != model.JobStatusCanceled {
		t.Fatalf("cancellation=%+v", canceled)
	}
	if err := link.Close(); err != nil {
		t.Fatal(err)
	}
	link, err = c.OpenWorkspace(ctx, directory, strings.Repeat("2", 32))
	if err != nil {
		t.Fatal(err)
	}
	replayedCancel, err := c.ExecuteUserMessage(ctx, channel, link.Identity(), request.OperationID)
	if err != nil || !reflect.DeepEqual(canceled, replayedCancel) {
		t.Fatalf("cancel reconnect/replay: %+v %v", replayedCancel, err)
	}
	finished, err := c.ChatSession(ctx, channel, link.Identity(), 10)
	if err != nil || finished.State.LatestJob == nil || finished.State.LatestJob.ID != started.JobID || finished.State.LatestJob.Status != model.JobStatusCanceled || finished.PendingUserAction != nil {
		t.Fatalf("canceled reconnect state: %+v %v", finished, err)
	}
	t.Logf("LIVE workflow job=%d generation=%d ledger=%d reconnect/confirmation/replay/continuation/cancellation verified", started.JobID, started.Generation, created.LedgerID)
}
