package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPostgresScrumChannelHTTPReplaySurvivesCardDeletion(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), fmt.Sprintf("scrum-http-replay-%d", time.Now().UnixNano()), t.TempDir(), "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(
		t.Context(), project.ID, "", "Replay card", "", "assigned", nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	operationID, err := queue.NewLifecycleOperationID("scrum-http-replay", strconv.FormatInt(project.ID, 10))
	if err != nil {
		t.Fatal(err)
	}
	request := queue.ScrumChannelOperationRequest{
		OperationID: operationID, ProjectID: project.ID, CardID: card.ID, Message: "Start exactly once.",
	}
	command := queue.ScrumChannelOperationCommand{
		Request: request, ExpectedCardUpdatedAt: card.UpdatedAt,
		Effect: queue.ScrumChannelEffect{
			Kind: queue.ScrumChannelStartJob, Instruction: request.Message,
			Pipeline: model.PipelineScrum,
			Metadata: json.RawMessage(fmt.Sprintf(`{"project_id":%d}`, project.ID)),
		},
		ResultAction: "started", ResultAgent: "omnidex",
	}
	first, err := repository.ExecuteScrumChannelOperation(
		t.Context(), command, scrumChannelHTTPTestBuilder(t, request),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupID, cleanupErr := queue.NewLifecycleOperationID("scrum-http-cleanup", strconv.FormatInt(first.Job.ID, 10))
		if cleanupErr != nil {
			t.Errorf("build cleanup lifecycle identity: %v", cleanupErr)
			return
		}
		if _, err := repository.CancelJob(ctx, queue.CancelJobCommand{
			OperationID: cleanupID, JobID: first.Job.ID, Reason: "Scrum HTTP replay test cleanup",
		}); err != nil {
			t.Errorf("cancel Scrum HTTP replay test job: %v", err)
		}
	})
	if err := repository.DeleteScrumCard(t.Context(), project.ID, card.ID); err != nil {
		t.Fatal(err)
	}

	server := &Server{repo: repository}
	exact := scrumChannelHTTPCall(t, server, project.ID, card.ID, operationID, request.Message)
	if exact.Code != http.StatusOK {
		t.Fatalf("exact replay status=%d body=%s", exact.Code, exact.Body.String())
	}
	var payload struct {
		Card   ScrumCard `json:"card"`
		Action string    `json:"action"`
		Agent  string    `json:"agent"`
	}
	if err := json.Unmarshal(exact.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Card.ID != first.Card.ID || payload.Card.JobID != first.Card.JobID ||
		payload.Action != first.Action || payload.Agent != first.Agent {
		t.Fatalf("HTTP replay payload=%+v stored=%+v", payload, first)
	}
	repeated := scrumChannelHTTPCall(t, server, project.ID, card.ID, operationID, request.Message)
	if repeated.Code != http.StatusOK || repeated.Body.String() != exact.Body.String() {
		t.Fatalf("repeated replay status=%d body=%s want=%s", repeated.Code, repeated.Body.String(), exact.Body.String())
	}
	changed := scrumChannelHTTPCall(t, server, project.ID, card.ID, operationID, "Changed content.")
	if changed.Code != http.StatusConflict {
		t.Fatalf("changed replay status=%d body=%s", changed.Code, changed.Body.String())
	}
	var operations, jobs int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT COUNT(*) FROM scrum_channel_operations WHERE operation_id=$1),
			(SELECT COUNT(*) FROM jobs WHERE project_id=$2)
	`, operationID, project.ID).Scan(&operations, &jobs); err != nil {
		t.Fatal(err)
	}
	if operations != 1 || jobs != 1 {
		t.Fatalf("operations=%d jobs=%d", operations, jobs)
	}
}

func scrumChannelHTTPTestBuilder(
	t *testing.T,
	request queue.ScrumChannelOperationRequest,
) queue.ScrumChannelCardBuilder {
	t.Helper()
	return func(_ queue.DBScrumCard, job model.Job) (queue.ScrumChannelCardUpdate, error) {
		chat, err := json.Marshal([]ScrumChatMessage{{
			ID: "http-replay-message", Role: "user", Content: request.Message,
			CreatedAt: "2026-08-09T00:00:00Z", OperationID: string(request.OperationID),
		}})
		if err != nil {
			return queue.ScrumChannelCardUpdate{}, err
		}
		return queue.ScrumChannelCardUpdate{
			Chat: chat, Column: "in_progress", JobID: strconv.FormatInt(job.ID, 10),
			PlayState: "running", QueueOrder: 0, SyncJobID: strconv.FormatInt(job.ID, 10),
		}, nil
	}
}

func scrumChannelHTTPCall(
	t *testing.T,
	server *Server,
	projectID int64,
	cardID string,
	operationID queue.LifecycleOperationID,
	message string,
) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{"operation_id": operationID, "message": message})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/v1/scrum/cards/%s/chat?project_id=%d", cardID, projectID),
		bytes.NewReader(body),
	)
	response := httptest.NewRecorder()
	server.handleScrumCardChat(response, request, cardID)
	return response
}
