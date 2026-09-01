package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestChatPlanReviewNoteReplansTheSamePersistedJob(t *testing.T) {
	const workspaceIdentity = "directory_identity_v1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const jobID int64 = 84
	const note = "Keep confirmation local and remove the speculative export leaf."
	const selectedStatement = "The software lets a user confirm the item."
	expectedFeedback, err := planReviewNoteFeedback(selectedStatement, note)
	if err != nil {
		t.Fatal(err)
	}
	workspaceRoot := "/tmp/cli-plan-review-note"
	var guard sync.Mutex
	var session *chatSession
	var operationID queue.LifecycleOperationID
	sessionReads := 0
	replanRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		guard.Lock()
		defer guard.Unlock()
		switch {
		case strings.HasPrefix(request.URL.Path, "/v1/channels/") &&
			strings.HasSuffix(request.URL.Path, "/session"):
			sessionReads++
			status := model.JobStatusWaiting
			generation := int64(1)
			var controls []queue.ChannelSessionControl
			if replanRequests > 0 {
				status = model.JobStatusRunning
				generation = 2
				controls = []queue.ChannelSessionControl{{
					OperationID: operationID, JobID: jobID, Generation: generation,
					Kind: queue.ChannelSessionControlReplan, Text: expectedFeedback,
					Status:    model.JobStatusRunning,
					CreatedAt: time.Date(2026, time.September, 1, 15, 0, 3, 0, time.UTC),
				}}
			}
			snapshot := chatPlanReviewSnapshot(t, session, status, generation, controls)
			if generation == 2 {
				snapshot.Revision = "channel_session_revision_" + strings.Repeat("c", 64)
			}
			writeChatPlanReviewJSON(t, writer, http.StatusOK, snapshot)
		case request.URL.Path == "/v1/jobs/84/replan":
			replanRequests++
			var body struct {
				OperationID       queue.LifecycleOperationID `json:"operation_id"`
				Feedback          string                     `json:"feedback"`
				WorkspaceRoot     string                     `json:"workspace_root"`
				WorkspaceIdentity string                     `json:"workspace_identity"`
			}
			decodeChatPlanReviewJSON(t, request, &body)
			if _, err := queue.ParseLifecycleOperationID(string(body.OperationID)); err != nil {
				t.Errorf("replan operation ID: %v", err)
			}
			operationID = body.OperationID
			if body.Feedback != expectedFeedback || body.WorkspaceRoot != workspaceRoot ||
				body.WorkspaceIdentity != workspaceIdentity {
				t.Errorf("replan body = %#v", body)
			}
			if strings.Contains(body.Feedback, "coding_plan_leaf_") {
				t.Errorf("replan feedback exposed framework leaf identity: %q", body.Feedback)
			}
			writeChatPlanReviewJSON(t, writer, http.StatusOK, map[string]any{
				"job_id": jobID, "operation_id": operationID,
				"status": model.JobStatusRunning,
			})
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.String())
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	session = newChatOperationTestSession(t, server.URL)
	session.channel.WorkspaceRoot = workspaceRoot
	session.workspaceIdentity = workspaceIdentity
	session.active = waitingPlanReviewJob(t, session, jobID, 1)
	console, _ := planReviewTestConsole(t)
	session.renderer = chatRenderer{console: console}
	plan := singleLeafPlanReviewFixture(t, jobID, 1)
	state := mustPlanReviewState(t, plan)
	session.planReview = &state
	guard.Lock()
	guard.Unlock()
	if err := session.showPlanReview(); err != nil {
		t.Fatal(err)
	}

	armPlanReviewNoteTransition(t, console)
	if err := session.acceptPlanReviewKey(planReviewKeyNote); err != nil {
		t.Fatalf("open plan note: %v", err)
	}
	if !session.planNoteEditing || console.reviewActive || session.planNoteSubject != selectedStatement {
		t.Fatalf(
			"note editor state = editing %t review screen %t subject %q",
			session.planNoteEditing,
			console.reviewActive,
			session.planNoteSubject,
		)
	}
	if session.planReview.snapshot.Leaves[0].Decision != plan.Leaves[0].Decision {
		t.Fatalf("opening a selected-leaf note changed its persisted decision")
	}
	if err := session.acceptPlanReviewNote(note); err != nil {
		t.Fatalf("submit plan note: %v", err)
	}
	guard.Lock()
	actualReplanRequests := replanRequests
	actualSessionReads := sessionReads
	persistedOperationID := operationID
	guard.Unlock()
	if actualReplanRequests != 1 || actualSessionReads != 3 {
		t.Fatalf("note transport = replan %d session reads %d, want 1 and 3", actualReplanRequests, actualSessionReads)
	}
	if session.active == nil || session.active.Job.ID != jobID ||
		session.active.Job.CurrentGeneration != 2 || session.active.Job.Status != model.JobStatusRunning {
		t.Fatalf("note changed same-job generation authority incorrectly: %#v", session.active)
	}
	if session.planReview != nil || session.planNoteEditing || session.planNoteSubmitting ||
		session.planNoteSubject != "" ||
		session.pendingControl != nil {
		t.Fatalf(
			"note retained stale client state: plan=%#v editing=%t submitting=%t subject=%q pending=%#v",
			session.planReview,
			session.planNoteEditing,
			session.planNoteSubmitting,
			session.planNoteSubject,
			session.pendingControl,
		)
	}
	control, exists := session.controls[persistedOperationID]
	if !exists || control.JobID != jobID || control.Generation != 2 || control.Text != expectedFeedback {
		t.Fatalf("same-job replan is absent from persisted session controls: %#v", control)
	}
}

func TestPlanReviewNoteFeedbackLeavesGlobalNotesUnchanged(t *testing.T) {
	t.Parallel()

	const note = "Generate a new plan with a clearer interaction boundary."
	feedback, err := planReviewNoteFeedback("", note)
	if err != nil {
		t.Fatal(err)
	}
	if feedback != note {
		t.Fatalf("global plan note = %q, want exact self-contained note %q", feedback, note)
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

	otherID, err := model.NewCodingPlanLeafID("An unrelated outcome.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planReviewNoteSubject(plan, otherID); err == nil ||
		!strings.Contains(err.Error(), "absent from the authoritative review") {
		t.Fatalf("absent selected leaf error = %v", err)
	}
}

func TestPlanReviewNoteFeedbackFailsInsteadOfTruncatingBoundContext(t *testing.T) {
	t.Parallel()

	_, err := planReviewNoteFeedback(
		strings.Repeat("s", model.MaxCodingPlanStatementBytes),
		strings.Repeat("n", 2*1024),
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds the 2048-byte bound") {
		t.Fatalf("oversized selected-leaf note error = %v", err)
	}
}
