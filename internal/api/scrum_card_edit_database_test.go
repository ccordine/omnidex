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
		t.Context(), fmt.Sprintf("scrum-card-edit-forbidden-%d", time.Now().UnixNano()), t.TempDir(), "")

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
		"model_config":                map[string]string{"conversation_response_model": "retired"},
		"recipe_id":                   "retired-recipe",
		"recipe":                      map[string]any{"retired": true},
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
			response := patchScrumCardHTTP(t, server, project.ID, baseline.ID, map[string]any{
				"expected_updated_at": baseline.UpdatedAt.UTC().Format(time.RFC3339Nano), field: value,
			})
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
		t.Context(), fmt.Sprintf("scrum-card-edit-permitted-%d", time.Now().UnixNano()), t.TempDir(), "")

	if err != nil {
		t.Fatal(err)
	}
	baseline := createScrumCardEditFixture(t, repository, project.ID, "permitted")
	server := &Server{repo: repository}
	response := patchScrumCardHTTP(t, server, project.ID, baseline.ID, map[string]any{
		"expected_updated_at": baseline.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"title":               "Edited title",
		"description":         "  exact edited description\n",
		"ref_files":           []string{"docs/edited.md"},
		"card_ticket":         "Edited ticket",
		"card_prompt":         "Edited prompt",
		"tags":                []string{"edited"},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	after, err := repository.GetScrumCard(t.Context(), project.ID, baseline.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Title != "Edited title" || after.Description != "  exact edited description\n" ||
		after.CardTicket != "Edited ticket" || after.CardPrompt != "Edited prompt" {
		t.Fatalf("editable scalar fields were not persisted exactly: %#v", after)
	}
	assertJSONValueEqual(t, after.Checklist, string(baseline.Checklist))
	assertJSONValueEqual(t, after.RefFiles, `["docs/edited.md"]`)
	assertJSONValueEqual(t, after.Tags, `["edited"]`)
	assertJSONValueEqual(t, after.TestCriteria, string(baseline.TestCriteria))
	assertFlowMetricsOnlyAdvanceRevision(t, baseline.FlowMetrics, after.FlowMetrics, after.UpdatedAt)
	if !after.UpdatedAt.After(baseline.UpdatedAt) {
		t.Fatalf("editable patch did not advance the card revision: before=%s after=%s", baseline.UpdatedAt, after.UpdatedAt)
	}

	if after.JobID != baseline.JobID || after.ProjectID != baseline.ProjectID ||
		after.ID != baseline.ID || after.Column != baseline.Column ||
		after.PlayState != baseline.PlayState ||
		after.QueueOrder != baseline.QueueOrder || after.BoardOrder != baseline.BoardOrder ||
		after.SyncJobID != baseline.SyncJobID || after.StepContextCursor != baseline.StepContextCursor ||
		after.ChannelMessageCount != baseline.ChannelMessageCount ||
		after.ChannelContentBytes != baseline.ChannelContentBytes ||
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
	)
	if err != nil {
		t.Fatal(err)
	}
	ticket, prompt := "Original ticket", "Original prompt"
	tags := []string{"original"}
	card, err = repository.UpdateScrumCardAtRevision(
		t.Context(), projectID, card.ID, card.UpdatedAt,
		queue.ScrumCardRevisionPatch{CardTicket: &ticket, CardPrompt: &prompt, Tags: &tags},
	)
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

func assertFlowMetricsOnlyAdvanceRevision(
	t *testing.T,
	beforeRaw json.RawMessage,
	afterRaw json.RawMessage,
	cardUpdatedAt time.Time,
) {
	t.Helper()
	before, err := parseScrumFlowMetrics(beforeRaw)
	if err != nil {
		t.Fatalf("decode flow metrics before edit: %v", err)
	}
	after, err := parseScrumFlowMetrics(afterRaw)
	if err != nil {
		t.Fatalf("decode flow metrics after edit: %v", err)
	}
	observedRevision := after.UpdatedAt
	before.UpdatedAt = ""
	after.UpdatedAt = ""
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("editable patch changed flow metrics beyond the revision\nbefore=%s\nafter=%s", beforeRaw, afterRaw)
	}
	expectedRevision := cardUpdatedAt.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
	if observedRevision != expectedRevision {
		t.Fatalf("flow metrics revision=%q card revision=%q", observedRevision, expectedRevision)
	}
}
