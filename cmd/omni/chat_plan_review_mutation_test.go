package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestChatPlanReviewDecisionReusesOperationAfterAmbiguousTransport(t *testing.T) {
	const jobID int64 = 96
	plan := singleLeafPlanReviewFixture(t, jobID, 1)
	var guard sync.Mutex
	requests := 0
	var firstOperation queue.LifecycleOperationID
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		guard.Lock()
		defer guard.Unlock()
		requests++
		var body struct {
			OperationID       queue.LifecycleOperationID        `json:"operation_id"`
			Generation        int64                             `json:"generation"`
			Revision          int64                             `json:"revision"`
			WorkspaceRoot     string                            `json:"workspace_root"`
			WorkspaceIdentity string                            `json:"workspace_identity"`
			Decisions         []client.CodingPlanDecisionChange `json:"decisions"`
		}
		decodeChatPlanReviewJSON(t, request, &body)
		if requests == 1 {
			firstOperation = body.OperationID
			dropOperationGuardConnection(t, writer)
			return
		}
		if body.OperationID != firstOperation {
			t.Errorf("ambiguous decision operation changed from %q to %q", firstOperation, body.OperationID)
		}
		plan.Revision++
		plan.UpdatedAt = plan.UpdatedAt.Add(time.Second)
		plan.Leaves[0].Decision = model.CodingPlanDecisionApproved
		writeChatPlanReviewJSON(t, writer, http.StatusOK, plan)
	}))
	defer server.Close()

	session := newChatOperationTestSession(t, server.URL)
	console, _ := planReviewTestConsole(t)
	session.renderer = chatRenderer{console: console}
	state := mustPlanReviewState(t, plan)
	session.planReview = &state
	if err := session.showPlanReview(); err != nil {
		t.Fatal(err)
	}

	err := session.acceptPlanReviewKey(planReviewKeyToggle)
	if err == nil || definitiveChatRequestFailure(err) {
		t.Fatalf("dropped decision response error = %v, want ambiguous transport", err)
	}
	guard.Lock()
	retainedOperation := firstOperation
	guard.Unlock()
	if session.pendingPlan == nil || session.pendingPlan.operationID != retainedOperation {
		t.Fatalf("ambiguous decision lost operation guard: %#v", session.pendingPlan)
	}
	if err := session.acceptPlanReviewKey(planReviewKeyToggle); err != nil {
		t.Fatalf("replay exact decision: %v", err)
	}
	guard.Lock()
	actualRequests := requests
	guard.Unlock()
	if actualRequests != 2 || session.pendingPlan != nil ||
		session.planReview.snapshot.Leaves[0].Decision != model.CodingPlanDecisionApproved {
		t.Fatalf(
			"replayed decision state = requests %d pending %#v decision %q",
			actualRequests,
			session.pendingPlan,
			session.planReview.snapshot.Leaves[0].Decision,
		)
	}
}
