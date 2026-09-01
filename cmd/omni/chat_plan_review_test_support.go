package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func waitingPlanReviewJob(
	t *testing.T,
	session *chatSession,
	jobID int64,
	generation int64,
) *model.JobDetails {
	t.Helper()
	now := time.Date(2026, time.September, 1, 15, 0, 0, 0, time.UTC)
	metadata, err := json.Marshal(map[string]any{
		"channel_id": session.channel.ID, "channel_user_message_id": int64(1),
		"client_cwd":                session.channel.WorkspaceRoot,
		"client_workspace_identity": session.workspaceIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &model.JobDetails{
		Job: model.Job{
			ID: jobID, Instruction: "Let a user confirm the item.", Pipeline: model.PipelineChat,
			Status: model.JobStatusWaiting, Metadata: metadata, CurrentGeneration: generation,
			CreatedAt: now, UpdatedAt: now,
		},
		Steps: []model.Step{
			{
				ID: 11, JobID: jobID, Action: codingPlanReviewStepAction,
				Status: model.StepStatusWaiting, Generation: generation,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: 12, JobID: jobID, Action: "v3_coding",
				Status: model.StepStatusPending, Generation: generation,
				CreatedAt: now, UpdatedAt: now,
			},
		},
	}
}

func chatPlanReviewSnapshot(
	t *testing.T,
	session *chatSession,
	status string,
	generation int64,
	controls []queue.ChannelSessionControl,
) client.ChatSessionSnapshot {
	t.Helper()
	if session == nil || session.active == nil {
		t.Fatal("plan review snapshot requires an active test session")
	}
	details := waitingPlanReviewJob(t, session, session.active.Job.ID, generation)
	details.Job.Status = status
	details.Job.UpdatedAt = details.Job.UpdatedAt.Add(time.Duration(generation) * time.Second)
	if status == model.JobStatusRunning {
		details.Steps[0].Status = model.StepStatusCompleted
		details.Steps[1].Status = model.StepStatusPending
	}
	messageTime := details.Job.CreatedAt
	if controls == nil {
		controls = []queue.ChannelSessionControl{}
	}
	return client.ChatSessionSnapshot{
		RealtimeCursor: uint64(generation),
		Revision:       "channel_session_revision_" + strings.Repeat("b", 64),
		Channel:        session.channel, WorkspaceIdentity: session.workspaceIdentity,
		Messages: []model.ChannelMessage{{
			ID: 1, ChannelID: session.channel.ID, Role: model.ChannelMessageRoleUser,
			Content: details.Job.Instruction, CreatedAt: messageTime,
			Turn: &model.ChannelMessageTurnState{
				JobID: details.Job.ID, Status: status, UpdatedAt: details.Job.UpdatedAt,
			},
		}},
		Turns: []queue.ChannelSessionTurn{}, Controls: controls, ActiveJob: details,
	}
}

func singleLeafPlanReviewFixture(
	t *testing.T,
	jobID int64,
	generation int64,
) model.CodingPlan {
	t.Helper()
	now := time.Date(2026, time.September, 1, 15, 0, 0, 0, time.UTC)
	statement := "The software lets a user confirm the item."
	id, err := model.NewCodingPlanLeafID(statement)
	if err != nil {
		t.Fatal(err)
	}
	return model.CodingPlan{
		JobID: jobID, Generation: generation, Revision: 1,
		State: model.CodingPlanStateReview, ScopeMode: model.CodingScopeModeNormal,
		RequestSHA256: strings.Repeat("a", 64),
		Leaves: []model.CodingPlanLeaf{{
			ID: id, Statement: statement, Annotation: model.CodingPlanAnnotationGrounded,
			Decision: model.CodingPlanDecisionPending,
		}},
		CreatedAt: now, UpdatedAt: now,
	}
}

func decodeChatPlanReviewJSON(t *testing.T, request *http.Request, destination any) {
	t.Helper()
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		t.Fatalf("decode %s request: %v", request.URL.Path, err)
	}
}

func writeChatPlanReviewJSON(t *testing.T, writer http.ResponseWriter, status int, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Errorf("encode test response: %v", err)
	}
}

func armPlanReviewNoteTransition(t *testing.T, console *chatConsole) {
	t.Helper()
	if console == nil || console.reviewInput == nil {
		t.Fatal("test plan-review console is unavailable")
	}
	console.reviewInput.mu.Lock()
	defer console.reviewInput.mu.Unlock()
	if !console.reviewInput.review.Load() {
		t.Fatal("cannot arm a note transition outside active review input")
	}
	console.reviewInput.noteTransition = true
}
