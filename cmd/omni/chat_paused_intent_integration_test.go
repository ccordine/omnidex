package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/api"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
	"github.com/jackc/pgx/v5"
)

// Real CLI input, client, HTTP, semantic service and PostgreSQL. The provider is
// scripted: this proves binding and dispatch, not live-model accuracy.
func TestPausedPlanningOrdinaryUserIntent(t *testing.T) {
	testPausedPlanningUserIntent(t, false)
}

func TestPausedPlanningLiveUserIntent(t *testing.T) {
	if os.Getenv("OMNI_TEST_LIVE_CODEX") == "" {
		t.Skip("requires real native parser")
	}
	testPausedPlanningUserIntent(t, true)
}

func testPausedPlanningUserIntent(t *testing.T, live bool) {
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("isolated OMNI_TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	operation := func() model.LifecycleOperationID {
		id, err := model.NewLifecycleOperationID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	schema := "paused_intent_" + strings.TrimPrefix(string(operation()), "lifecycle_operation_")[:32]
	pool, err := db.ConnectRuntime(ctx, url, schema, database.SetupSQL())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	defer func() {
		if _, err := pool.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatal(err)
	}
	route := codingagent.Route{Provider: "codex", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
	repo := queue.New(pool, authority).WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: codingagent.Route{Provider: "ollama", Model: "fixture-parser"}})
	const text = "What is the current Omnidex job status?"
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if !strings.Contains(request.Prompt, text) {
			t.Error("exact owner text absent from semantic prompt")
		}
		if strings.Contains(request.Prompt, "Approve the entire exact displayed plan") || strings.Contains(request.Prompt, "Remove displayed item") {
			t.Error("missing display exposed ledger authority")
		}
		answer := `{"patch":[{"op":"replace","path":"/status_visible","value":true}]}`
		t.Logf("scripted semantic prompt=%q response=%q", request.Prompt, answer)
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "fixture-parser", "created_at": time.Now().UTC().Format(time.RFC3339Nano), "response": answer, "done": true, "done_reason": "stop", "prompt_eval_count": 128, "eval_count": 1})
	}))
	defer provider.Close()
	providerConfig := config.Config{OllamaBaseURL: provider.URL}
	wantCalls := 1
	if live {
		configuration := codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: codingagent.Route{Provider: "codex", Model: "gpt-6.1-sol", Effort: "low", ServiceTier: "default"}}
		repo.WithCodingAgents(configuration)
		providerConfig.CodexCommand = os.Getenv("OMNI_TEST_LIVE_CODEX")
		providerConfig.CodexDirectory = t.TempDir()
		wantCalls = 0 // No scripted provider request is allowed in live mode.
	}
	server, err := api.NewServer(repo, nil, api.ServerOptions{LifecycleContext: ctx, ProviderConfig: providerConfig, HostDirectoryAccessRoot: os.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer server.CloseUserIntentProvider()
	var observed sessionintent.Request
	handler := server.Handler()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/session/intent") {
			var raw json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				t.Error(err)
			}
			if err := json.Unmarshal(raw, &observed); err != nil {
				t.Error(err)
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	apiClient, err := client.New(httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := projectroot.OpenDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	link, err := apiClient.OpenWorkspace(ctx, directory, strings.Repeat("1", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer link.Close()
	channel, err := apiClient.BootstrapCLIChatSession(ctx, directory.Path(), link.Identity())
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := repo.SubmitChannelSessionTurn(ctx, queue.ChannelSessionTurnCommand{OperationID: operation(), ChannelID: channel.ID, WorkspaceRoot: directory.Path(), WorkspaceIdentity: link.Identity(), CodingMode: codingagent.Agents, Text: "Present one synthetic local report."})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		_, err = repo.ReplanJob(ctx, queue.ReplanJobCommand{OperationID: operation(), JobID: submitted.Job.ID, Feedback: "Retain the requested synthetic report.", WorkspaceRoot: directory.Path(), WorkspaceIdentity: link.Identity()})
		if err != nil {
			t.Fatal(err)
		}
	}
	planner, err := repo.ClaimNextStep(ctx, "paused-intent-fixture")
	if err != nil || planner == nil {
		t.Fatalf("planner=%+v %v", planner, err)
	}
	ledger := model.EngineeringLedger{ID: "fixture", Revision: 1, Tasks: []model.LedgerTask{{LedgerItem: model.LedgerItem{ID: "task", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "fixture", Revision: 1}, Rationale: "Requested synthetic report.", Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}, Goal: "Present one synthetic local report.", WriteFiles: []string{"fixture.txt"}}}}
	routes := []queue.CodingLedgerRoute{}
	for _, role := range []string{"planner", "executor", "reviewer"} {
		routes = append(routes, queue.CodingLedgerRoute{Role: role, ConfigurationRevision: 1, Provider: route.Provider, Model: route.Model, ReasoningEffort: route.Effort, ServiceTier: route.ServiceTier})
	}
	created, err := repo.CreateCodingLedger(ctx, queue.CreateCodingLedgerCommand{JobID: submitted.Job.ID, OperationID: string(operation()), OwnerTurnID: "fixture", OwnerTurnText: "Present one synthetic local report.", Ledger: ledger, Routes: routes, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.ApplyCodingLedgerMutation(ctx, queue.ApplyCodingLedgerMutationCommand{LedgerID: created.LedgerID, OwnerTurnText: "Agree with this synthetic task.", Mutation: model.LedgerMutation{OperationID: string(operation()), BaseRevision: 1, OwnerTurnID: "agree", OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: "task", ReviewState: model.LedgerReviewAgreed, Rationale: "Synthetic owner agreement."}}}})
	if err != nil {
		t.Fatal(err)
	}
	paused, err := repo.InterruptJob(ctx, queue.ReplanJobCommand{OperationID: operation(), JobID: submitted.Job.ID, Feedback: "Pause for further direction.", WorkspaceRoot: directory.Path(), WorkspaceIdentity: link.Identity()})
	if err != nil {
		t.Fatal(err)
	}
	if paused.Job.CurrentGeneration != 4 || paused.Job.Status != model.JobStatusWaiting {
		t.Fatalf("wrong paused fixture: %+v", paused.Job)
	}
	var revision int64
	var frozen *int64
	var planState, stepState string
	if err := pool.QueryRow(ctx, `SELECT current_revision,frozen_revision,state FROM coding_ledgers WHERE id=$1`, created.LedgerID).Scan(&revision, &frozen, &planState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM job_steps WHERE job_id=$1 AND generation=4 AND action='v3_coding_plan' AND superseded_at_generation IS NULL`, submitted.Job.ID).Scan(&stepState); err != nil {
		t.Fatal(err)
	}
	if revision != 2 || frozen != nil || planState != "proposed" || stepState != model.StepStatusWaiting {
		t.Fatalf("wrong ledger/planning fixture %d %v %s %s", revision, frozen, planState, stepState)
	}
	snapshot, err := apiClient.ChatSession(ctx, channel, link.Identity(), 10)
	if err != nil {
		t.Fatal(err)
	}
	session := newChatOperationTestSession(t, httpServer.URL)
	console, _ := planReviewTestConsole(t)
	session.renderer = chatRenderer{console: console}
	session.client = apiClient
	session.channel = channel
	session.workspaceIdentity = link.Identity()
	session.serverState = snapshot.State
	session.active = snapshot.ActiveJob
	for _, control := range snapshot.Controls {
		session.controls[control.OperationID] = control
	}
	review := mustPlanReviewState(t, singleLeafPlanReviewFixture(t, submitted.Job.ID, 4))
	session.planReview = &review
	if err := session.reconcilePlanReview(); err != nil {
		t.Fatal(err)
	}
	if !session.currentJobInterrupted() || session.planReview != nil {
		t.Fatal("interruption retained displayed review")
	}
	beforeJob := snapshot.ActiveJob.Job
	beforeLedger, _, err := repo.CurrentCodingLedgerForJob(ctx, submitted.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.acceptUserMessage(text); err != nil {
		t.Fatal(err)
	}
	if observed.Text != text || observed.LedgerRevision != 0 || observed.SelectedItem != "" || calls != wantCalls {
		t.Fatalf("input changed or inference missing: %+v calls=%d", observed, calls)
	}
	if live {
		var evidence []byte
		if err := pool.QueryRow(ctx, `SELECT evidence_json FROM channel_user_intents WHERE operation_id=$1`, observed.OperationID).Scan(&evidence); err != nil {
			t.Fatal(err)
		}
		var leaves []json.RawMessage
		if err := json.Unmarshal(evidence, &leaves); err != nil || len(leaves) != 1 {
			t.Fatalf("live interpretation count=%d error=%v", len(leaves), err)
		}
		t.Logf("LIVE CLI exact-input evidence=%s", evidence)
	}
	receipt, err := apiClient.ExecuteUserMessage(ctx, channel, link.Identity(), observed.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.JobID != submitted.Job.ID || receipt.Generation != 4 || receipt.Revision != 2 || receipt.Status != model.JobStatusWaiting || receipt.Summary != fmt.Sprintf("Job %d: waiting_input.", submitted.Job.ID) {
		t.Fatalf("incorrect status receipt %+v", receipt)
	}
	replay, err := apiClient.ExecuteUserMessage(ctx, channel, link.Identity(), observed.OperationID)
	if err != nil || !reflect.DeepEqual(receipt, replay) {
		t.Fatalf("receipt replay %+v %v", replay, err)
	}
	afterLedger, _, err := repo.CurrentCodingLedgerForJob(ctx, submitted.Job.ID)
	if err != nil || !reflect.DeepEqual(beforeLedger, afterLedger) {
		t.Fatalf("ledger changed %v", err)
	}
	after, err := apiClient.ChatSession(ctx, channel, link.Identity(), 10)
	if err != nil || !reflect.DeepEqual(beforeJob, after.ActiveJob.Job) || !snapshot.State.Equal(after.State) {
		t.Fatalf("job/session changed: before=%+v after=%+v stateBefore=%+v stateAfter=%+v error=%v", beforeJob, after.ActiveJob.Job, snapshot.State, after.State, err)
	}
	for _, patch := range []userintent.Patch{
		{Op: "replace", Path: "/items/0/review_state", Value: json.RawMessage(`"agreed"`)},
		{Op: "replace", Path: "/items/0/disposition", Value: json.RawMessage(`"tombstoned"`)},
		{Op: "replace", Path: "/plan_state", Value: json.RawMessage(`"frozen"`)},
	} {
		request := observed
		request.OperationID = operation()
		_, err := repo.InterpretUserIntent(ctx, request, func(_ context.Context, c sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
			if c.Planning || len(c.Items) != 0 {
				t.Error("undisplayed review authority exposed")
			}
			return sessionintent.Result{OperationID: request.OperationID, Patch: []userintent.Patch{patch}}, json.RawMessage(`[]`), nil
		})
		if err == nil {
			t.Fatalf("missing display authorized %s", patch.Path)
		}
	}
	original := observed
	stale := original
	stale.OperationID = operation()
	stale.LedgerRevision = 1
	if _, err := apiClient.InterpretUserMessage(ctx, channel, stale); !client.IsHTTPStatus(err, http.StatusConflict) {
		t.Fatalf("explicit stale accepted %v", err)
	}
	if calls != wantCalls {
		t.Fatalf("rejected authority invoked provider: %d", calls)
	}
	changed := original
	changed.OperationID = operation()
	if _, err := apiClient.InterpretUserMessage(ctx, channel, changed); err != nil {
		t.Fatal(err)
	}
	_, err = repo.ReplanJob(ctx, queue.ReplanJobCommand{OperationID: operation(), JobID: submitted.Job.ID, ObservedLedgerRevision: 2, Feedback: "Continue the synthetic report.", WorkspaceRoot: directory.Path(), WorkspaceIdentity: link.Identity()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := apiClient.ExecuteUserMessage(ctx, channel, link.Identity(), changed.OperationID); !client.IsHTTPStatus(err, http.StatusConflict) {
		t.Fatalf("changed state executed %v", err)
	}
	t.Logf("exact input=%q; inference calls=%d; receipt=%+v; paused generation4/revision2/no frozen revision preserved; missing-display mutations and stale/state-change execution rejected", text, calls, receipt)
	if live {
		if err := session.reloadSnapshot(); err != nil {
			t.Fatal(err)
		}
		const content = `Tell Codex to keep the report heading "do not approve or start" exactly as written.`
		if err := session.acceptUserMessage(content); err != nil {
			t.Fatal("live CLI task content:", err)
		}
		forwarded, err := apiClient.ExecuteUserMessage(ctx, channel, link.Identity(), observed.OperationID)
		if err != nil || forwarded.JobID != submitted.Job.ID {
			t.Fatalf("task content receipt: %+v %v", forwarded, err)
		}
		consumer, err := repo.ClaimNextStep(ctx, "live-content-consumer")
		if err != nil || consumer == nil || consumer.Job.ID != forwarded.JobID {
			t.Fatalf("message consumer: %+v %v", consumer, err)
		}
		queued, err := repo.NextChannelSessionMessage(ctx, consumer.Authority)
		if err != nil || queued == nil || queued.AdmittedJob.ID != forwarded.JobID || queued.Command.ExpectedGeneration == nil || *queued.Command.ExpectedGeneration != forwarded.Generation {
			t.Fatalf("durable task content: %+v %v", queued, err)
		}
		persisted := queued.Command.Text
		if !strings.Contains(persisted, `"do not approve or start"`) {
			t.Fatalf("task quotation changed: %q", persisted)
		}
		var evidence []byte
		if err := pool.QueryRow(ctx, `SELECT evidence_json FROM channel_user_intents WHERE operation_id=$1`, observed.OperationID).Scan(&evidence); err != nil {
			t.Fatal(err)
		}
		var leaves []json.RawMessage
		if err := json.Unmarshal(evidence, &leaves); err != nil || len(leaves) != 1 {
			t.Fatalf("task content interpretation count=%d %v", len(leaves), err)
		}
		t.Logf("LIVE CLI task content persisted=%q receipt=%+v evidence=%s", persisted, forwarded, evidence)
		if err := session.reloadSnapshot(); err != nil {
			t.Fatal(err)
		}
		beforeRecipient := session.serverState
		if err := session.acceptUserMessage(`Forward "Keep the report title." to the other person.`); err != nil {
			t.Fatal("unresolved recipient:", err)
		}
		var resultJSON, recipientEvidence []byte
		if err := pool.QueryRow(ctx, `SELECT result_json,evidence_json FROM channel_user_intents WHERE operation_id=$1`, observed.OperationID).Scan(&resultJSON, &recipientEvidence); err != nil {
			t.Fatal(err)
		}
		var uncertain sessionintent.Result
		if err := json.Unmarshal(resultJSON, &uncertain); err != nil || uncertain.Clarification == "" || len(uncertain.Patch) != 0 {
			t.Fatalf("unknown recipient did not clarify: %+v %v", uncertain, err)
		}
		afterRecipient, err := repo.ChannelSessionState(ctx, channel.ID, link.Identity())
		if err != nil || !beforeRecipient.Equal(afterRecipient) {
			t.Fatalf("unknown recipient changed state: %+v %v", afterRecipient, err)
		}
		if err := json.Unmarshal(recipientEvidence, &leaves); err != nil || len(leaves) != 1 {
			t.Fatalf("recipient interpretation count=%d %v", len(leaves), err)
		}
		t.Logf("LIVE CLI unknown recipient result=%s evidence=%s", resultJSON, recipientEvidence)
	}
}
