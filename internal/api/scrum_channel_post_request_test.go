package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestScrumChannelPostDecoderPreservesExactNonblankMessage(t *testing.T) {
	operationID := scrumChannelPostOperationID(t)
	message := "  preserve this exact message\n"
	body := fmt.Sprintf(`{"operation_id":%q,"message":%q}`, operationID, message)
	request := httptest.NewRequest(http.MethodPost, "/v1/scrum/cards/card_1/chat", strings.NewReader(body))

	decoded, err := decodeScrumChannelPostRequest(httptest.NewRecorder(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.OperationID != operationID || decoded.Message != message {
		t.Fatalf("decoded=%+v want operation=%q exact message=%q", decoded, operationID, message)
	}
}

func TestScrumChannelPostRejectsUnregisteredTransportAuthority(t *testing.T) {
	operationID := scrumChannelPostOperationID(t)
	validPrefix := fmt.Sprintf(`{"operation_id":%q,"message":"continue"`, operationID)
	tests := []struct {
		name   string
		body   []byte
		status int
	}{
		{name: "oversized", body: []byte(`{"operation_id":"` + string(operationID) + `","message":"` + strings.Repeat("x", int(maxScrumChannelPostBodyBytes)) + `"}`), status: http.StatusRequestEntityTooLarge},
		{name: "unknown op", body: []byte(validPrefix + `,"op":"create_file"}`), status: http.StatusBadRequest},
		{name: "unknown path", body: []byte(validPrefix + `,"path":"/tmp/target"}`), status: http.StatusBadRequest},
		{name: "unknown tool", body: []byte(validPrefix + `,"tool":"shell"}`), status: http.StatusBadRequest},
		{name: "unknown model", body: []byte(validPrefix + `,"model":"planner"}`), status: http.StatusBadRequest},
		{name: "duplicate", body: []byte(fmt.Sprintf(`{"operation_id":%q,"message":"first","message":"second"}`, operationID)), status: http.StatusBadRequest},
		{name: "case alias", body: []byte(fmt.Sprintf(`{"operation_id":%q,"Message":"continue"}`, operationID)), status: http.StatusBadRequest},
		{name: "trailing JSON", body: []byte(validPrefix + `} {}`), status: http.StatusBadRequest},
		{name: "invalid UTF-8", body: append([]byte(validPrefix+`,"extra":"`), 0xff, '"', '}'), status: http.StatusBadRequest},
		{name: "NUL", body: []byte(fmt.Sprintf(`{"operation_id":%q,"message":"bad\u0000message"}`, operationID)), status: http.StatusBadRequest},
		{name: "empty", body: []byte(fmt.Sprintf(`{"operation_id":%q,"message":""}`, operationID)), status: http.StatusBadRequest},
		{name: "blank", body: []byte(fmt.Sprintf(`{"operation_id":%q,"message":"  \n "}`, operationID)), status: http.StatusBadRequest},
		{name: "turn bound", body: []byte(fmt.Sprintf(`{"operation_id":%q,"message":%q}`, operationID, strings.Repeat("x", model.MaxFreeFormTurnBytes+1))), status: http.StatusBadRequest},
		{name: "invalid operation", body: []byte(`{"operation_id":"operation-from-model","message":"continue"}`), status: http.StatusBadRequest},
	}
	server := &Server{repo: &queue.Repository{}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodPost, "/v1/scrum/cards/card_1/chat?project_id=1", bytes.NewReader(test.body),
			)
			response := httptest.NewRecorder()
			server.handleScrumCardChat(response, request, "card_1")
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
		})
	}
}

func TestScrumCardActionRejectsExtraPathSegments(t *testing.T) {
	server := &Server{repo: &queue.Repository{}}
	for _, path := range []string{
		"/v1/scrum/cards/card_1/chat/arbitrary?project_id=1",
		"/v1/scrum/cards/card_1/chat/?project_id=1",
		"/v1/scrum/cards/%20card_1/chat?project_id=1",
		"/v1/scrum/cards/card_1/%20chat?project_id=1",
		"/v1/scrum/cards//chat?project_id=1",
		"/v1/scrum/cards/" + strings.Repeat("c", maxScrumCardPathIDBytes+1) + "/chat?project_id=1",
		"/v1/scrum/cards/card_1/" + strings.Repeat("a", maxScrumCardPathActionBytes+1) + "?project_id=1",
	} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		response := httptest.NewRecorder()
		server.handleScrumCardByID(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("inexact card-action path %q status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestParseJobIDRequiresOneCanonicalPositiveInteger(t *testing.T) {
	t.Parallel()
	if id, err := parseJobID("14"); err != nil || id != 14 {
		t.Fatalf("id=%d error=%v", id, err)
	}
	for _, raw := range []string{"", "0", "-1", "+14", "014", " 14", "14 ", "14 trailing"} {
		if _, err := parseJobID(raw); err == nil {
			t.Fatalf("job ID %q unexpectedly accepted", raw)
		}
	}
}

func scrumChannelPostOperationID(t *testing.T) queue.LifecycleOperationID {
	t.Helper()
	id, err := queue.NewLifecycleOperationID("scrum-channel-post-decoder")
	if err != nil {
		t.Fatal(err)
	}
	return id
}
