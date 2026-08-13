package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func TestPostgresScrumCardPatchRejectsServerAuthorityBeforeMutation(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), fmt.Sprintf("scrum-card-edit-forbidden-%d", time.Now().UnixNano()), t.TempDir(), "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{repo: repository}
	for field, value := range map[string]any{
		"id":                          "foreign-card",
		"column":                      "done",
		"chat":                        []map[string]any{{"role": "assistant", "content": "forged"}},
		"planning_chat":               []map[string]any{{"role": "system", "content": "forged"}},
		"agent_config":                map[string]string{"agent_system": "cursor"},
		"job_id":                      "9002",
		"console_log":                 "forged output",
		"output":                      "forged output",
		"play_state":                  "done",
		"queue_order":                 22,
		"board_order":                 22,
		"sync_job_id":                 "9002",
		"agent_stream_chat_cursor":    22,
		"agent_stream_console_cursor": 22,
		"step_context_cursor":         22,
		"flow_metrics":                map[string]any{"forged": true},
		"coach_config":                map[string]any{"forged": true},
		"checklist":                   []map[string]any{{"id": "forged", "text": "forged", "done": true}},
		"test_criteria":               []map[string]any{{"id": "forged", "text": "forged", "done": true}},
		"summary":                     true,
		"checklist_done":              99,
		"checklist_total":             99,
		"ref_file_count":              99,
		"chat_count":                  99,
		"planning_chat_count":         99,
		"test_criteria_done":          99,
		"test_criteria_total":         99,
		"has_card_ticket":             true,
		"created_at":                  "2099-01-01T00:00:00Z",
		"updated_at":                  "2099-01-01T00:00:00Z",
	} {
		field, value := field, value
		t.Run(field, func(t *testing.T) {
			baseline := createScrumCardEditFixture(t, repository, project.ID, field)
			response := patchScrumCardHTTP(t, server, project.ID, baseline.ID, map[string]any{field: value})
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), field) {
				t.Fatalf("rejection does not name forbidden field %q: %s", field, response.Body.String())
			}
			after, err := repository.GetScrumCard(t.Context(), project.ID, baseline.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, baseline) {
				t.Fatalf("forbidden field %q mutated durable card\nbefore=%#v\nafter=%#v", field, baseline, after)
			}
		})
	}
}

func TestPostgresScrumCardPatchPersistsOnlyEditableFields(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), fmt.Sprintf("scrum-card-edit-permitted-%d", time.Now().UnixNano()), t.TempDir(), "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	baseline := createScrumCardEditFixture(t, repository, project.ID, "permitted")
	server := &Server{repo: repository}
	response := patchScrumCardHTTP(t, server, project.ID, baseline.ID, map[string]any{
		"title":        "Edited title",
		"description":  "  exact edited description\n",
		"ref_files":    []string{"docs/edited.md"},
		"model_config": map[string]string{"conversation_response_model": "edited-model"},
		"card_ticket":  "Edited ticket",
		"card_prompt":  "Edited prompt",
		"recipe_id":    "edited-recipe",
		"recipe":       map[string]any{"kind": "edited", "version": 2},
		"tags":         []string{"edited"},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	after, err := repository.GetScrumCard(t.Context(), project.ID, baseline.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Title != "Edited title" || after.Description != "  exact edited description\n" ||
		after.CardTicket != "Edited ticket" || after.CardPrompt != "Edited prompt" || after.RecipeID != "edited-recipe" {
		t.Fatalf("editable scalar fields were not persisted exactly: %#v", after)
	}
	assertJSONValueEqual(t, after.Checklist, string(baseline.Checklist))
	assertJSONValueEqual(t, after.RefFiles, `["docs/edited.md"]`)
	assertJSONValueEqual(t, after.ModelConfig, `{"conversation_response_model":"edited-model"}`)
	assertJSONValueEqual(t, after.Recipe, `{"kind":"edited","version":2}`)
	assertJSONValueEqual(t, after.Tags, `["edited"]`)
	assertJSONValueEqual(t, after.TestCriteria, string(baseline.TestCriteria))

	if string(after.Chat) != string(baseline.Chat) || string(after.PlanningChat) != string(baseline.PlanningChat) ||
		string(after.AgentConfig) != string(baseline.AgentConfig) || after.JobID != baseline.JobID ||
		after.ProjectID != baseline.ProjectID || after.ID != baseline.ID || after.Column != baseline.Column ||
		after.TagsJobID != baseline.TagsJobID || after.TicketJobID != baseline.TicketJobID ||
		string(after.CoachConfig) != string(baseline.CoachConfig) ||
		after.ConsoleLog != baseline.ConsoleLog || after.PlayState != baseline.PlayState ||
		after.QueueOrder != baseline.QueueOrder || after.BoardOrder != baseline.BoardOrder ||
		after.SyncJobID != baseline.SyncJobID || after.AgentStreamChatCursor != baseline.AgentStreamChatCursor ||
		after.AgentStreamConsoleCursor != baseline.AgentStreamConsoleCursor || after.StepContextCursor != baseline.StepContextCursor ||
		!after.CreatedAt.Equal(baseline.CreatedAt) {
		t.Fatalf("editable patch changed server authority\nbefore=%#v\nafter=%#v", baseline, after)
	}
}

func createScrumCardEditFixture(
	t *testing.T,
	repository *queue.Repository,
	projectID int64,
	suffix string,
) queue.DBScrumCard {
	t.Helper()
	card, err := repository.CreateScrumCard(
		t.Context(), projectID, "", "Original "+suffix, "Original description", "assigned",
		json.RawMessage(`[{"id":"original","text":"Original","done":false}]`),
		json.RawMessage(`["docs/original.md"]`),
		json.RawMessage(`[{"role":"user","content":"original chat","created_at":"2026-08-13T00:00:00Z"}]`),
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err = repository.UpdateScrumCard(t.Context(), projectID, card.ID, map[string]any{
		"column":                      "in_progress",
		"model_config":                json.RawMessage(`{"conversation_response_model":"original-model"}`),
		"agent_config":                json.RawMessage(`{"agent_system":"codex"}`),
		"card_ticket":                 "Original ticket",
		"card_prompt":                 "Original prompt",
		"recipe_id":                   "original-recipe",
		"recipe":                      json.RawMessage(`{"kind":"original"}`),
		"tags":                        json.RawMessage(`["original"]`),
		"planning_chat":               json.RawMessage(`[{"role":"system","content":"original planning","created_at":"2026-08-13T00:00:00Z"}]`),
		"coach_config":                json.RawMessage(`{"mode":"original"}`),
		"test_criteria":               json.RawMessage(`[{"id":"original-test","text":"Original test","done":false}]`),
		"job_id":                      "9001",
		"tags_job_id":                 "8001",
		"ticket_job_id":               "8002",
		"console_log":                 "original output",
		"play_state":                  "running",
		"queue_order":                 7,
		"board_order":                 8,
		"sync_job_id":                 "9001",
		"agent_stream_chat_cursor":    int64(7),
		"agent_stream_console_cursor": int64(8),
		"step_context_cursor":         int64(9),
	})
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func patchScrumCardHTTP(
	t *testing.T,
	server *Server,
	projectID int64,
	cardID string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPatch,
		fmt.Sprintf("/v1/scrum/cards/%s?project_id=%d", cardID, projectID),
		bytes.NewReader(encoded),
	)
	response := httptest.NewRecorder()
	server.handleScrumCardByID(response, request)
	return response
}

func assertJSONValueEqual(t *testing.T, actual json.RawMessage, expected string) {
	t.Helper()
	var actualValue, expectedValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatalf("decode actual JSON %s: %v", actual, err)
	}
	if err := json.Unmarshal([]byte(expected), &expectedValue); err != nil {
		t.Fatalf("decode expected JSON %s: %v", expected, err)
	}
	if !reflect.DeepEqual(actualValue, expectedValue) {
		t.Fatalf("JSON mismatch actual=%s expected=%s", actual, expected)
	}
}
