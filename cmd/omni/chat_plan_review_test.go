package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestChatPlanReviewPersistsDecisionThenDeliberatelyFreezes(t *testing.T) {
	workspaceRoot := "/tmp/cli-plan-review"
	const workspaceIdentity = "directory_identity_v1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const jobID int64 = 73
	plan := singleLeafPlanReviewFixture(t, jobID, 1)
	var mu sync.Mutex
	var session *chatSession
	decisionRequests := 0
	freezeRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch request.URL.Path {
		case "/v1/jobs/73/plan":
			query := request.URL.Query()
			if request.Method != http.MethodGet || len(query) != 2 ||
				len(query["workspace_root"]) != 1 || query.Get("workspace_root") != workspaceRoot ||
				len(query["workspace_identity"]) != 1 || query.Get("workspace_identity") != workspaceIdentity {
				t.Errorf("plan read authority = %s %s", request.Method, request.URL.String())
			}
			writeChatPlanReviewJSON(t, writer, http.StatusOK, plan)
		case "/v1/jobs/73/plan/decisions":
			decisionRequests++
			var body struct {
				OperationID       queue.LifecycleOperationID        `json:"operation_id"`
				Generation        int64                             `json:"generation"`
				Revision          int64                             `json:"revision"`
				WorkspaceRoot     string                            `json:"workspace_root"`
				WorkspaceIdentity string                            `json:"workspace_identity"`
				Decisions         []client.CodingPlanDecisionChange `json:"decisions"`
			}
			decodeChatPlanReviewJSON(t, request, &body)
			if _, err := queue.ParseLifecycleOperationID(string(body.OperationID)); err != nil {
				t.Errorf("decision operation ID: %v", err)
			}
			if body.Generation != plan.Generation || body.Revision != plan.Revision ||
				body.WorkspaceRoot != workspaceRoot || body.WorkspaceIdentity != workspaceIdentity ||
				len(body.Decisions) != 1 || body.Decisions[0].LeafID != plan.Leaves[0].ID ||
				body.Decisions[0].Decision != model.CodingPlanDecisionApproved {
				t.Errorf("decision body = %#v; plan = %#v", body, plan)
			}
			plan.Revision++
			plan.UpdatedAt = plan.UpdatedAt.Add(time.Second)
			plan.Leaves[0].Decision = model.CodingPlanDecisionApproved
			writeChatPlanReviewJSON(t, writer, http.StatusOK, plan)
		case "/v1/jobs/73/plan/freeze":
			freezeRequests++
			var body struct {
				OperationID       queue.LifecycleOperationID `json:"operation_id"`
				Generation        int64                      `json:"generation"`
				Revision          int64                      `json:"revision"`
				WorkspaceRoot     string                     `json:"workspace_root"`
				WorkspaceIdentity string                     `json:"workspace_identity"`
			}
			decodeChatPlanReviewJSON(t, request, &body)
			if _, err := queue.ParseLifecycleOperationID(string(body.OperationID)); err != nil {
				t.Errorf("freeze operation ID: %v", err)
			}
			if body.Generation != plan.Generation || body.Revision != plan.Revision ||
				body.WorkspaceRoot != workspaceRoot || body.WorkspaceIdentity != workspaceIdentity {
				t.Errorf("freeze body = %#v; plan = %#v", body, plan)
			}
			plan.Revision++
			plan.State = model.CodingPlanStateFrozen
			plan.UpdatedAt = plan.UpdatedAt.Add(time.Second)
			frozenAt := plan.UpdatedAt
			plan.FrozenAt = &frozenAt
			writeChatPlanReviewJSON(t, writer, http.StatusOK, client.CodingPlanFreezeReceipt{
				Plan: plan, JobStatus: model.JobStatusRunning,
			})
		default:
			if strings.HasPrefix(request.URL.Path, "/v1/channels/") &&
				strings.HasSuffix(request.URL.Path, "/session") {
				query, _ := url.ParseQuery(request.URL.RawQuery)
				if request.Method != http.MethodGet || query.Get("workspace_identity") != workspaceIdentity {
					t.Errorf("session reload authority = %s %s", request.Method, request.URL.String())
				}
				writeChatPlanReviewJSON(
					t, writer, http.StatusOK,
					chatPlanReviewSnapshot(t, session, model.JobStatusRunning, 1, nil),
				)
				return
			}
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
	mu.Lock()
	mu.Unlock()

	if err := session.reconcilePlanReview(); err != nil {
		t.Fatalf("discover plan review: %v", err)
	}
	if session.planReview == nil || !console.reviewActive {
		t.Fatal("authoritative waiting plan did not enter interactive review")
	}
	if err := session.acceptPlanReviewKey(planReviewKeyToggle); err != nil {
		t.Fatalf("approve selected leaf: %v", err)
	}
	if got := session.planReview.snapshot.Leaves[0].Decision; got != model.CodingPlanDecisionApproved {
		t.Fatalf("persisted decision = %q, want approved", got)
	}
	if err := session.acceptPlanReviewKey(planReviewKeyEnter); err != nil {
		t.Fatalf("open freeze confirmation: %v", err)
	}
	mu.Lock()
	firstFreezeCount := freezeRequests
	mu.Unlock()
	if firstFreezeCount != 0 || !session.planReview.confirming {
		t.Fatalf("first Enter froze=%d confirming=%t; want deliberate confirmation", firstFreezeCount, session.planReview.confirming)
	}
	if err := session.acceptPlanReviewKey(planReviewKeyEnter); err != nil {
		t.Fatalf("confirm plan freeze: %v", err)
	}
	mu.Lock()
	actualDecisionRequests := decisionRequests
	actualFreezeRequests := freezeRequests
	mu.Unlock()
	if actualDecisionRequests != 1 || actualFreezeRequests != 1 {
		t.Fatalf("requests = decisions %d freeze %d, want one each", actualDecisionRequests, actualFreezeRequests)
	}
	if session.pendingPlan != nil || session.planReview != nil || console.reviewActive {
		t.Fatalf(
			"frozen review retained client authority: pending=%#v review=%#v screen=%t",
			session.pendingPlan,
			session.planReview,
			console.reviewActive,
		)
	}
	if session.active == nil || session.active.Job.Status != model.JobStatusRunning {
		t.Fatalf("coding did not resume after freeze: %#v", session.active)
	}
}

func TestChatPlanReviewZeroLeafPlanRequestsGuidanceWithoutMutation(t *testing.T) {
	const jobID int64 = 74
	plan := singleLeafPlanReviewFixture(t, jobID, 1)
	plan.Leaves = []model.CodingPlanLeaf{}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/v1/jobs/74/plan" {
			t.Errorf("unexpected zero-leaf plan request %s %s", request.Method, request.URL.String())
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
			return
		}
		writeChatPlanReviewJSON(t, writer, http.StatusOK, plan)
	}))
	defer server.Close()

	session := newChatOperationTestSession(t, server.URL)
	session.active = waitingPlanReviewJob(t, session, jobID, 1)
	console, output := planReviewTestConsole(t)
	session.renderer = chatRenderer{console: console}
	if err := session.reconcilePlanReview(); err != nil {
		t.Fatalf("discover zero-leaf plan review: %v", err)
	}
	if session.planReview == nil || !console.reviewActive ||
		!strings.Contains(output.String(), "Guidance is required before coding can start.") {
		t.Fatalf("zero-leaf plan did not enter visible guidance review: state=%#v output=%q", session.planReview, output.String())
	}
	if err := session.acceptPlanReviewKey(planReviewKeyToggle); err != nil {
		t.Fatalf("ignore zero-leaf toggle: %v", err)
	}
	if err := session.acceptPlanReviewKey(planReviewKeyEnter); err != nil {
		t.Fatalf("ignore zero-leaf freeze: %v", err)
	}
	if session.planReview.confirming || session.pendingPlan != nil || requests != 1 {
		t.Fatalf(
			"zero-leaf mutation escaped guidance state: confirming=%t pending=%#v requests=%d",
			session.planReview.confirming,
			session.pendingPlan,
			requests,
		)
	}
	armPlanReviewNoteTransition(t, console)
	if err := session.acceptPlanReviewKey(planReviewKeyNote); err != nil {
		t.Fatalf("open zero-leaf guidance note: %v", err)
	}
	if !session.planNoteEditing || console.reviewActive || session.planNoteSubject != "" {
		t.Fatalf(
			"zero-leaf note state = editing %t review %t subject %q",
			session.planNoteEditing,
			console.reviewActive,
			session.planNoteSubject,
		)
	}
	if err := session.acceptPlanReviewNote(""); err != nil {
		t.Fatalf("cancel zero-leaf note: %v", err)
	}
	if session.planNoteEditing || session.planNoteSubject != "" || !console.reviewActive || requests != 1 {
		t.Fatalf(
			"zero-leaf note cancel state = editing %t subject %q review %t requests %d",
			session.planNoteEditing,
			session.planNoteSubject,
			console.reviewActive,
			requests,
		)
	}
}

func TestChatPlanReviewClearsStaleWaitingObservationAfterConcurrentFreeze(t *testing.T) {
	const jobID int64 = 75
	plan := singleLeafPlanReviewFixture(t, jobID, 1)
	plan.Leaves[0].Decision = model.CodingPlanDecisionApproved
	plan.State = model.CodingPlanStateFrozen
	plan.Revision++
	plan.UpdatedAt = plan.UpdatedAt.Add(time.Second)
	plan.FrozenAt = &plan.UpdatedAt
	planRequests := 0
	sessionRequests := 0
	var session *chatSession
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/jobs/75/plan":
			planRequests++
			writeChatPlanReviewJSON(t, writer, http.StatusOK, plan)
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/channels/") &&
			strings.HasSuffix(request.URL.Path, "/session"):
			sessionRequests++
			writeChatPlanReviewJSON(
				t, writer, http.StatusOK,
				chatPlanReviewSnapshot(t, session, model.JobStatusRunning, 1, nil),
			)
		default:
			t.Errorf("unexpected stale-freeze request %s %s", request.Method, request.URL.String())
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	session = newChatOperationTestSession(t, server.URL)
	session.active = waitingPlanReviewJob(t, session, jobID, 1)
	console, _ := planReviewTestConsole(t)
	session.renderer = chatRenderer{console: console}
	if err := session.reconcilePlanReview(); err != nil {
		t.Fatalf("reconcile concurrently frozen plan: %v", err)
	}
	if planRequests != 1 || sessionRequests != 1 || session.planReview != nil ||
		console.reviewActive || session.active == nil ||
		session.active.Job.Status != model.JobStatusRunning {
		t.Fatalf(
			"stale freeze reconciliation = plan requests %d session requests %d review %#v active %#v",
			planRequests,
			sessionRequests,
			session.planReview,
			session.active,
		)
	}
}

func TestChatPlanReviewReloadsOnceForConcurrentReplanGeneration(t *testing.T) {
	const jobID int64 = 76
	plan := singleLeafPlanReviewFixture(t, jobID, 2)
	planRequests := 0
	sessionRequests := 0
	var session *chatSession
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/jobs/76/plan":
			planRequests++
			writeChatPlanReviewJSON(t, writer, http.StatusOK, plan)
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/channels/") &&
			strings.HasSuffix(request.URL.Path, "/session"):
			sessionRequests++
			writeChatPlanReviewJSON(
				t, writer, http.StatusOK,
				chatPlanReviewSnapshot(t, session, model.JobStatusWaiting, 2, nil),
			)
		default:
			t.Errorf("unexpected concurrent-replan request %s %s", request.Method, request.URL.String())
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	session = newChatOperationTestSession(t, server.URL)
	session.active = waitingPlanReviewJob(t, session, jobID, 1)
	console, _ := planReviewTestConsole(t)
	session.renderer = chatRenderer{console: console}
	if err := session.reconcilePlanReview(); err != nil {
		t.Fatalf("reconcile concurrent plan generation: %v", err)
	}
	if planRequests != 2 || sessionRequests != 1 || session.planReview == nil ||
		session.planReview.snapshot.Generation != 2 || !console.reviewActive ||
		session.active == nil || session.active.Job.CurrentGeneration != 2 {
		t.Fatalf(
			"concurrent replan reconciliation = plan requests %d session requests %d review %#v active %#v",
			planRequests,
			sessionRequests,
			session.planReview,
			session.active,
		)
	}
}

func TestActiveJobRequiresPlanReviewUsesExactWaitingStepAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		job    string
		action string
		step   string
		want   bool
		bad    bool
	}{
		{name: "exact", job: model.JobStatusWaiting, action: codingPlanReviewStepAction, step: model.StepStatusWaiting, want: true},
		{name: "ordinary wait", job: model.JobStatusWaiting, action: "v3_coding", step: model.StepStatusWaiting},
		{name: "running plan", job: model.JobStatusRunning, action: codingPlanReviewStepAction, step: model.StepStatusRunning},
		{name: "contradictory", job: model.JobStatusRunning, action: codingPlanReviewStepAction, step: model.StepStatusWaiting, bad: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			details := &model.JobDetails{
				Job: model.Job{ID: 1, Status: test.job, CurrentGeneration: 1},
				Steps: []model.Step{{
					ID: 1, JobID: 1, Generation: 1, Action: test.action, Status: test.step,
				}},
			}
			actual, err := activeJobRequiresPlanReview(details)
			if test.bad {
				if err == nil {
					t.Fatal("contradictory plan authority was accepted")
				}
				return
			}
			if err != nil || actual != test.want {
				t.Fatalf("activeJobRequiresPlanReview() = %t, %v; want %t", actual, err, test.want)
			}
		})
	}
}
