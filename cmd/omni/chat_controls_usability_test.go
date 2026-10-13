package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCancelDoesNotRequireExplanation(t *testing.T) {
	for _, reason := range []string{"", " \t ", "Stop this task."} {
		actual, err := validatedChatControlText("cancel", reason)
		if err != nil || strings.TrimSpace(actual) == "" {
			t.Fatalf("cancel reason %q = %q, error %v; want valid cancellation", reason, actual, err)
		}
		if reason == "Stop this task." && actual != reason {
			t.Fatalf("changed explicit cancellation reason %q", actual)
		}
	}
}

func TestStopCommandCancelsCurrentWorkAndIsSafeWhileIdle(t *testing.T) {
	for _, command := range []string{"/stop", "/cancel", "/cancel stop"} {
		console, output := planReviewTestConsole(t)
		var session *chatSession
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			snapshot := operationGuardActiveSnapshot(t, session)
			snapshot.ActiveJob = nil
			snapshot.Messages = []model.ChannelMessage{}
			snapshot.Turns = []model.ChannelSessionTurn{}
			snapshot.Controls = []model.ChannelSessionControl{}
			snapshot.State.LatestJob = nil
			writeOperationGuardJSON(t, w, snapshot)
		}))
		session = newChatOperationTestSession(t, server.URL)
		session.renderer = chatRenderer{console: console}
		quit, err := session.acceptInput(command, false)
		server.Close()
		if err != nil || quit || !strings.Contains(strings.ToLower(output.String()), "no active") {
			t.Fatalf("idle %q = quit %t, error %v, output %q", command, quit, err, output.String())
		}
	}
}
