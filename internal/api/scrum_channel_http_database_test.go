package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPostgresScrumChannelHTTPReplayRejectsDeletedLiveCardAndPreservesReceipt(t *testing.T) {
	pool := openIsolatedAPIDatabasePool(t)
	repository := queue.New(pool)
	if err := repository.ResetDatabase(t.Context(), loadAPITestDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), fmt.Sprintf("scrum-http-replay-%d", time.Now().UnixNano()), t.TempDir(), "")

	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(
		t.Context(), project.ID, "", "Replay card", "", "assigned", nil, nil,
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
		},
		ResultAction: "started",
	}
	first, err := repository.ExecuteScrumChannelOperation(
		t.Context(), command, scrumChannelHTTPTestBuilder(t, request),
	)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := repository.PauseScrumCardPlayAtRevision(
		t.Context(), project.ID, card.ID, first.Card.UpdatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteScrumCardAtRevision(t.Context(), project.ID, card.ID, paused.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateScrumCard(
		t.Context(), project.ID, card.ID, "Reused identity", "", "assigned", nil, nil,
	); err == nil || !strings.Contains(err.Error(), "immutable operation receipt and cannot be reused") {
		t.Fatalf("operated card identity reuse error=%v", err)
	}

	server := &Server{repo: repository}
	exact := scrumChannelHTTPCall(t, server, project.ID, card.ID, operationID, request.Message)
	if exact.Code != http.StatusNotFound {
		t.Fatalf("deleted live replay status=%d body=%s", exact.Code, exact.Body.String())
	}
	repeated := scrumChannelHTTPCall(t, server, project.ID, card.ID, operationID, request.Message)
	if repeated.Code != http.StatusNotFound || repeated.Body.String() != exact.Body.String() {
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
	return func(locked queue.DBScrumCard, job model.Job) (queue.ScrumChannelCardUpdate, error) {
		return buildScrumChannelCardUpdate(locked, request, "started", job)
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
