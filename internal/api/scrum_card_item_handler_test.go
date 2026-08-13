package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDecodeScrumCardItemRequestAcceptsOnlyExactTypedActions(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for name, body := range map[string]string{
		"add":    `{"action":"add","expected_updated_at":"` + now + `","text":"Server identity"}`,
		"toggle": `{"action":"toggle","expected_updated_at":"` + now + `","item_id":"item-1","done":true}`,
		"remove": `{"action":"remove","expected_updated_at":"` + now + `","item_id":"item-1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			if _, err := decodeScrumCardItemRequest(httptest.NewRecorder(), request); err != nil {
				t.Fatal(err)
			}
		})
	}
	for name, body := range map[string]string{
		"client id on add":  `{"action":"add","expected_updated_at":"` + now + `","item_id":"client","text":"wrong"}`,
		"whole list":        `{"action":"add","expected_updated_at":"` + now + `","checklist":[]}`,
		"missing revision":  `{"action":"remove","item_id":"item-1"}`,
		"trailing document": `{"action":"remove","expected_updated_at":"` + now + `","item_id":"item-1"}{}`,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			if _, err := decodeScrumCardItemRequest(httptest.NewRecorder(), request); err == nil {
				t.Fatal("expected exact typed request rejection")
			}
		})
	}
}

func TestPostgresScrumChecklistMutationOwnsIdentityAndRevision(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(t.Context(), fmt.Sprintf("scrum-items-%d", time.Now().UnixNano()), t.TempDir(), "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(t.Context(), project.ID, "", "Items", "", "assigned", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{repo: repository}
	add := postScrumItemMutation(t, server, project.ID, card.ID, `{"action":"add","expected_updated_at":"`+card.UpdatedAt.Format(time.RFC3339Nano)+`","text":"First"}`)
	if add.Code != http.StatusOK {
		t.Fatalf("add status=%d body=%s", add.Code, add.Body.String())
	}
	added, err := repository.GetScrumCard(t.Context(), project.ID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	var items []queue.ScrumCardItem
	err = json.Unmarshal(added.Checklist, &items)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !strings.HasPrefix(items[0].ID, "item_") || items[0].Text != "First" {
		t.Fatalf("server-owned checklist=%+v", items)
	}
	stale := postScrumItemMutation(t, server, project.ID, card.ID, `{"action":"remove","expected_updated_at":"`+card.UpdatedAt.Format(time.RFC3339Nano)+`","item_id":"`+items[0].ID+`"}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body.String())
	}
	toggle := postScrumItemMutation(t, server, project.ID, card.ID, `{"action":"toggle","expected_updated_at":"`+added.UpdatedAt.Format(time.RFC3339Nano)+`","item_id":"`+items[0].ID+`","done":true}`)
	if toggle.Code != http.StatusOK {
		t.Fatalf("toggle status=%d body=%s", toggle.Code, toggle.Body.String())
	}
}

func postScrumItemMutation(t *testing.T, server *Server, projectID int64, cardID, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/scrum/cards/%s/checklist?project_id=%d", cardID, projectID), strings.NewReader(body))
	response := httptest.NewRecorder()
	server.handleScrumCardItem(response, request, cardID, queue.ScrumCardChecklist)
	return response
}
