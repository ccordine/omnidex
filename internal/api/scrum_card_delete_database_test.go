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

func TestPostgresScrumCardDeleteReturnsExactCommittedIdentity(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundleThroughPrefix(t, "089")); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(t.Context(), "Delete response", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(t.Context(), project.ID, "delete-response-card", "Delete", "", "backlog", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	revision := card.UpdatedAt.UTC().Format(time.RFC3339Nano)
	request := httptest.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("/v1/scrum/cards/%s?project_id=%d", card.ID, project.ID),
		strings.NewReader(`{"expected_updated_at":"`+revision+`"}`),
	)
	response := httptest.NewRecorder()
	(&Server{repo: repository}).handleScrumCardByID(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 5 || payload["commit_state"] != "committed" ||
		payload["project_id"] != float64(project.ID) || payload["card_id"] != card.ID ||
		payload["expected_updated_at"] != revision || payload["deleted"] != true {
		t.Fatalf("delete response=%v", payload)
	}
	if _, err := repository.GetScrumCard(t.Context(), project.ID, card.ID); err == nil {
		t.Fatal("committed delete response left card present")
	}
}
