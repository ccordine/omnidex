package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCLIQuestionAnswerSendsExactWaitingAuthority(t *testing.T) {
	t.Parallel()
	channel := testCLIChannel("/workspace/answer")
	operationID := testOperationID(t, "question-answer")
	pause := &model.CodingAgentPause{
		Schema: "omnidex-coding-agent-pause-v1", Generation: 2, StepID: 11, WaitingAttempt: 3,
		Questions: []string{"Which approved value should be retained?"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requireRequestAuthority(t, request, http.MethodPost, "/v1/jobs/41/feedback", "")
		requireJSONBody(t, request, map[string]any{
			"operation_id": string(operationID), "feedback": "Keep the value.\nPreserve spacing.",
			"workspace_root": channel.WorkspaceRoot, "workspace_identity": testWorkspaceIdentity,
			"expected_generation": float64(2), "expected_waiting_attempt": float64(3),
		})
		writeJSON(t, writer, http.StatusOK, lifecycleControlReceipt{
			JobID: 41, OperationID: operationID, Status: model.JobStatusRunning,
		})
	}))
	defer server.Close()
	job, err := testClient(t, server.URL).AnswerAgentQuestion(context.Background(), channel,
		testWorkspaceIdentity, 41, operationID, "Keep the value.\nPreserve spacing.", pause)
	if err != nil || job.ID != 41 || job.Status != model.JobStatusRunning {
		t.Fatalf("answer receipt = %#v, error %v", job, err)
	}
}

func TestCLIQuestionAnswerRejectsMissingAuthorityBeforeSending(t *testing.T) {
	t.Parallel()
	channel := testCLIChannel("/workspace/answer")
	operationID := testOperationID(t, "invalid-question-answer")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Error("invalid question was sent to the server")
	}))
	defer server.Close()
	for _, pause := range []*model.CodingAgentPause{
		nil,
		{Schema: "omnidex-coding-agent-pause-v1", Generation: 2, StepID: 11, Questions: []string{"Choose a value."}},
		{Schema: "omnidex-coding-agent-pause-v1", Generation: 2, StepID: 11, WaitingAttempt: 3, Questions: []string{" "}},
	} {
		if _, err := testClient(t, server.URL).AnswerAgentQuestion(context.Background(), channel,
			testWorkspaceIdentity, 41, operationID, "Keep the value.", pause); err == nil {
			t.Fatalf("invalid question accepted: %#v", pause)
		}
	}
}
