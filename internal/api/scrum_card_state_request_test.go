package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestScrumCardMoveRequestRequiresCanonicalTypedAuthority(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"column":" done","before_card_id":"","expected_updated_at":"2026-08-13T12:00:00Z"}`,
		`{"column":"DONE","before_card_id":"","expected_updated_at":"2026-08-13T12:00:00Z"}`,
		`{"column":"in-progress","before_card_id":"","expected_updated_at":"2026-08-13T12:00:00Z"}`,
		`{"column":"done","before_card_id":" target","expected_updated_at":"2026-08-13T12:00:00Z"}`,
		`{"column":"done","before_card_id":"bad\u0000id","expected_updated_at":"2026-08-13T12:00:00Z"}`,
		`{"column":"done","before_card_id":"` + strings.Repeat("x", maxScrumCardPathIDBytes+1) + `","expected_updated_at":"2026-08-13T12:00:00Z"}`,
		`{"column":"done","before_card_id":""}`,
		`{"column":"done","before_card_id":"","expected_updated_at":"2026-08-13T08:00:00-04:00"}`,
	} {
		request := httptest.NewRequest("POST", "/", strings.NewReader(body))
		if _, err := decodeScrumCardMoveRequest(httptest.NewRecorder(), request); err == nil {
			t.Errorf("decodeScrumCardMoveRequest(%s) unexpectedly succeeded", body)
		}
	}

	request := httptest.NewRequest("POST", "/", strings.NewReader(
		`{"column":"in_progress","before_card_id":"target","expected_updated_at":"2026-08-13T12:00:00.123456Z"}`,
	))
	decoded, err := decodeScrumCardMoveRequest(httptest.NewRecorder(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Column != queue.ScrumCardInProgress || decoded.BeforeCardID != "target" ||
		decoded.ExpectedUpdatedAt.Value.UTC().Format("2006-01-02T15:04:05.999999999Z07:00") != "2026-08-13T12:00:00.123456Z" {
		t.Fatalf("decoded request=%+v", decoded)
	}
}

func TestScrumCardDoneRequestRequiresCanonicalServerRevision(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{}`,
		`{"expected_updated_at":null}`,
		`{"expected_updated_at":"2026-08-13T08:00:00-04:00"}`,
		`{"expected_updated_at":"2026-08-13T12:00:00Z","column":"done"}`,
	} {
		request := httptest.NewRequest("POST", "/", strings.NewReader(body))
		if _, err := decodeScrumCardDoneRequest(httptest.NewRecorder(), request); err == nil {
			t.Errorf("decodeScrumCardDoneRequest(%s) unexpectedly succeeded", body)
		}
	}
	request := httptest.NewRequest("POST", "/", strings.NewReader(
		`{"expected_updated_at":"2026-08-13T12:00:00.123456Z"}`,
	))
	if _, err := decodeScrumCardDoneRequest(httptest.NewRecorder(), request); err != nil {
		t.Fatal(err)
	}
}

func TestScrumCardStateHandlersRejectInexactProjectQuery(t *testing.T) {
	t.Parallel()
	server := &Server{repo: &queue.Repository{}}
	for _, test := range []struct {
		target string
		done   bool
	}{
		{target: "/v1/scrum/cards/card-1/move?project_id=01"},
		{target: "/v1/scrum/cards/card-1/move?project_id=1&project_id=2"},
		{target: "/v1/scrum/cards/card-1/done?project_id=1&agent=cursor", done: true},
	} {
		body := `{"column":"ready","expected_updated_at":"2026-08-13T12:00:00Z"}`
		request := httptest.NewRequest(http.MethodPost, test.target, strings.NewReader(body))
		response := httptest.NewRecorder()
		if test.done {
			server.handleScrumCardDone(response, request, "card-1")
		} else {
			server.handleScrumCardMove(response, request, "card-1")
		}
		if response.Code != http.StatusBadRequest {
			t.Fatalf("target=%q status=%d body=%s", test.target, response.Code, response.Body.String())
		}
	}
}
