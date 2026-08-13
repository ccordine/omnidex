package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func TestPostgresScrumMoveAndDoneReturnDurableServerState(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), fmt.Sprintf("scrum-move-http-%d", time.Now().UnixNano()), t.TempDir(), "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(t.Context(), project.ID, "", "Move me", "", "backlog", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{repo: repository, lifecycleContext: context.Background()}

	moved := postScrumCardStateAction(t, server, project.ID, card.ID, "move", `{"column":"review"}`)
	if moved.Code != http.StatusOK {
		t.Fatalf("move status=%d body=%s", moved.Code, moved.Body.String())
	}
	movedCard := decodeScrumCardTicketResponse(t, moved)
	stored, err := repository.GetScrumCard(t.Context(), project.ID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if movedCard.Column != "review" || stored.Column != "review" ||
		movedCard.UpdatedAt != stored.UpdatedAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("move response=%#v durable=%#v", movedCard, stored)
	}

	done := postScrumCardStateAction(t, server, project.ID, card.ID, "done", `{}`)
	if done.Code != http.StatusOK {
		t.Fatalf("done status=%d body=%s", done.Code, done.Body.String())
	}
	doneCard := decodeScrumCardTicketResponse(t, done)
	stored, err = repository.GetScrumCard(t.Context(), project.ID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if doneCard.Column != "done" || stored.Column != "done" ||
		doneCard.UpdatedAt != stored.UpdatedAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("done response=%#v durable=%#v", doneCard, stored)
	}
}

func TestScrumMoveAndDoneRejectUnregisteredRequestAuthority(t *testing.T) {
	t.Parallel()
	server := &Server{repo: &queue.Repository{}}
	for _, test := range []struct {
		action string
		body   string
	}{
		{action: "move", body: `{"column":"done","ai_reason":"choose done"}`},
		{action: "move", body: `{"column":"done"} {}`},
		{action: "done", body: `{"column":"review"}`},
	} {
		response := postScrumCardStateAction(t, server, 1, "card_1", test.action, test.body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("action=%s status=%d body=%s", test.action, response.Code, response.Body.String())
		}
	}
}

func postScrumCardStateAction(
	t *testing.T,
	server *Server,
	projectID int64,
	cardID string,
	action string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/v1/scrum/cards/%s/%s?project_id=%d", cardID, action, projectID),
		strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.handleScrumCardByID(response, request)
	return response
}
