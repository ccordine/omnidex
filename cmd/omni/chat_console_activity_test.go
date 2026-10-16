package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func TestChatActivityKeepsVisibleElapsedWorkAndStopsWhenWaiting(t *testing.T) {
	console, _ := planReviewTestConsole(t)
	started := time.Now().Add(-2 * time.Minute)
	activity := chatActivity{JobID: 41, Generation: 1, StepID: 12, Label: "Planning", Working: true, StartedAt: started}
	if err := console.SetActivity(activity); err != nil {
		t.Fatal(err)
	}
	if err := console.SetPrompt("you #41> "); err != nil {
		t.Fatal(err)
	}
	if err := console.SetActivityDetail(41, 12, "Checking requirements"); err != nil {
		t.Fatal(err)
	}
	if err := console.PulseActivity(started.Add(125 * time.Second)); err != nil {
		t.Fatal(err)
	}
	before := console.renderedPrompt
	for _, required := range []string{"Planning", "Checking requirements", "2m5s", "you #41> "} {
		if !strings.Contains(before, required) {
			t.Errorf("activity omitted %q: %q", required, before)
		}
	}
	if err := console.PulseActivity(started.Add(126 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if console.renderedPrompt == before {
		t.Fatal("active work had no live indicator")
	}
	if err := console.SetActivityDetail(99, 12, "stale work"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(console.renderedPrompt, "stale work") {
		t.Fatal("another job replaced current activity")
	}
	activity.Label, activity.Working = "Waiting for you", false
	if err := console.SetActivity(activity); err != nil {
		t.Fatal(err)
	}
	before = console.renderedPrompt
	if err := console.PulseActivity(started.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if console.renderedPrompt != before || !strings.Contains(before, "Waiting for you") || strings.Contains(before, "Checking requirements") {
		t.Fatalf("waiting activity kept animating or retained stale detail: %q", console.renderedPrompt)
	}
	if err := (chatRenderer{console: console}).prompt(nil); err != nil {
		t.Fatal(err)
	}
	if console.renderedPrompt != "you> " {
		t.Fatalf("idle composer retained work: %q", console.renderedPrompt)
	}
}

func TestChatActivityPreservesPlanReviewAndRetiresDisplayLoop(t *testing.T) {
	console, output := planReviewTestConsole(t)
	if err := console.SetActivity(chatActivity{JobID: 41, Label: "Planning", Working: true}); err != nil {
		t.Fatal(err)
	}
	if err := console.ShowPlanReview("Choose a plan\n"); err != nil {
		t.Fatal(err)
	}
	before := output.Len()
	if err := console.PulseActivity(time.Now()); err != nil {
		t.Fatal(err)
	}
	if output.Len() != before {
		t.Fatal("animation overwrote the plan-review screen")
	}
	if err := console.HidePlanReview(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(console.renderedPrompt, "Planning") {
		t.Fatal("plan review lost the work indicator")
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, stop := startChatActivityDisplay(ctx, console)
	cancel()
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("closed chat retained its display loop")
	}
}

func TestChatVerbosityCommandsChangeOnlyPresentation(t *testing.T) {
	console, output := planReviewTestConsole(t)
	session := &chatSession{renderer: chatRenderer{console: console}}
	for _, level := range []string{"quiet", "normal", "verbose", "debug"} {
		if quit, err := session.acceptInput("/verbosity "+level, false); quit || err != nil {
			t.Fatalf("verbosity %q = quit %t, error %v", level, quit, err)
		}
		if session.renderer.verbosity.String() != level || !strings.Contains(output.String(), "Verbosity: "+level) {
			t.Fatalf("verbosity %q had no consumer or feedback", level)
		}
	}
	if _, err := session.acceptInput("/verbosity arbitrary", false); err == nil || session.renderer.verbosity != chatDebug {
		t.Fatal("invalid verbosity changed accepted presentation")
	}
	if session.pendingTurn != nil || session.pendingControl != nil {
		t.Fatal("presentation change created runtime work")
	}
}

func TestCancelAndStopSubmitCurrentBoundControlWithoutReason(t *testing.T) {
	for _, test := range []struct {
		name, command string
		planning      bool
	}{
		{"cancel", "/cancel", false}, {"stop", "/stop", false},
		{"planning cancel", "/cancel", true}, {"planning stop", "/stop", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var session *chatSession
			var snapshot client.ChatSessionSnapshot
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					writeOperationGuardJSON(t, w, snapshot)
					return
				}
				posts++
				var body struct {
					Reason            string                     `json:"reason"`
					WorkspaceIdentity string                     `json:"workspace_identity"`
					OperationID       model.LifecycleOperationID `json:"operation_id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.URL.Path != "/v1/jobs/41/cancel" || body.Reason != defaultChatCancelReason || body.WorkspaceIdentity != session.workspaceIdentity || body.OperationID == "" {
					t.Errorf("cancel lost exact current scope: path %s payload %+v", r.URL.Path, body)
				}
				http.Error(w, "fixture refused mutation", http.StatusConflict)
			}))
			defer server.Close()
			session = newChatOperationTestSession(t, server.URL)
			session.renderer = operationGuardRenderer()
			snapshot = operationGuardActiveSnapshot(t, session)
			if err := session.reloadSnapshot(); err != nil {
				t.Fatal(err)
			}
			if test.planning {
				console, _ := planReviewTestConsole(t)
				session.renderer.console = console
				authority, err := console.BeginPlanReviewNote()
				if err != nil {
					t.Fatal(err)
				}
				session.planNoteAuthority, session.planNoteEditing = authority, true
			}
			quit, err := session.acceptInput(test.command, false)
			if quit || !client.IsHTTPStatus(err, http.StatusConflict) || posts != 1 {
				t.Fatalf("cancel = quit %t, posts %d, error %v", quit, posts, err)
			}
			if session.pendingControl != nil {
				t.Fatal("definitive rejection retained a cancellation")
			}
			if session.pendingTurn != nil || session.planNoteSubmitting || session.planNoteEditing {
				t.Fatal("typed stop became planning work or stranded the input mode")
			}
		})
	}
}

func TestPlanningExitRemainsATypedCommand(t *testing.T) {
	session := &chatSession{planNoteEditing: true}
	quit, err := session.acceptInput("/exit", false)
	if err != nil || !quit || session.pendingTurn != nil || session.pendingControl != nil {
		t.Fatalf("planning /exit was reinterpreted as feedback: quit=%v error=%v", quit, err)
	}
}
