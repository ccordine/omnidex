package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDecodeScrumCardEditRequestAcceptsOnlyEditableFields(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest("PATCH", "/v1/scrum/cards/card-1", strings.NewReader(`{
		"expected_updated_at":"2026-08-13T12:00:00.123456Z",
		"title":"Changed",
		"description":"Exact description",
		"ref_files":["docs/readme.md"],
		"card_ticket":"Ticket",
		"card_prompt":"Prompt",
		"tags":["typed"]
	}`))
	response := httptest.NewRecorder()

	edit, err := decodeScrumCardEditRequest(response, request)
	if err != nil {
		t.Fatal(err)
	}
	patch := edit.repositoryPatch()
	if patch.Title == nil || patch.Description == nil || patch.RefFiles == nil ||
		patch.CardTicket == nil || patch.CardPrompt == nil || patch.Tags == nil {
		t.Fatalf("typed repository patch lost an editable field: %#v", patch)
	}
}

func TestScrumCardTagAuthorityIsNormalizedOnlyByTheServer(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(
		http.MethodPatch,
		"/v1/scrum/cards/card-1?project_id=1",
		strings.NewReader(`{"expected_updated_at":"2026-08-13T12:00:00.123456Z","description":"  exact bytes\n","tags":[" API Client ","Bug   Fix","api-client"," \t "]}`),
	)
	response := httptest.NewRecorder()
	edit, err := decodeScrumCardEditRequest(response, request)
	if err != nil {
		t.Fatal(err)
	}
	patch := edit.repositoryPatch()
	if patch.Description == nil || *patch.Description != "  exact bytes\n" {
		t.Fatalf("non-tag bytes changed: %#v", patch.Description)
	}
	if patch.Tags == nil {
		t.Fatal("server-owned tag projection is missing")
	}
	want := []string{"api-client", "bug-fix"}
	if !reflect.DeepEqual(*patch.Tags, want) {
		t.Fatalf("tags=%q want=%q", *patch.Tags, want)
	}
}

func TestDecodeScrumCardEditRequestRejectsServerOwnedFields(t *testing.T) {
	t.Parallel()
	for field, value := range map[string]string{
		"id":                          `"foreign"`,
		"column":                      `"done"`,
		"chat":                        `[]`,
		"planning_chat":               `[]`,
		"agent_config":                `{}`,
		"model_config":                `{"conversation_response_model":"retired"}`,
		"recipe_id":                   `"retired"`,
		"recipe":                      `{}`,
		"job_id":                      `"99"`,
		"tags_job_id":                 `"99"`,
		"ticket_job_id":               `"99"`,
		"console_log":                 `"fake output"`,
		"output":                      `"fake output"`,
		"play_state":                  `"complete"`,
		"queue_order":                 `1`,
		"board_order":                 `1`,
		"sync_job_id":                 `"99"`,
		"agent_stream_chat_cursor":    `9`,
		"agent_stream_console_cursor": `9`,
		"step_context_cursor":         `9`,
		"flow_metrics":                `{}`,
		"coach_config":                `{}`,
		"checklist":                   `[]`,
		"test_criteria":               `[]`,
		"summary":                     `true`,
		"checklist_done":              `1`,
		"checklist_total":             `1`,
		"ref_file_count":              `1`,
		"chat_count":                  `1`,
		"planning_chat_count":         `1`,
		"test_criteria_done":          `1`,
		"test_criteria_total":         `1`,
		"has_card_ticket":             `true`,
		"created_at":                  `"future"`,
		"updated_at":                  `"future"`,
	} {
		field, value := field, value
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest("PATCH", "/v1/scrum/cards/card-1", strings.NewReader(`{"expected_updated_at":"2026-08-13T12:00:00Z","`+field+`":`+value+`}`))
			response := httptest.NewRecorder()
			_, err := decodeScrumCardEditRequest(response, request)
			if err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("field %q error=%v; want explicit rejection", field, err)
			}
		})
	}
}

func TestDecodeScrumCardEditRequestEnforcesBoundAndExactEOF(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{name: "missing revision", body: []byte(`{"title":"one"}`), want: "expected_updated_at is required"},
		{name: "empty object", body: []byte(`{}`), want: "at least one editable field"},
		{name: "null", body: []byte(`null`), want: "must be one JSON object"},
		{name: "trailing JSON", body: []byte(`{"expected_updated_at":"2026-08-13T12:00:00Z","title":"one"} {"title":"two"}`), want: "trailing JSON"},
		{name: "duplicate field", body: []byte(`{"expected_updated_at":"2026-08-13T12:00:00Z","title":"one","title":"two"}`), want: "duplicate key"},
		{name: "inexact field alias", body: []byte(`{"Title":"one"}`), want: `inexact or unknown field "Title"`},
		{name: "client checklist authority", body: []byte(`{"checklist":[]}`), want: `inexact or unknown field "checklist"`},
		{name: "null reference file", body: []byte(`{"ref_files":[null]}`), want: "reference file 0 must not be null"},
		{name: "null tag", body: []byte(`{"tags":[null]}`), want: "tag 0 must not be null"},
		{name: "client test criteria authority", body: []byte(`{"test_criteria":[]}`), want: `inexact or unknown field "test_criteria"`},
		{name: "retired card model route", body: []byte(`{"model_config":{"conversation_response_model":"retired"}}`), want: `inexact or unknown field "model_config"`},
		{name: "nul text", body: []byte(`{"description":"\u0000"}`), want: "NUL"},
		{name: "invalid UTF-8", body: []byte{'{', '"', 't', 'i', 't', 'l', 'e', '"', ':', '"', 0xff, '"', '}'}, want: "valid UTF-8"},
		{name: "too large", body: bytes.Repeat([]byte(" "), int(maxScrumCardEditBodyBytes+1)), want: "transport bound"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest("PATCH", "/v1/scrum/cards/card-1", bytes.NewReader(test.body))
			response := httptest.NewRecorder()
			_, err := decodeScrumCardEditRequest(response, request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want substring %q", err, test.want)
			}
		})
	}
}

func TestScrumCardPatchRejectsForbiddenAuthorityBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()
	server := &Server{repo: &queue.Repository{}}
	for name, body := range map[string]string{
		"chat authority":    `{"chat":[]}`,
		"agent authority":   `{"agent_config":{"agent_system":"codex"}}`,
		"output authority":  `{"output":"forged"}`,
		"cursor authority":  `{"step_context_cursor":1}`,
		"trailing document": `{"title":"valid"} {}`,
	} {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPatch, "/v1/scrum/cards/card-1?project_id=1", strings.NewReader(body))
			response := httptest.NewRecorder()
			server.handleScrumCardByID(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}

	body := `{"description":"` + strings.Repeat("x", int(maxScrumCardEditBodyBytes)) + `"}`
	request := httptest.NewRequest(http.MethodPatch, "/v1/scrum/cards/card-1?project_id=1", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.handleScrumCardByID(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestScrumCardPatchSourceHasNoFullCardOrRawMapAuthority(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"scrum_handlers.go", "scrum_service.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"scrumUpdateCard(",
			"var patch ScrumCard",
			"raw := map[string]json.RawMessage",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s retains generic PATCH authority %q", path, forbidden)
			}
		}
	}
}

func TestInternalScrumCardEditableCallersUseTypedFields(t *testing.T) {
	t.Parallel()
	for path, required := range map[string][]string{
		"scrum_card_ticket.go": {"UpdateScrumCardTicket(", "queue.ScrumCardTicketMutation{"},
		"scrum_handlers.go":    {"decodeScrumCardEditRequest(", "scrumEditCard("},
		"scrum_card_edit.go":   {"RefFiles          scrumCardEditableField[[]string]", "expected_updated_at"},
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range required {
			if !strings.Contains(string(source), fragment) {
				t.Errorf("%s does not use typed editable card field %q", path, fragment)
			}
		}
	}
}
