package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func TestConversationModeResearchPaneAndLedgerControls(t *testing.T) {
	history := []sessionintent.ResearchTurn{{OperationID: "lifecycle_operation_old", Question: "Earlier research", Answer: "Separate source evidence"}}
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/session/research") {
			t.Error("research used main conversation", r.URL.Path)
		}
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(history)
			return
		}
		posts++
		var request sessionintent.ResearchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		turn := sessionintent.ResearchTurn{OperationID: request.OperationID, Question: request.Text, Answer: "New research evidence"}
		history = append(history, turn)
		_ = json.NewEncoder(w).Encode(turn)
	}))
	defer server.Close()
	session := newChatOperationTestSession(t, server.URL)
	console, _ := planReviewTestConsole(t)
	view, _ := testWorkspace(t, 100, 24)
	console.workspace = view
	session.renderer = chatRenderer{console: console}
	for _, r := range "Main conversation retained" {
		view.conversation.put(r)
	}
	if err := session.setConversationMode("Research"); err != nil {
		t.Fatal(err)
	}
	if err := session.acceptUserMessage("Keep approval words literal in this question."); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-session.researchResults:
		if err := session.completeResearch(result); err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("research did not finish")
	}
	frame, _, _, _ := view.frameLocked()
	if joined := strings.Join(frame, "\n"); !strings.Contains(joined, "New research evidence") || strings.Contains(joined, "Main conversation retained") {
		t.Fatal(joined)
	}
	if err := session.setConversationMode("Interact"); err != nil {
		t.Fatal(err)
	}
	frame, _, _, _ = view.frameLocked()
	if joined := strings.Join(frame, "\n"); !strings.Contains(joined, "Main conversation retained") || strings.Contains(joined, "New research evidence") {
		t.Fatal(joined)
	}
	if err := session.setConversationMode("Ledger"); err != nil {
		t.Fatal(err)
	}
	if view.focus != workspaceLedgerPane || !view.ledger.opened || session.mode != sessionintent.Ledger {
		t.Fatal("Ledger did not open explicit item controls")
	}
	if err := session.acceptUserMessage("Approve everything"); err == nil {
		t.Fatal("Ledger inferred a free text action")
	}
	if posts != 1 {
		t.Fatal("mode selection or Ledger text dispatched a question", posts)
	}
	if err := session.setConversationMode("Infer"); err != nil {
		t.Fatal(err)
	}
	if session.mode != sessionintent.Infer {
		t.Fatal("Infer not selected")
	}
	if err := session.setConversationMode("other"); err == nil {
		t.Fatal("invalid mode accepted")
	}
}

func TestInteractBindsKnownRecipientAndExactTextBeforeTransport(t *testing.T) {
	inputID := int64(9)
	for _, recipient := range []sessionintent.Recipient{sessionintent.Planner, sessionintent.Codex, sessionintent.Native, sessionintent.Approval} {
		t.Run(string(recipient), func(t *testing.T) {
			text := "  Keep /mode research literal.\nDo not approve.  "
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request sessionintent.Request
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Mode != sessionintent.Interact || request.Recipient != recipient || request.Text != text {
					t.Errorf("changed routing or owner text: %+v", request)
				}
				// Stop after intake so this test observes the actual request without
				// introducing unrelated execution/snapshot fixtures.
				http.Error(w, "stale fixture", http.StatusConflict)
			}))
			defer server.Close()
			session := newChatOperationTestSession(t, server.URL)
			session.mode = sessionintent.Interact
			console, _ := planReviewTestConsole(t)
			session.renderer = chatRenderer{console: console}
			switch recipient {
			case sessionintent.Planner:
				session.active = &model.JobDetails{Job: model.Job{Status: model.JobStatusWaiting, CurrentGeneration: 1}, Steps: []model.Step{{Action: codingPlanReviewStepAction, Generation: 1, Status: model.StepStatusWaiting}}}
			case sessionintent.Native:
				session.active = &model.JobDetails{ExecutionInputID: &inputID}
			case sessionintent.Approval:
				session.lastPendingIntent = "lifecycle_operation_pending"
			}
			if err := session.acceptUserMessage(text); err == nil || calls != 1 {
				t.Fatal("expected exactly one stale intake", calls, err)
			}
		})
	}
}

func TestConversationModeSwitchPreservesPendingAndReview(t *testing.T) {
	session := newChatOperationTestSession(t, "http://unused.invalid")
	console, _ := planReviewTestConsole(t)
	view, _ := testWorkspace(t, 28, 12)
	console.workspace = view
	session.renderer = chatRenderer{console: console}
	state := mustPlanReviewState(t, eligiblePlanReviewFixture(t))
	session.planReview = &state
	session.pendingResearch = &sessionintent.ResearchRequest{Text: "unresolved question"}
	pending := session.pendingResearch
	session.pendingIntent = &pendingUserIntent{}
	session.pendingControl = &pendingControl{}
	session.pendingTurn = &pendingSessionTurn{}
	session.pendingPlan = &pendingPlanMutation{}
	intent, control, turn, plan := session.pendingIntent, session.pendingControl, session.pendingTurn, session.pendingPlan
	review := session.planReview
	view.conversationOffset = 7
	view.activityOffset = 3
	for _, mode := range []string{"interact", "infer"} {
		if err := session.setConversationMode(mode); err != nil {
			t.Fatal(err)
		}
		if session.pendingResearch != pending || session.planReview != review || session.pendingIntent != intent || session.pendingControl != control || session.pendingTurn != turn || session.pendingPlan != plan {
			t.Fatal("switch discarded pending/review state")
		}
	}
	if view.conversationOffset != 7 || view.activityOffset != 3 {
		t.Fatal("switch reset scroll state")
	}
}

func TestConversationModeResearchReplyPreservesScroll(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			session := newChatOperationTestSession(t, "http://unused.invalid")
			console, _ := planReviewTestConsole(t)
			view, _ := testWorkspace(t, 36, 12)
			console.workspace = view
			session.renderer = chatRenderer{console: console}
			history := []sessionintent.ResearchTurn{{Question: "first", Answer: strings.Repeat("answer line\n", 30)}}
			if err := session.renderResearch(history); err != nil {
				t.Fatal(err)
			}
			if active {
				view.conversationMode = "research"
				view.conversationOffset = 5
			} else {
				view.modeOffsets["research"] = 5
			}
			left, _, _, _ := view.paneGeometryLocked()
			before := len(view.researchContentLocked(max(1, left-4)))
			history = append(history, sessionintent.ResearchTurn{Question: "second", Answer: strings.Repeat("new reply\n", 8)})
			if err := session.renderResearch(history); err != nil {
				t.Fatal(err)
			}
			want := 5 + len(view.researchContentLocked(max(1, left-4))) - before
			got := view.modeOffsets["research"]
			if active {
				got = view.conversationOffset
			}
			if got != want {
				t.Fatalf("scroll = %d, want %d", got, want)
			}
			if err := session.renderResearch(history); err != nil {
				t.Fatal(err)
			}
			if active {
				got = view.conversationOffset
			} else {
				got = view.modeOffsets["research"]
			}
			if got != want {
				t.Fatalf("unchanged history moved scroll: %d", got)
			}
		})
	}
}
