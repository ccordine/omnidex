package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestTerminalDistinguishesChecksFromWaitingQuestions(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	if err := renderer.realtime(semanticPlanningJSONEvent(t, "semantic_started", "")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "check> ") || strings.Contains(output.String(), "omni> ") {
		t.Fatalf("semantic observation looks like a user question: %q", output.String())
	}
	output.Reset()
	details := questionTestDetails()
	if err := renderer.job(details, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "question> ") || !strings.Contains(output.String(), "Waiting for your answer.") {
		t.Fatalf("durable question lacks an explicit waiting state: %q", output.String())
	}
	if err := renderer.prompt(&details); err != nil {
		t.Fatal(err)
	}
	if console.prompt != "answer #41> " || console.activity.Working {
		t.Fatalf("question prompt/activity = %q %#v", console.prompt, console.activity)
	}
}

func TestChatAnswerUsesObservedQuestionAuthority(t *testing.T) {
	var session *chatSession
	var operation model.LifecycleOperationID
	var answer string
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writeOperationGuardJSON(t, writer, answeredQuestionSnapshot(t, session, operation, answer))
			return
		}
		requests++
		if request.URL.Path != "/v1/jobs/41/feedback" {
			t.Errorf("answer used %s instead of the observed question", request.URL.Path)
			writer.WriteHeader(http.StatusConflict)
			return
		}
		var body struct {
			OperationID    model.LifecycleOperationID `json:"operation_id"`
			Feedback       string                     `json:"feedback"`
			Generation     int64                      `json:"expected_generation"`
			WaitingAttempt int64                      `json:"expected_waiting_attempt"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Generation != 2 || body.WaitingAttempt != 3 {
			t.Errorf("answer authority = generation %d attempt %d", body.Generation, body.WaitingAttempt)
		}
		operation, answer = body.OperationID, body.Feedback
		writeOperationGuardJSON(t, writer, map[string]any{"job_id": 41, "operation_id": operation, "status": model.JobStatusRunning})
	}))
	defer server.Close()
	session = newChatOperationTestSession(t, server.URL)
	session.renderer = operationGuardRenderer()
	details := questionTestDetails()
	session.active = &details
	if err := session.acceptText("Keep the approved value."); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || session.pendingTurn != nil || session.active.Job.ID != 41 || session.active.Job.CurrentGeneration != 2 {
		t.Fatalf("answer changed session authority: requests=%d pending=%#v active=%#v", requests, session.pendingTurn, session.active)
	}
}

func TestChatAnswerRetryRetainsQuestionAfterAnotherPause(t *testing.T) {
	var session *chatSession
	var operation model.LifecycleOperationID
	requests := 0
	const answer = "Keep the approved value."
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			snapshot := answeredQuestionSnapshot(t, session, operation, answer)
			snapshot.ActiveJob.Job.Status = model.JobStatusWaiting
			snapshot.ActiveJob.Steps[0].Status = model.StepStatusWaiting
			pause := *questionTestDetails().AgentPause
			pause.WaitingAttempt = 4
			pause.Questions = []string{"Which next value should be used?"}
			snapshot.ActiveJob.AgentPause = &pause
			snapshot.State.LatestJob.Status = model.JobStatusWaiting
			snapshot.Messages[0].Turn.Status = model.JobStatusWaiting
			writeOperationGuardJSON(t, writer, snapshot)
			return
		}
		requests++
		if request.URL.Path != "/v1/jobs/41/feedback" {
			t.Errorf("answer retry used %s", request.URL.Path)
			writer.WriteHeader(http.StatusConflict)
			return
		}
		var body struct {
			OperationID    model.LifecycleOperationID `json:"operation_id"`
			Generation     int64                      `json:"expected_generation"`
			WaitingAttempt int64                      `json:"expected_waiting_attempt"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Generation != 2 || body.WaitingAttempt != 3 {
			t.Errorf("retry rebound to a different question: %#v", body)
		}
		if requests == 1 {
			operation = body.OperationID
			dropOperationGuardConnection(t, writer)
			return
		}
		if body.OperationID != operation {
			t.Errorf("retry changed operation %q to %q", operation, body.OperationID)
		}
		writeOperationGuardJSON(t, writer, map[string]any{"job_id": 41, "operation_id": operation, "status": model.JobStatusRunning})
	}))
	defer server.Close()
	session = newChatOperationTestSession(t, server.URL)
	session.renderer = operationGuardRenderer()
	details := questionTestDetails()
	session.active = &details
	if err := session.acceptText(answer); err == nil || definitiveChatRequestFailure(err) {
		t.Fatalf("dropped answer error = %v", err)
	}
	newPause := *details.AgentPause
	newPause.WaitingAttempt = 4
	newPause.Questions = []string{"Which next value should be used?"}
	session.active.AgentPause = &newPause
	if err := session.acceptText(answer); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || session.pendingTurn != nil || session.active.AgentPause.WaitingAttempt != 4 {
		t.Fatalf("historical answer consumed the new question: requests=%d pending=%#v pause=%#v", requests, session.pendingTurn, session.active.AgentPause)
	}
}

func TestChatAnswerReconciliationRequiresSameQuestionJobAndGeneration(t *testing.T) {
	for _, field := range []string{"job", "generation", "disposition"} {
		t.Run(field, func(t *testing.T) {
			session := newChatOperationTestSession(t, "http://127.0.0.1:1")
			details := questionTestDetails()
			session.active = &details
			operation, err := session.sessionTurnOperationID("Keep the approved value.")
			if err != nil {
				t.Fatal(err)
			}
			turn := model.ChannelSessionTurn{OperationID: operation, Text: "Keep the approved value.", JobID: 41, Generation: 2, Disposition: model.ChannelSessionTurnFeedback}
			switch field {
			case "job":
				turn.JobID++
			case "generation":
				turn.Generation++
			case "disposition":
				turn.Disposition = model.ChannelSessionTurnReplanned
			}
			session.turns[operation] = turn
			if err := session.reconcilePersistedPendingOperations(); err == nil || session.pendingTurn == nil {
				t.Fatalf("contradictory persisted %s resolved the answer: pending=%#v error=%v", field, session.pendingTurn, err)
			}
		})
	}
}

func TestChatStaleAnswerDoesNotBecomeReplan(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodPost || request.URL.Path != "/v1/jobs/41/feedback" {
			t.Errorf("stale answer changed its route: %s %s", request.Method, request.URL.Path)
		}
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte(`{"error":"The displayed question is no longer waiting."}`))
	}))
	defer server.Close()
	session := newChatOperationTestSession(t, server.URL)
	session.renderer = operationGuardRenderer()
	details := questionTestDetails()
	session.active = &details
	err := session.acceptText("Keep the approved value.")
	if !client.IsHTTPStatus(err, http.StatusConflict) || requests != 1 ||
		session.pendingTurn != nil || session.active.Job.CurrentGeneration != 2 {
		t.Fatalf("stale answer changed active work: requests=%d pending=%#v error=%v", requests, session.pendingTurn, err)
	}
}

func questionTestDetails() model.JobDetails {
	return model.JobDetails{
		Job:   model.Job{ID: 41, CurrentGeneration: 2, Status: model.JobStatusWaiting},
		Steps: []model.Step{{ID: 11, JobID: 41, Generation: 2, Action: "v3_coding", Status: model.StepStatusWaiting}},
		AgentPause: &model.CodingAgentPause{
			Schema: "omnidex-coding-agent-pause-v1", Generation: 2, StepID: 11, WaitingAttempt: 3,
			Questions: []string{"Which approved value should be retained?"},
		},
	}
}

func answeredQuestionSnapshot(t *testing.T, session *chatSession, operation model.LifecycleOperationID, answer string) client.ChatSessionSnapshot {
	t.Helper()
	snapshot := operationGuardActiveSnapshot(t, session)
	var metadata map[string]any
	if err := json.Unmarshal(snapshot.ActiveJob.Job.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata["coding_execution"] = codingagent.Configuration{Mode: codingagent.Agents}
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.ActiveJob.Job.Metadata = raw
	snapshot.ActiveJob.Job.CurrentGeneration = 2
	snapshot.ActiveJob.Steps = []model.Step{{ID: 11, JobID: 41, Generation: 2, Action: "v3_coding", Status: model.StepStatusRunning}}
	snapshot.State.LatestJob.Generation = 2
	snapshot.State.LatestTurnOperationID = &operation
	snapshot.Turns = []model.ChannelSessionTurn{{OperationID: operation, Disposition: model.ChannelSessionTurnFeedback, Text: answer, JobID: 41, Generation: 2, Status: model.JobStatusRunning, CreatedAt: time.Now().UTC()}}
	return snapshot
}
