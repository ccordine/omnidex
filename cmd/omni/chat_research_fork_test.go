package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func TestResearchBackgroundSwitchAndSelectedCancellation(t *testing.T) {
	started := make(chan struct{})
	var mu sync.Mutex
	var history []sessionintent.ResearchTurn
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			mu.Lock()
			defer mu.Unlock()
			json.NewEncoder(w).Encode(history)
			return
		}
		var request sessionintent.ResearchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			return
		}
		close(started)
		<-r.Context().Done()
		mu.Lock()
		history = []sessionintent.ResearchTurn{{OperationID: request.OperationID, Question: request.Text, Error: "context canceled"}}
		mu.Unlock()
	}))
	defer server.Close()
	session := newChatOperationTestSession(t, server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	session.ctx = ctx
	console, _ := planReviewTestConsole(t)
	view, _ := testWorkspace(t, 100, 24)
	console.workspace = view
	session.renderer = chatRenderer{console: console}
	if err := session.setConversationMode("research"); err != nil {
		t.Fatal(err)
	}
	if err := session.acceptResearchQuestion("Read the source"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Research did not start")
	}
	if err := session.setConversationMode("interact"); err != nil {
		t.Fatal(err)
	}
	if session.mode != sessionintent.Interact || session.pendingResearch == nil {
		t.Fatal("switch lost research")
	}
	// Cancellation selects only this research operation and leaves coding intact.
	pending := session.pendingResearch
	if err := session.setConversationMode("research"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.acceptInput("/cancel", false); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-session.researchResults:
		if result.err != nil || result.turn.Error != "context canceled" {
			t.Fatal("durable cancellation not observed", result)
		}
		if err := session.completeResearch(result); err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled Research did not finish")
	}
	if pending == nil || session.pendingResearch != nil || session.pendingControl != nil {
		t.Fatal("cancel changed coding authority or lost replay")
	}
}

func TestResearchForkSelectionAndFindingPlanningOrigin(t *testing.T) {
	for _, selection := range []string{"messages:1,2", "ledger:3:4"} {
		seed, err := parseResearchSelection(selection, "source")
		if err != nil || seed.SourceChannel != "source" {
			t.Fatal(seed, err)
		}
	}
	turn := sessionintent.ResearchTurn{OperationID: "lifecycle_operation_finding", ConversationID: "lifecycle_operation_fork", Answer: "First finding. Selected finding.", Fork: &sessionintent.ResearchSeed{SourceChannel: "source", MessageIDs: []int64{1, 2}}}
	text, err := researchPlanningInput("source", turn, "Selected finding.")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"source_session", "source", "lifecycle_operation_finding", "lifecycle_operation_fork", "message_ids", "Selected finding.", "not approval"} {
		if !strings.Contains(text, expected) {
			t.Fatal("lost origin", text)
		}
	}
	if strings.Contains(text, "First finding.") {
		t.Fatal("unselected finding shared")
	}
	if _, err := researchPlanningInput("source", turn, "invented"); err == nil {
		t.Fatal("invented selection accepted")
	}
	turn.Error = "failed"
	if _, err := researchPlanningInput(model.ChannelID("source"), turn, ""); err == nil {
		t.Fatal("failed finding shared")
	}
}

func TestResearchShareSendsOnlySelectedFindingToPlanner(t *testing.T) {
	for _, deferredSwitch := range []bool{false, true} {
		name := "immediate editor switch"
		if deferredSwitch {
			name = "queued editor switch"
		}
		t.Run(name, func(t *testing.T) {

			var received sessionintent.Request
			turn := sessionintent.ResearchTurn{OperationID: "lifecycle_operation_selected_finding", ConversationID: "lifecycle_operation_selected_fork", Question: "Investigate", Answer: "Keep private. Share this.", Fork: &sessionintent.ResearchSeed{SourceChannel: "fixture-channel", MessageIDs: []int64{7}}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/session/research") {
					if r.URL.Query().Get("conversation_id") != string(turn.ConversationID) {
						t.Error("wrong conversation")
					}
					json.NewEncoder(w).Encode([]sessionintent.ResearchTurn{turn})
					return
				}
				if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
					t.Error(err)
				}
				// Observe the real Interact intake, then fail before any fixture mutation.
				http.Error(w, "stale fixture", http.StatusConflict)
			}))
			defer server.Close()
			session := newChatOperationTestSession(t, server.URL)
			console, _ := planReviewTestConsole(t)
			if deferredSwitch {
				console, _ = modeEditorConsole(t)
				console.workspace.conversationMode = "research"
			}
			session.renderer = chatRenderer{console: console}
			session.researchConversation = turn.ConversationID
			session.mode = sessionintent.Research
			session.active = &model.JobDetails{Job: model.Job{Status: model.JobStatusWaiting, CurrentGeneration: 1}, Steps: []model.Step{{Action: codingPlanReviewStepAction, Generation: 1, Status: model.StepStatusWaiting}}}
			if err := session.researchCommand("research-share", string(turn.OperationID)+" Share this."); err == nil {
				t.Fatal("expected stale intake")
			}
			if received.Mode != sessionintent.Interact || received.Recipient != sessionintent.Planner || !strings.Contains(received.Text, "Share this.") || strings.Contains(received.Text, "Keep private.") || !strings.Contains(received.Text, string(turn.OperationID)) || !strings.Contains(received.Text, string(turn.ConversationID)) || !strings.Contains(received.Text, "message_ids") {
				t.Fatalf("wrong planning input: %+v", received)
			}
			received = sessionintent.Request{}
			inputID := int64(9)
			session.active.ExecutionInputID = &inputID
			if err := session.researchCommand("research-share", string(turn.OperationID)+" Share this."); err == nil {
				t.Fatal("sharing answered native input instead of planner")
			}
			if received.OperationID != "" {
				t.Fatal("sharing dispatched to native input", received)
			}

		})
	}

}
