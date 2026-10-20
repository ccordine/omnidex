package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/sessionintent"
)

// A real HTTP provider fixture supplies bounded semantic leaves. These tests
// establish protocol, transaction and authority behavior, not live AI accuracy.
func TestUserIntentHTTPProposalReconnectAndApproval(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	for _, goal := range []string{"Present a short report.", "Record a selected value."} {
		t.Run(goal, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var mu sync.Mutex
			meanings := []string{}
			calls := 0
			modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				calls++
				var request struct {
					Prompt string `json:"prompt"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if len(meanings) == 0 {
					http.Error(w, "unexpected model call", 500)
					return
				}
				meaning := meanings[0]
				meanings = meanings[1:]
				answer := meaning
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"model": "fixture-parser", "created_at": time.Now().UTC().Format(time.RFC3339Nano), "response": answer, "done": true, "done_reason": "stop", "prompt_eval_count": 128, "eval_count": 1})
			}))
			defer modelServer.Close()
			setMeanings := func(values ...string) { mu.Lock(); defer mu.Unlock(); meanings = values }
			repository := freshCLIChatAPIRepository(t, databaseURL)
			route := codingagent.Route{Provider: "codex", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
			configuration := codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: codingagent.Route{Provider: "ollama", Model: "fixture-parser"}}
			repository.WithCodingAgents(configuration)
			server, err := NewServer(repository, nil, ServerOptions{LifecycleContext: ctx, ProviderConfig: config.Config{OllamaBaseURL: modelServer.URL}, HostDirectoryAccessRoot: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			dropCommittedResponse := false
			handler := server.Handler()
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if dropCommittedResponse && strings.HasSuffix(r.URL.Path, "/session/intent/execute") {
					dropCommittedResponse = false
					recorded := httptest.NewRecorder()
					handler.ServeHTTP(recorded, r)
					if recorded.Code != http.StatusOK {
						t.Errorf("commit before lost response: %d %s", recorded.Code, recorded.Body.String())
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("{")) // Committed response lost in transit.
					return
				}
				handler.ServeHTTP(w, r)
			}))
			defer httpServer.Close()
			apiClient, err := client.New(httpServer.URL, 10*time.Second)
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
			state, err := repository.ChannelSessionState(ctx, channel.ID, link.Identity())
			if err != nil {
				t.Fatal(err)
			}
			ambiguousID, _ := model.NewLifecycleOperationID()
			setMeanings(`{"patch":[],"clarification":"Which item should I remove?"}`)
			ambiguous, err := apiClient.InterpretUserMessage(ctx, channel, sessionintent.Request{OperationID: ambiguousID, Text: "remove that", State: state})
			if err != nil || ambiguous.Clarification != "Which item should I remove?" || len(ambiguous.Patch) != 0 {
				t.Fatalf("ambiguity=%+v %v", ambiguous, err)
			}
			if _, err := apiClient.ExecuteUserMessage(ctx, channel, link.Identity(), ambiguousID); err == nil {
				t.Fatal("ambiguous message executed")
			}
			unchanged, err := repository.ChannelSessionState(ctx, channel.ID, link.Identity())
			if err != nil || !state.Equal(unchanged) {
				t.Fatalf("ambiguous message changed authority: %+v %v", unchanged, err)
			}
			id, _ := model.NewLifecycleOperationID()
			messageJSON, _ := json.Marshal(sessionintent.Content{Text: goal, Recipient: "codex"})
			setMeanings(`{"patch":[{"op":"replace","path":"/message","value":` + string(messageJSON) + `} ]}`)
			parsed, err := apiClient.InterpretUserMessage(ctx, channel, sessionintent.Request{OperationID: id, Text: goal, State: state})
			if err != nil {
				t.Fatal(err)
			}
			if parsed.RequiresApproval || len(parsed.Patch) != 1 || parsed.Patch[0].Path != "/message" {
				t.Fatalf("initial routing=%+v", parsed)
			}
			receipt, err := apiClient.ExecuteUserMessage(ctx, channel, link.Identity(), id)
			if err != nil {
				t.Fatal(err)
			}
			planner, err := repository.ClaimNextStep(ctx, "user-intent-http")
			if err != nil || planner == nil {
				t.Fatalf("planner=%+v error=%v", planner, err)
			}
			item := model.LedgerItem{ID: "one", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "fixture", Revision: 1}, Rationale: "Requested local task.", Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}
			ledger := model.EngineeringLedger{ID: "fixture", Revision: 1, Tasks: []model.LedgerTask{{LedgerItem: item, Goal: goal, WriteFiles: []string{"fixture.txt"}}}}
			routes := []queue.CodingLedgerRoute{}
			for _, role := range []string{"planner", "executor", "reviewer"} {
				routes = append(routes, queue.CodingLedgerRoute{Role: role, ConfigurationRevision: 1, Provider: route.Provider, Model: route.Model, ReasoningEffort: route.Effort, ServiceTier: route.ServiceTier})
			}
			created, err := repository.CreateCodingLedger(ctx, queue.CreateCodingLedgerCommand{JobID: receipt.JobID, OperationID: string(id) + "_create", OwnerTurnID: "fixture", OwnerTurnText: goal, Ledger: ledger, Routes: routes, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"one"})})
			if err != nil {
				t.Fatal(err)
			}
			if err := repository.MarkCodingLedgerReviewReady(ctx, planner.Authority, created.LedgerID, created.Revision); err != nil {
				t.Fatal(err)
			}
			state, err = repository.ChannelSessionState(ctx, channel.ID, link.Identity())
			if err != nil {
				t.Fatal(err)
			}

			// A read-only committed receipt is authority for that intent only.
			statusID, _ := model.NewLifecycleOperationID()
			setMeanings(`{"patch":[{"op":"replace","path":"/status_visible","value":true}]}`)
			statusRequest := sessionintent.Request{OperationID: statusID, Text: "Show status.", State: state, LedgerRevision: created.Revision}
			if _, err := apiClient.InterpretUserMessage(ctx, channel, statusRequest); err != nil {
				t.Fatal(err)
			}
			statusReceipt, err := apiClient.ExecuteUserMessage(ctx, channel, link.Identity(), statusID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := apiClient.Cancel(ctx, channel, link.Identity(), receipt.JobID, statusID, "Cancel this job."); !client.IsHTTPStatus(err, http.StatusConflict) {
				t.Fatalf("status receipt authorized cancel: %v", err)
			}
			replayStatus, err := apiClient.ExecuteUserMessage(ctx, channel, link.Identity(), statusID)
			if err != nil || !reflect.DeepEqual(statusReceipt, replayStatus) {
				t.Fatalf("status replay=%+v %v", replayStatus, err)
			}
			unchangedStatus, err := repository.ChannelSessionState(ctx, channel.ID, link.Identity())
			if err != nil || !state.Equal(unchangedStatus) {
				t.Fatalf("receipt collision changed state: %+v %v", unchangedStatus, err)
			}
			removalID, _ := model.NewLifecycleOperationID()
			setMeanings(`{"patch":[{"op":"replace","path":"/items/0/disposition","value":"tombstoned"}]}`)
			removal, err := apiClient.InterpretUserMessage(ctx, channel, sessionintent.Request{OperationID: removalID, Text: "remove that", State: state, LedgerRevision: created.Revision, SelectedItem: "one"})
			if err != nil {
				t.Fatal(err)
			}
			if !removal.RequiresApproval || removal.Pitch == "" {
				t.Fatalf("missing pitch=%+v", removal)
			}
			if err := link.Close(); err != nil {
				t.Fatal(err)
			}
			link, err = apiClient.OpenWorkspace(ctx, directory, strings.Repeat("1", 32))
			if err != nil {
				t.Fatal(err)
			}
			defer link.Close()
			snapshot, err := apiClient.ChatSession(ctx, channel, link.Identity(), 10)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.PendingUserAction == nil || snapshot.PendingUserAction.ID != removalID {
				t.Fatalf("lost pending proposal on reconnect: %+v", snapshot.PendingUserAction)
			}
			approvalID, _ := model.NewLifecycleOperationID()
			setMeanings(`{"patch":[{"op":"replace","path":"/approval","value":true}]}`)
			approval, err := apiClient.InterpretUserMessage(ctx, channel, sessionintent.Request{OperationID: approvalID, Text: "yeah that's what I meant", State: snapshot.State, LedgerRevision: created.Revision})
			if err != nil {
				t.Fatal(err)
			}
			if approval.RequiresApproval {
				t.Fatal("recursive approval")
			}
			dropCommittedResponse = true
			if _, err := apiClient.ExecuteUserMessage(ctx, channel, link.Identity(), approvalID); err == nil {
				t.Fatal("lost response appeared successful")
			}
			final, err := repository.ExecuteUserIntent(ctx, approvalID, channel.ID, link.Identity())
			if err != nil {
				t.Fatal(err)
			}
			if err := link.Close(); err != nil {
				t.Fatal(err)
			}
			link, err = apiClient.OpenWorkspace(ctx, directory, strings.Repeat("1", 32))
			if err != nil {
				t.Fatal(err)
			}
			defer link.Close()
			current, _, err := repository.CurrentCodingLedgerForJob(ctx, receipt.JobID)
			if err != nil || current.Ledger.Tasks[0].Disposition != model.LedgerDispositionTombstoned {
				t.Fatalf("removal was not committed: %+v %v", current, err)
			}
			replay, err := apiClient.ExecuteUserMessage(ctx, channel, link.Identity(), approvalID)
			if err != nil || !reflect.DeepEqual(replay, final) {
				t.Fatalf("replay=%+v error=%v", replay, err)
			}
			afterReplay, _, err := repository.CurrentCodingLedgerForJob(ctx, receipt.JobID)
			if err != nil || afterReplay.Revision != current.Revision {
				t.Fatal("receipt replay repeated ledger effect")
			}
			staleID, _ := model.NewLifecycleOperationID()
			if _, err := apiClient.InterpretUserMessage(ctx, channel, sessionintent.Request{OperationID: staleID, Text: "start", State: snapshot.State, LedgerRevision: created.Revision}); !client.IsHTTPStatus(err, http.StatusConflict) {
				t.Fatalf("stale API authority: %v", err)
			}
			changedRequest := sessionintent.Request{OperationID: approvalID, Text: "different action", State: snapshot.State, LedgerRevision: created.Revision}
			if _, err := apiClient.InterpretUserMessage(ctx, channel, changedRequest); !client.IsHTTPStatus(err, http.StatusConflict) {
				t.Fatalf("changed operation API authority: %v", err)
			}
			state, err = repository.ChannelSessionState(ctx, channel.ID, link.Identity())
			if err != nil {
				t.Fatal(err)
			}
			canceledID, _ := model.NewLifecycleOperationID()
			canceledRequest := sessionintent.Request{OperationID: canceledID, Text: "Interrupted before interpretation.", State: state, LedgerRevision: final.Revision}
			canceled, err := apiClient.CancelUserMessage(ctx, channel, canceledRequest)
			if err != nil || !canceled.Interrupted {
				t.Fatalf("message cancellation=%+v %v", canceled, err)
			}
			if _, err := apiClient.InterpretUserMessage(ctx, channel, canceledRequest); !client.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
				t.Fatalf("canceled message was interpreted: %v", err)
			}
			mu.Lock()
			remaining, count := len(meanings), calls
			mu.Unlock()
			if remaining != 0 || count != 5 {
				t.Fatal(fmt.Sprintf("provider calls=%d remaining=%d", count, remaining))
			}
		})
	}
}
