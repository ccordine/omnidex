package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func TestConversationModeKeyboardReachesAuthoritativeSessionLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/realtime/ws":
			conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			<-ctx.Done()
		case strings.HasSuffix(r.URL.Path, "/session/research") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode([]sessionintent.ResearchTurn{{Question: "earlier question", Answer: "SESSION LOOP HISTORY"}})
		default:
			t.Errorf("mode selection dispatched unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	fixture := newChatOperationTestSession(t, server.URL)
	stream, err := fixture.client.OpenJobEvents(ctx, fixture.channel.ID, fixture.workspaceIdentity, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	console, keys := modeEditorConsole(t, ctx)
	result := make(chan error, 1)
	go func() {
		result <- runChatSession(chatSessionConfig{
			Context: ctx, Cancel: cancel, Client: fixture.client, Channel: fixture.channel,
			WorkspaceIdentity: fixture.workspaceIdentity, Stream: stream, Console: console, Signals: make(chan os.Signal),
			Snapshot: client.ChatSessionSnapshot{Channel: fixture.channel, WorkspaceIdentity: fixture.workspaceIdentity,
				State: model.ChannelSessionState{ChannelID: fixture.channel.ID, WorkspaceRoot: fixture.channel.WorkspaceRoot, WorkspaceIdentity: fixture.workspaceIdentity}},
		})
	}()
	// History is loaded only by the session's selected mode, never the editor.
	modeKeys(t, keys, "draft\x02r")
	waitMode(t, console, "research", "SESSION LOOP HISTORY")
	modeKeys(t, keys, "\x02f")
	waitMode(t, console, "infer", "draft")
	cancel()
	select {
	case err := <-result:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("session/input reader did not stop")
	}
}

func TestConversationModePendingApprovalRemainsVisibleOnReturn(t *testing.T) {
	console, keys := modeEditorConsole(t)
	if err := console.ShowPlanReview("Pending approval\n> selected task\nSpace decide"); err != nil {
		t.Fatal(err)
	}
	authority := console.CurrentInputAuthority()
	done := make(chan string, 1)
	go func() {
		line, _, _, err := console.ReadLine()
		if err != nil {
			done <- err.Error()
		} else {
			done <- line
		}
	}()
	modeKeys(t, keys, "\x02rseparate question\r")
	select {
	case line := <-done:
		if line != "separate question" {
			t.Fatal(line)
		}
	case <-time.After(time.Second):
		t.Fatal("switch blocked by pending approval")
	}
	if console.CurrentInputAuthority() != authority {
		t.Fatal("switch changed approval authority")
	}
	go func() {
		line, _, _, err := console.ReadLine()
		if err != nil {
			done <- err.Error()
		} else {
			done <- line
		}
	}()
	modeKeys(t, keys, "\x02f")
	waitMode(t, console, "infer", "selected task")
	if !console.reviewActive || !console.reviewInput.ReviewEnabled() {
		t.Fatal("approval state was cleared")
	}
}

func TestConversationModeRetainsViewportsAndLedgerSelection(t *testing.T) {
	console, keys := modeEditorConsole(t)
	if err := console.workspace.acceptLedger(panelLedgerFixture()); err != nil {
		t.Fatal(err)
	}
	if err := console.WriteOutput(strings.Repeat("scrollable transcript line\n", 50)); err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() { line, _, _, _ := console.ReadLine(); done <- line }()
	modeKeys(t, keys, "draft")
	waitMode(t, console, "infer", "draft")
	view := console.workspace
	view.mu.Lock()
	view.conversationOffset, view.activityOffset = 8, 3
	view.mu.Unlock()
	modeKeys(t, keys, "\x02eother")
	waitMode(t, console, "interact", "other")
	view.mu.Lock()
	if view.conversationOffset != 0 {
		t.Error("new view inherited another viewport")
	}
	view.conversationOffset = 2
	view.mu.Unlock()
	modeKeys(t, keys, "\x02lj")
	waitMode(t, console, "ledger", "panel-second")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		view.mu.Lock()
		ready := view.ledger.selected == "panel-second"
		view.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(time.Millisecond)
	}
	view.mu.Lock()
	selected := view.ledger.selected
	view.mu.Unlock()
	if selected != "panel-second" {
		t.Fatalf("keyboard selection = %s", selected)
	}
	if err := console.WriteOutput("new pending reply\n"); err != nil {
		t.Fatal(err)
	}
	modeKeys(t, keys, "\x02f")
	waitMode(t, console, "infer", "draft")
	view.mu.Lock()
	if view.conversationOffset != 9 || view.activityOffset != 3 {
		t.Error("return lost conversation/activity viewport")
	}
	view.mu.Unlock()
	modeKeys(t, keys, "\x02e")
	waitMode(t, console, "interact", "other")
	view.mu.Lock()
	if view.conversationOffset != 3 {
		t.Error("return lost independent viewport")
	}
	view.mu.Unlock()
	modeKeys(t, keys, "\x02l")
	waitMode(t, console, "ledger", "panel-second")
	view.mu.Lock()
	if view.ledger.selected != selected {
		t.Error("return lost ledger selection")
	}
	view.mu.Unlock()
}

func TestConversationModeSubmittedInputKeepsItsModeAcrossLaterSwitch(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request sessionintent.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Mode != sessionintent.Interact || request.Text != "already submitted" {
			t.Errorf("input rerouted after switch: %+v", request)
		}
		http.Error(w, "fixture stale", http.StatusConflict)
	}))
	defer server.Close()
	session := newChatOperationTestSession(t, server.URL)
	console, _ := modeEditorConsole(t)
	session.renderer = chatRenderer{console: console}
	console.workspace.conversationMode = "research"
	_, err := session.acceptAuthorizedInput(chatInput{Text: "already submitted", Mode: sessionintent.Interact, Authority: console.CurrentInputAuthority()})
	if err == nil || calls != 1 {
		t.Fatalf("expected one bound intake: %d %v", calls, err)
	}
	if session.mode != sessionintent.Research {
		t.Fatal("submitted input overwrote selected mode")
	}
}

func TestResearchSubmissionKeepsSessionLoopAndPaneSwitchingLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	started := make(chan struct{})
	interacted := make(chan sessionintent.Request, 1)
	release := make(chan struct{})
	var mu sync.Mutex
	var history []sessionintent.ResearchTurn
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/realtime/ws":
			conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			<-ctx.Done()
		case strings.HasSuffix(r.URL.Path, "/session/research") && r.Method == http.MethodGet:
			mu.Lock()
			defer mu.Unlock()
			json.NewEncoder(w).Encode(history)
		case strings.HasSuffix(r.URL.Path, "/session/research") && r.Method == http.MethodPost:
			var request sessionintent.ResearchRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				return
			}
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return
			}
			turn := sessionintent.ResearchTurn{OperationID: request.OperationID, Question: request.Text, Answer: "BACKGROUND RESEARCH FINISHED"}
			mu.Lock()
			history = append(history, turn)
			mu.Unlock()
			json.NewEncoder(w).Encode(turn)
		case strings.HasSuffix(r.URL.Path, "/session/intent") && r.Method == http.MethodPost:
			var request sessionintent.Request
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			interacted <- request
			json.NewEncoder(w).Encode(sessionintent.Result{OperationID: request.OperationID, Clarification: "INTERACT RESPONSE WHILE RESEARCH RUNS"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer cancel()
	fixture := newChatOperationTestSession(t, server.URL)
	stream, err := fixture.client.OpenJobEvents(ctx, fixture.channel.ID, fixture.workspaceIdentity, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	console, keys := modeEditorConsole(t, ctx)
	result := make(chan error, 1)
	go func() {
		result <- runChatSession(chatSessionConfig{Context: ctx, Cancel: cancel, Client: fixture.client, Channel: fixture.channel, WorkspaceIdentity: fixture.workspaceIdentity, Stream: stream, Console: console, Signals: make(chan os.Signal), Snapshot: client.ChatSessionSnapshot{Channel: fixture.channel, WorkspaceIdentity: fixture.workspaceIdentity, State: model.ChannelSessionState{ChannelID: fixture.channel.ID, WorkspaceRoot: fixture.channel.WorkspaceRoot, WorkspaceIdentity: fixture.workspaceIdentity}}})
	}()
	modeKeys(t, keys, "\x02rInspect the repository\r")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Research not submitted")
	}
	modeKeys(t, keys, "\x02fmain draft")
	waitMode(t, console, "infer", "main draft")
	modeKeys(t, keys, "\x02einteract draft")
	waitMode(t, console, "interact", "interact draft")
	modeKeys(t, keys, "\r")
	select {
	case request := <-interacted:
		if request.Mode != sessionintent.Interact || request.Text != "interact draft" {
			t.Fatal("changed concurrent interaction", request)
		}
	case <-time.After(time.Second):
		t.Fatal("Research blocked Interact submission")
	}
	waitMode(t, console, "interact", "INTERACT RESPONSE WHILE RESEARCH RUNS")
	close(release)
	modeKeys(t, keys, "\x02r")
	waitMode(t, console, "research", "BACKGROUND RESEARCH FINISHED")
	modeKeys(t, keys, "\x02f")
	waitMode(t, console, "infer", "main draft")
	cancel()
	select {
	case err := <-result:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("session did not stop")
	}
}
