package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
)

func TestChatPlanReviewNoteUsesUserInterpreterBeforePlanning(t *testing.T) {
	const note = "Keep confirmation local and remove the speculative export leaf."
	calls := 0
	var session *chatSession
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.HasSuffix(r.URL.Path, "/session/intent") {
			t.Errorf("message bypassed the user interpreter: %s", r.URL.Path)
			http.Error(w, "wrong route", 500)
			return
		}
		var request sessionintent.Request
		decodeChatPlanReviewJSON(t, r, &request)
		if request.Text != note || request.LedgerRevision != 1 || request.SelectedItem != model.LedgerItemID(session.planReview.snapshot.Leaves[0].ID) {
			t.Errorf("lost exact note/revision/selected-item context: %+v", request)
		}
		writeChatPlanReviewJSON(t, w, http.StatusOK, sessionintent.Result{OperationID: request.OperationID, Patch: []userintent.Patch{{Op: "replace", Path: "/items/0/disposition", Value: json.RawMessage(`"tombstoned"`)}}, RequiresApproval: true, Pitch: "I will remove the selected plan item. Proceed?"})
	}))
	defer server.Close()
	session = newChatOperationTestSession(t, server.URL)
	session.active = waitingPlanReviewJob(t, session, 84, 1)
	console, _ := planReviewTestConsole(t)
	session.renderer = chatRenderer{console: console}
	plan := singleLeafPlanReviewFixture(t, 84, 1)
	state := mustPlanReviewState(t, plan)
	session.planReview = &state
	if err := session.showPlanReview(); err != nil {
		t.Fatal(err)
	}
	armPlanReviewNoteTransition(t, console)
	if err := session.acceptPlanReviewKey(planReviewKeyNote); err != nil {
		t.Fatal(err)
	}
	if err := session.acceptPlanReviewNote(note); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !session.planNoteEditing || session.planNoteSubmitting || session.pendingIntent != nil || session.active.Job.CurrentGeneration != 1 {
		t.Fatalf("pitch changed workflow or input authority: calls=%d session=%+v", calls, session)
	}
}

func TestPlanReviewNoteSubjectUsesOnlyAuthoritativeSemanticStatement(t *testing.T) {
	t.Parallel()

	plan := singleLeafPlanReviewFixture(t, 91, 1)
	subject, err := planReviewNoteSubject(plan, plan.Leaves[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if subject != plan.Leaves[0].Statement || strings.Contains(subject, string(plan.Leaves[0].ID)) {
		t.Fatalf("selected plan-note subject = %q", subject)
	}

	otherID, err := model.NewCodingPlanLeafID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planReviewNoteSubject(plan, otherID); err == nil ||
		!strings.Contains(err.Error(), "absent from the authoritative review") {
		t.Fatalf("absent selected leaf error = %v", err)
	}
}
