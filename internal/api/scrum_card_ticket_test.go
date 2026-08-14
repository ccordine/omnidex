package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDecodeScrumCardTicketActionRequestAcceptsTypedActions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		body        string
		action      scrumCardTicketAction
		elaboration string
	}{
		{name: "assemble", body: `{"action":"assemble","expected_updated_at":"2026-08-13T12:00:00.123456Z"}`, action: scrumCardTicketAssemble},
		{name: "apply exact elaboration", body: `{"action":"apply_elaboration","expected_updated_at":"2026-08-13T12:00:00Z","elaboration":"  Use the existing boundary.\nKeep this tab:\t "}`, action: scrumCardTicketApplyElaboration, elaboration: "  Use the existing boundary.\nKeep this tab:\t "},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest("POST", "/card-ticket", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			decoded, err := decodeScrumCardTicketActionRequest(response, request)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Action != test.action || decoded.Elaboration != test.elaboration {
				t.Fatalf("decoded=%#v", decoded)
			}
		})
	}
}

func TestDecodeScrumCardTicketActionRequestRejectsInexactAuthority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{name: "missing action", body: []byte(`{"expected_updated_at":"2026-08-13T12:00:00Z"}`), want: "action is required"},
		{name: "unknown action", body: []byte(`{"action":"write","expected_updated_at":"2026-08-13T12:00:00Z"}`), want: `action "write" is not registered`},
		{name: "missing revision", body: []byte(`{"action":"assemble"}`), want: "expected_updated_at is required"},
		{name: "noncanonical revision", body: []byte(`{"action":"assemble","expected_updated_at":"2026-08-13T08:00:00-04:00"}`), want: "canonical UTC"},
		{name: "assemble elaboration", body: []byte(`{"action":"assemble","expected_updated_at":"2026-08-13T12:00:00Z","elaboration":"extra"}`), want: "must not include elaboration"},
		{name: "blank elaboration", body: []byte(`{"action":"apply_elaboration","expected_updated_at":"2026-08-13T12:00:00Z","elaboration":"  "}`), want: "requires elaboration"},
		{name: "null elaboration", body: []byte(`{"action":"apply_elaboration","expected_updated_at":"2026-08-13T12:00:00Z","elaboration":null}`), want: "must not be null"},
		{name: "legacy prompt", body: []byte(`{"prompt":"build a ticket"}`), want: `unknown field "prompt"`},
		{name: "legacy iterate", body: []byte(`{"iterate":true}`), want: `unknown field "iterate"`},
		{name: "legacy generate", body: []byte(`{"action":"generate","expected_updated_at":"2026-08-13T12:00:00Z"}`), want: "not registered"},
		{name: "legacy elaborate", body: []byte(`{"action":"elaborate","expected_updated_at":"2026-08-13T12:00:00Z","elaboration":"text"}`), want: "not registered"},
		{name: "inexact action whitespace", body: []byte(`{"action":" assemble","expected_updated_at":"2026-08-13T12:00:00Z"}`), want: "not registered"},
		{name: "duplicate", body: []byte(`{"action":"assemble","action":"apply_elaboration","expected_updated_at":"2026-08-13T12:00:00Z"}`), want: "duplicate key"},
		{name: "trailing JSON", body: []byte(`{"action":"assemble","expected_updated_at":"2026-08-13T12:00:00Z"} {}`), want: "trailing"},
		{name: "NUL", body: []byte(`{"action":"apply_elaboration","expected_updated_at":"2026-08-13T12:00:00Z","elaboration":"bad\u0000text"}`), want: "NUL"},
		{name: "invalid UTF-8", body: []byte{'{', '"', 'a', 'c', 't', 'i', 'o', 'n', '"', ':', '"', 0xff, '"', '}'}, want: "valid UTF-8"},
		{name: "too large", body: bytes.Repeat([]byte(" "), int(maxScrumCardTicketActionBodyBytes+1)), want: "transport bound"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest("POST", "/card-ticket", bytes.NewReader(test.body))
			response := httptest.NewRecorder()
			_, err := decodeScrumCardTicketActionRequest(response, request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want substring %q", err, test.want)
			}
		})
	}
}

func TestAssembleScrumCardTicketUsesOnlyAuthoritativeCardFields(t *testing.T) {
	t.Parallel()
	card := ScrumCard{
		Title: "Bounded mutation", Description: "Keep repository truth exact.",
		Checklist:    []ScrumChecklistItem{{ID: "c1", Text: "Stage the change", Done: true}},
		TestCriteria: []ScrumChecklistItem{{ID: "t1", Text: "Broad tests pass", Done: false}},
		RefFiles:     []string{"internal/z.go", "docs/a.md"}, Tags: []string{"server", "authority"},
		CardPrompt: "Preserve the current API.",
	}
	ticket, err := assembleScrumCardTicket(card)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Bounded mutation\n\n## Objective\n\nKeep repository truth exact.\n\n## Checklist\n\n- [x] Stage the change\n\n## Test criteria\n\n- [ ] Broad tests pass\n\n## Reference files\n\n- `docs/a.md`\n- `internal/z.go`\n\n## Tags\n\n- authority\n- server\n\n## Elaboration\n\nPreserve the current API.\n"
	if ticket != want {
		t.Fatalf("ticket mismatch\nactual:\n%s\nwant:\n%s", ticket, want)
	}
}

func TestAssembleScrumCardTicketPreservesExactUserElaborationBytes(t *testing.T) {
	t.Parallel()
	exact := "  Preserve leading spaces.\nKeep trailing tab:\t "
	ticket, err := assembleScrumCardTicket(ScrumCard{Title: "Exact", CardPrompt: exact})
	if err != nil {
		t.Fatal(err)
	}
	want := "# Exact\n\n## Objective\n\n## Elaboration\n\n" + exact + "\n"
	if ticket != want {
		t.Fatalf("ticket=%q want=%q", ticket, want)
	}
}

func TestScrumCardTicketActionSourceHasNoInferenceOrLegacyRequestShape(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("scrum_card_ticket.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`json:"prompt"`, `json:"card_prompt"`, `json:"ticket"`, `json:"iterate"`,
		`json:"iterate_notes"`, "LLM", "Model", "JobID", "Enqueue", "writeRemovedInferenceAction",
		`= "generate"`, `= "elaborate"`, "strings.TrimSpace(body.Elaboration.Value)",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("ticket action source retains forbidden authority %q", forbidden)
		}
	}
	browserSource, err := os.ReadFile("web/src/lib/scrum_ticket_api.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`action: "generate"`, `action: "elaborate"`, "elaboration.trim(),"} {
		if strings.Contains(string(browserSource), forbidden) {
			t.Errorf("ticket browser transport retains forbidden authority %q", forbidden)
		}
	}
}

func TestLegacyScrumCardTicketInferenceShapeFailsBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()
	server := &Server{repo: &queue.Repository{}}
	for _, body := range []string{
		`{}`,
		`{"prompt":"create a ticket"}`,
		`{"ticket":"caller-selected whole ticket"}`,
		`{"iterate":true,"iterate_notes":"rewrite it"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/card-ticket?project_id=1", strings.NewReader(body))
		response := httptest.NewRecorder()
		server.handleScrumCardTicket(response, request, "card_1")
		if response.Code != http.StatusBadRequest {
			t.Errorf("body=%s status=%d response=%s", body, response.Code, response.Body.String())
		}
	}
}

func TestScrumCardTicketRejectsInexactProjectQueryBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()
	server := &Server{repo: &queue.Repository{}}
	body := `{"action":"assemble","expected_updated_at":"2026-08-13T12:00:00Z"}`
	for _, target := range []string{
		"/card-ticket?project_id=01",
		"/card-ticket?project_id=1&project_id=2",
		"/card-ticket?project_id=1&agent=cursor",
	} {
		request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
		response := httptest.NewRecorder()
		server.handleScrumCardTicket(response, request, "card_1")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("target=%q status=%d body=%s", target, response.Code, response.Body.String())
		}
	}
}
