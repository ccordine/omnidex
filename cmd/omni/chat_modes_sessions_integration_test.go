package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/api"
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
	"github.com/gryph/omnidex/internal/workspacetransport"
	"github.com/jackc/pgx/v5"
)

// Combined terminal -> HTTP -> PostgreSQL scenario. Research and approval
// interpretation use scripted semantic fixtures, as does planner ledger output.
// Cancellation, receipts, streams and workspace binding use production code. The same origin then shares its fork, approves its plan and
// queues an amendment without changing frozen execution authority.
func TestIntegratedModesSessionsResearchCancellationReconnectAndPaste(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	id, _ := model.NewLifecycleOperationID()
	schema := "integrated_" + strings.TrimPrefix(string(id), "lifecycle_operation_")[:32]
	pool, err := db.ConnectRuntime(ctx, databaseURL, schema, database.SetupSQL())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		pool.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		pool.Close()
	}()
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatal(err)
	}
	route := codingagent.Route{Provider: "codex", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
	repo := queue.New(pool, authority).WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route})
	server, err := api.NewServer(repo, nil, api.ServerOptions{LifecycleContext: ctx, HostDirectoryAccessRoot: t.TempDir(), RealtimeStreamMaxAge: "1m", RealtimeHeartbeat: "1s", RealtimeWriteTimeout: "1s"})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan model.ChannelID, 3)
	requests := make(chan sessionintent.ResearchRequest, 3)
	releases := map[model.ChannelID]chan struct{}{}
	var releaseMu sync.Mutex
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Script only the semantic choice; persistence, stale-authority validation
		// and execution use the ordinary repository and production HTTP endpoint.
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/session/intent") {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
			var request sessionintent.Request
			if err := json.Unmarshal(raw, &request); err != nil {
				t.Error(err)
				return
			}
			if request.Recipient == sessionintent.Native {
				result, err := repo.InterpretUserIntent(r.Context(), request, func(_ context.Context, current sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
					if err := request.ValidateRouting(current); err != nil {
						return sessionintent.Result{}, nil, err
					}
					if request.Mode != sessionintent.Interact || request.Text != "deny" || !reflect.DeepEqual(current.NativeChoices, []string{"allow once", "deny"}) {
						return sessionintent.Result{}, nil, fmt.Errorf("unexpected terminal approval: %+v", request)
					}
					return sessionintent.Result{OperationID: request.OperationID, Patch: []userintent.Patch{{Op: "replace", Path: "/native_reply", Value: json.RawMessage(`"deny"`)}}}, json.RawMessage(`[]`), nil
				})
				if err != nil {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
				json.NewEncoder(w).Encode(result)
				return
			}
		}
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/session/research") {
			server.Handler().ServeHTTP(w, r)
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		channelID := model.ChannelID(parts[3])
		var request sessionintent.ResearchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		requests <- request
		result, err := repo.Research(r.Context(), channelID, request, func(ctx context.Context, _ sessionintent.ResearchContext) (string, json.RawMessage, error) {
			entered <- channelID
			releaseMu.Lock()
			release := releases[channelID]
			releaseMu.Unlock()
			select {
			case <-release:
				answer := "FINDING " + string(channelID)
				if request.Fork != nil {
					answer = "Private fork detail. " + answer
				}
				return answer, json.RawMessage(`[]`), nil
			case <-ctx.Done():
				return "", json.RawMessage(`[]`), ctx.Err()
			}
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(result)
	})
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()
	apiClient, err := client.New(httpServer.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	owner := strings.Repeat("1", 32)
	var channels []model.Channel
	var identities []string
	var jobs []int64
	var secondLink *workspacetransport.Local
	for index := range 2 {
		directory, err := projectroot.OpenDirectory(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer directory.Close()
		link, err := apiClient.OpenWorkspace(ctx, directory, owner)
		if err != nil {
			t.Fatal(err)
		}
		defer link.Close()
		if index == 1 {
			secondLink = link
		}
		channel, err := apiClient.BootstrapCLIChatSession(ctx, directory.Path(), link.Identity())
		if err != nil {
			t.Fatal(err)
		}
		channels = append(channels, channel)
		identities = append(identities, link.Identity())
		releases[channel.ID] = make(chan struct{})
		op, _ := model.NewLifecycleOperationID()
		turn, err := repo.SubmitChannelSessionTurn(ctx, queue.ChannelSessionTurnCommand{OperationID: op, ChannelID: channel.ID, WorkspaceRoot: channel.WorkspaceRoot, WorkspaceIdentity: link.Identity(), Text: "Keep session work running", CodingMode: codingagent.Agents})
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, turn.Job.ID)
	}
	claims := map[int64]model.StepAttemptAuthority{}
	for range 2 {
		claim, err := repo.ClaimNextStep(ctx, "sessions-host")
		if err != nil || claim == nil {
			t.Fatal(claim, err)
		}
		claims[claim.Job.ID] = claim.Authority
	}
	op, _ := model.NewLifecycleOperationID()
	if _, err := apiClient.Interrupt(ctx, channels[1], identities[1], jobs[1], op, "Wait for owner"); err != nil {
		t.Fatal(err)
	}
	{
		if err := secondLink.Close(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			listing, err := apiClient.CLISessions(ctx, channels[0].WorkspaceRoot, identities[0])
			if err != nil {
				t.Fatal(err)
			}
			disconnected := false
			for _, entry := range listing.Sessions {
				if entry.ID == channels[1].ID {
					disconnected = !entry.Connected
				}
			}
			if disconnected {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("fixture connection did not retire")
			}
			time.Sleep(time.Millisecond)
		}
	}
	bootstrap, err := loadChatBootstrap(ctx, ctx, apiClient, channels[0].WorkspaceRoot, identities[0])
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.stream.Close()
	console, keys := modeEditorConsole(t, ctx)
	result := make(chan error, 1)
	go func() {
		result <- runChatSessions(chatSessionConfig{Context: ctx, Cancel: cancel, Client: apiClient, Channel: bootstrap.channel, WorkspaceIdentity: identities[0], Snapshot: bootstrap.snapshot, Stream: bootstrap.stream, Console: console, Signals: make(chan os.Signal)}, owner)
	}()
	defer func() {
		cancel()
		select {
		case <-result:
		case <-time.After(3 * time.Second):
			t.Error("host resources failed to retire")
		}
	}()
	modeKeys(t, keys, "界ab\x1b[D\x02r")
	waitMode(t, console, "research", "Research")
	modeKeys(t, keys, "\x1b[200~first research\n  界 paste\x1b[201~\r")
	select {
	case got := <-entered:
		if got != channels[0].ID {
			t.Fatal(got)
		}
	case <-ctx.Done():
		t.Fatal("first research did not start")
	}
	modeKeys(t, keys, "\x02l")
	waitMode(t, console, "ledger", "")
	modeKeys(t, keys, "\x02r\x025")
	waitSessionPane(t, console, 2)
	// Listing initially selects the origin; navigate to the other exact row.
	console.workspace.mu.Lock()
	index := console.workspace.sessions.selected
	console.workspace.mu.Unlock()
	if index == 0 {
		modeKeys(t, keys, "j")
	} else {
		modeKeys(t, keys, "k")
	}
	modeKeys(t, keys, "\r")
	waitSelectedSession(t, console, channels[1].ID)
	modeKeys(t, keys, "\x02rsecond research\r")
	select {
	case got := <-entered:
		if got != channels[1].ID {
			t.Fatal(got)
		}
	case <-ctx.Done():
		t.Fatal("second research did not start")
	}
	// Cancel only the selected Research operation; the waiting coding job remains.
	modeKeys(t, keys, "/cancel\r")
	deadlineCancel := time.Now().Add(4 * time.Second)
	for {
		history, err := apiClient.ResearchHistory(ctx, channels[1], identities[1])
		if err != nil {
			t.Fatal(err)
		}
		if len(history) == 1 && history[0].Error != "" {
			break
		}
		if time.Now().After(deadlineCancel) {
			t.Fatal("selected research cancellation was not durable")
		}
		time.Sleep(time.Millisecond)
	}
	// Finish the hidden origin's native work while the waiting session is selected.
	close(releases[channels[0].ID])
	deadline := time.Now().Add(4 * time.Second)
	for {
		console.workspace.mu.Lock()
		completed := strings.Contains(strings.Join(console.workspace.researchLines, "\n"), "FINDING "+string(channels[0].ID))
		console.workspace.mu.Unlock()
		if completed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("hidden research did not complete")
		}
		time.Sleep(time.Millisecond)
	}
	modeKeys(t, keys, "\x025")
	// The second pane is independent; allow its HTTP listing to arrive before j.
	time.Sleep(150 * time.Millisecond)
	if channels[1].ID < channels[0].ID {
		modeKeys(t, keys, "j\r")
	} else {
		modeKeys(t, keys, "k\r")
	}
	waitSelectedSession(t, console, channels[0].ID)
	frame, _, _, _ := workspaceFrame(console.workspace)
	if !strings.Contains(strings.Join(frame, "\n"), "FINDING "+string(channels[0].ID)) {
		t.Fatal("origin finding was not restored")
	}
	// A selected-message fork uses the same origin while retaining main history.
	state, err := repo.ChannelSessionState(ctx, channels[0].ID, identities[0])
	if err != nil || state.LatestMessageID == nil {
		t.Fatal("missing fork source", err)
	}
	modeKeys(t, keys, fmt.Sprintf("/research-fork messages:%d inspect selected source\r", *state.LatestMessageID))
	select {
	case channel := <-entered:
		if channel != channels[0].ID {
			t.Fatal("fork crossed sessions", channel)
		}
	case <-ctx.Done():
		t.Fatal("fork did not start")
	}
	<-requests // first main question
	<-requests // second canceled question
	fork := <-requests
	deadlineFork := time.Now().Add(4 * time.Second)
	for {
		history, err := apiClient.ResearchConversationHistory(ctx, channels[0], identities[0], fork.ConversationID)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) == 1 {
			if history[0].Fork == nil || history[0].Fork.SourceChannel != channels[0].ID || len(history[0].Fork.MessageIDs) != 1 || history[0].Fork.MessageIDs[0] != *state.LatestMessageID || history[0].Answer == "" {
				t.Fatal("fork lost selected source", history)
			}
			break
		}
		if time.Now().After(deadlineFork) {
			t.Fatal("fork outcome was not durable")
		}
		time.Sleep(time.Millisecond)
	}
	// The origin's independent editor survives hidden work and a narrow resize.
	modeKeys(t, keys, "\x02f")
	waitMode(t, console, "infer", "界ab")
	modeKeys(t, keys, "\x02eother")
	waitMode(t, console, "interact", "other")
	console.presentationMu.Lock()
	columns, rows, resizeErr := console.workspace.Resize(36, 12)
	if resizeErr == nil {
		resizeErr = console.terminal.SetSize(columns, rows)
	}
	console.terminalWidth, console.terminalHeight = columns, rows
	console.presentationMu.Unlock()
	if resizeErr != nil {
		t.Fatal(resizeErr)
	}
	modeKeys(t, keys, "\x02fX")
	waitMode(t, console, "infer", "界aXb")
	// Reconnect through the normal bootstrap without changing the job or history.
	reconnected, err := loadChatBootstrap(ctx, ctx, apiClient, channels[0].WorkspaceRoot, identities[0])
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.stream.Close()
	if reconnected.channel.ID != channels[0].ID || reconnected.snapshot.ActiveJob == nil || reconnected.snapshot.ActiveJob.Job.ID != jobs[0] {
		t.Fatal("reconnect changed the origin or running job")
	}
	for i, channel := range channels {
		snapshot, err := apiClient.ChatSession(ctx, channel, identities[i], client.MaxChatSessionMessages)
		if err != nil {
			t.Fatal(err)
		}
		expected := model.JobStatusRunning
		if i == 1 {
			expected = model.JobStatusWaiting
		}
		if snapshot.ActiveJob == nil || snapshot.ActiveJob.Job.ID != jobs[i] || snapshot.ActiveJob.Job.Status != expected {
			t.Fatal("switch stopped or rebound work", snapshot.ActiveJob)
		}
		history, err := apiClient.ResearchHistory(ctx, channel, identities[i])
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && (len(history) != 1 || history[0].Question != "first research\n  界 paste") {
			t.Fatal("research crossed origins", history)
		}
	}
	// Continue this exact origin through planner intake and explicit owner approval.
	modeKeys(t, keys, "\x02r")
	waitMode(t, console, "research", "")
	task := model.LedgerTask{LedgerItem: model.LedgerItem{ID: "task", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "integrated", Revision: 1}, Rationale: "Requested integrated fixture.", Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}, Goal: "Keep session work running", ReadFiles: []string{}, WriteFiles: []string{"fixture.txt"}, DeleteFiles: []string{}}
	task.Objective = "Preserve the running session while collecting research."
	task.OwnerContext = []model.TaskContext{{Text: "Preserve checkout edits, workspace binding and owner approval authority.", Provenance: task.Provenance}}
	task.Constraints = []string{"Research findings do not authorize execution.", "Queue amendments without changing the frozen task."}
	task.FileResponsibilities = []model.TaskFileResponsibility{{Path: "fixture.txt", Action: "create", Responsibility: "Retain the approved fixture output."}}
	task.Acceptance = []model.TaskAcceptance{{ID: "session", Text: "The session and frozen task survive research and reconnect.", Verification: []string{"Compare session identity, generation and frozen task after reconnect."}}}
	routes := []queue.CodingLedgerRoute{}
	for _, role := range []string{"planner", "executor", "reviewer"} {
		routes = append(routes, queue.CodingLedgerRoute{Role: role, ConfigurationRevision: 1, Provider: route.Provider, Model: route.Model, ReasoningEffort: route.Effort, ServiceTier: route.ServiceTier})
	}
	ledger, err := repo.CreateCodingLedger(ctx, queue.CreateCodingLedgerCommand{JobID: jobs[0], OperationID: "integrated-create", OwnerTurnID: "integrated", OwnerTurnText: task.Goal, Ledger: model.EngineeringLedger{ID: "integrated", Revision: 1, Tasks: []model.LedgerTask{task}}, Routes: routes, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{task.ID})})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkCodingLedgerReviewReady(ctx, claims[jobs[0]], ledger.LedgerID, ledger.Revision); err != nil {
		t.Fatal(err)
	}
	// /status reloads authoritative session state before the following command.
	modeKeys(t, keys, "/status\r")
	modeKeys(t, keys, fmt.Sprintf("/research-share %s FINDING %s\r", fork.OperationID, channels[0].ID))
	var shared string
	deadline = time.Now().Add(4 * time.Second)
	for {
		err := pool.QueryRow(ctx, `SELECT request_json->>'text' FROM channel_user_intents WHERE channel_id=$1 AND request_json->>'recipient'='planner' AND receipt_json IS NOT NULL ORDER BY created_at DESC LIMIT 1`, channels[0].ID).Scan(&shared)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fork was not shared through CLI/API", err)
		}
		time.Sleep(time.Millisecond)
	}
	for _, want := range []string{string(fork.OperationID), string(fork.ConversationID), "message_ids", "FINDING " + string(channels[0].ID), "not approval"} {
		if !strings.Contains(shared, want) {
			t.Fatalf("shared finding lost %q: %s", want, shared)
		}
	}
	if strings.Contains(shared, "Private fork detail.") {
		t.Fatal("unselected fork detail reached planner")
	}
	planner, err := repo.ClaimNextStep(ctx, "integrated-finding-planner")
	if err != nil || planner == nil || planner.Job.ID != jobs[0] {
		t.Fatal("share lost planner job", planner, err)
	}
	current, _, err := repo.CurrentCodingLedgerForJob(ctx, jobs[0])
	if err != nil {
		t.Fatal(err)
	}
	if current.State == "frozen" {
		t.Fatal("sharing authorized execution")
	}
	// The scripted planner keeps the same requested task; only owner controls freeze.
	ledger, err = repo.ApplyCodingLedgerMutation(ctx, queue.ApplyCodingLedgerMutationCommand{LedgerID: ledger.LedgerID, OwnerTurnText: shared, Mutation: model.LedgerMutation{OperationID: "integrated-finding", OwnerTurnID: "integrated-finding", BaseRevision: current.Revision, Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationAmend, TargetID: task.ID, Item: &model.LedgerMutationItem{Task: &task}, Rationale: "Retain the requested task after research."}}, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{task.ID})}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkCodingLedgerReviewReady(ctx, planner.Authority, ledger.LedgerID, ledger.Revision); err != nil {
		t.Fatal(err)
	}
	op, _ = model.NewLifecycleOperationID()
	approved, err := apiClient.MutateCodingLedgerDirect(ctx, channels[0], identities[0], jobs[0], client.PlanningTurn{OperationID: string(op), OwnerTurnID: string(op), Text: "Agree this exact task.", ObservedRevision: ledger.Revision}, client.DirectPlanningAction{Kind: model.LedgerMutationReview, TargetID: task.ID, ReviewState: model.LedgerReviewAgreed, Rationale: "Explicit fixture owner agreement."})
	if err != nil {
		t.Fatal(err)
	}
	if claim, err := repo.ClaimNextStep(ctx, "before-owner-freeze"); err != nil || claim != nil {
		t.Fatal("agreement started execution", claim, err)
	}
	op, _ = model.NewLifecycleOperationID()
	frozen, err := apiClient.FreezeCodingLedger(ctx, channels[0], identities[0], jobs[0], client.PlanningTurn{OperationID: string(op), OwnerTurnID: string(op), Text: "Sign off and start this plan.", ObservedRevision: approved.Revision})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := repo.ClaimNextStep(ctx, "integrated-executor")
	if err != nil || executor == nil || executor.Job.ID != jobs[0] {
		t.Fatal(executor, err)
	}
	selected, err := repo.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || selected == nil {
		t.Fatal(selected, err)
	}
	if selected.Instructions() != task.Instructions() {
		t.Fatal("execution selection lost the complete approved task packet")
	}
	// A native approval arrives on this frozen task. Visit other modes before
	// answering in Interact; the exact denied response must resume this attempt.
	callID, err := repo.CodingAgentEvidence(executor.Authority).Append(ctx, codingagent.Entry{Generation: executor.Authority.Generation, Phase: "call_opened", Role: codingagent.Execute, TaskID: selected.ID})
	if err != nil {
		t.Fatal(err)
	}
	inputID, err := repo.WaitForCodingNativeInput(ctx, executor.Authority, codingagent.NativeInputRequest{ID: json.RawMessage(`1`), Method: "item/commandExecution/requestApproval", ThreadID: "integrated-thread", TurnID: "integrated-turn", Prompt: "Allow the integrated fixture command?", Choices: []codingagent.NativeInputChoice{
		{Label: "allow once", Response: json.RawMessage(`{"decision":"accept"}`)},
		{Label: "deny", Response: json.RawMessage(`{"decision":"decline"}`)},
	}}, callID, selected.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Interact deliberately retained its earlier "other" draft. Clear that
	// editor before entering the command; a switch must not discard it.
	modeKeys(t, keys, "\x15/status\r\x02l")
	waitMode(t, console, "ledger", "")
	modeKeys(t, keys, "\x02r")
	waitMode(t, console, "research", "")
	if reply, found, err := repo.CodingNativeInputReply(ctx, executor.Authority, inputID); err != nil || found {
		t.Fatal("mode switch answered approval", string(reply), err)
	}
	modeKeys(t, keys, "\x02edeny\r")
	deadline = time.Now().Add(4 * time.Second)
	for {
		reply, found, err := repo.CodingNativeInputReply(ctx, executor.Authority, inputID)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			var decision map[string]string
			if err := json.Unmarshal(reply, &decision); err != nil || decision["decision"] != "decline" {
				t.Fatal("wrong native reply", string(reply), err)
			}
			break
		}
		if time.Now().After(deadline) {
			frame, _, _, _ := workspaceFrame(console.workspace)
			t.Fatal("terminal approval did not reach native request", strings.Join(frame, "\n"))
		}
		time.Sleep(time.Millisecond)
	}
	op, _ = model.NewLifecycleOperationID()
	receipt, err := apiClient.SubmitObservedSessionTurn(ctx, channels[0], identities[0], op, "Amend the plan after this task.", codingagent.Agents, jobs[0], executor.Authority.Generation)
	if err != nil || receipt.JobID != jobs[0] {
		t.Fatal(receipt, err)
	}
	message, err := repo.NextChannelSessionMessage(ctx, executor.Authority)
	if err != nil || message == nil || message.OperationID != op || message.PlanningBinding == nil || message.PlanningBinding.Revision != frozen.Revision {
		t.Fatal("amendment lost frozen binding", message, err)
	}
	resolved, err := repo.ResolveChannelSessionMessage(ctx, queue.ResolveChannelSessionMessageCommand{Authority: executor.Authority, OperationID: op, Kind: assemblyline.ObjectiveKindWorkspaceMutation})
	if err != nil || resolved.Replanned {
		t.Fatal("amendment changed running authority", resolved, err)
	}
	reconnectRepo := queue.New(pool, authority).WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route})
	retained, _, err := reconnectRepo.CurrentCodingLedgerForJob(ctx, jobs[0])
	if err != nil || retained.Revision != frozen.Revision || !reflect.DeepEqual(retained.Ledger, frozen.Ledger) || len(retained.PendingAmendments) != 1 {
		t.Fatal("reconnect lost queued amendment", retained, err)
	}
	again, err := reconnectRepo.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || !reflect.DeepEqual(selected, again) {
		t.Fatal("amendment changed frozen task", err)
	}
	final, err := loadChatBootstrap(ctx, ctx, apiClient, channels[0].WorkspaceRoot, identities[0])
	if err != nil {
		t.Fatal(err)
	}
	defer final.stream.Close()
	if final.channel.ID != channels[0].ID || final.snapshot.ActiveJob == nil || final.snapshot.ActiveJob.Job.ID != jobs[0] || final.snapshot.ActiveJob.Job.CurrentGeneration != executor.Authority.Generation {
		t.Fatal("amendment reconnect changed session/job")
	}
	close(releases[channels[1].ID])
	cancel()
	select {
	case err := <-result:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		result <- nil
	case <-time.After(3 * time.Second):
		t.Fatal("host failed to stop")
	}
}
