package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestLifecycleFeedbackTransportPreservesExactAuthority(t *testing.T) {
	operationID, err := queue.NewLifecycleOperationID("http-exact-feedback", "41")
	if err != nil {
		t.Fatal(err)
	}
	exact := "  preserve leading authority\nwith a trailing tab\t "
	body := []byte(`{"operation_id":"` + string(operationID) + `","feedback":"  preserve leading authority\nwith a trailing tab\t "}`)
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	response := httptest.NewRecorder()

	decoded, err := decodeLifecycleFeedbackRequest(response, request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.OperationID != operationID || decoded.Feedback != exact {
		t.Fatalf("decoded=%+v want operation=%q feedback=%q", decoded, operationID, exact)
	}
}

func TestLifecycleCancelTransportPreservesExactAuthority(t *testing.T) {
	operationID, err := queue.NewLifecycleOperationID("http-exact-cancel", "41")
	if err != nil {
		t.Fatal(err)
	}
	exact := "  stop after this exact checkpoint\n "
	body := []byte(`{"operation_id":"` + string(operationID) + `","reason":"  stop after this exact checkpoint\n "}`)
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	response := httptest.NewRecorder()

	decoded, err := decodeLifecycleCancelRequest(response, request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.OperationID != operationID || decoded.Reason != exact {
		t.Fatalf("decoded=%+v want operation=%q reason=%q", decoded, operationID, exact)
	}
}

func TestLifecycleControlTransportRejectsInexactOrUnboundedAuthority(t *testing.T) {
	operationID, err := queue.NewLifecycleOperationID("http-invalid-lifecycle", "41")
	if err != nil {
		t.Fatal(err)
	}
	validID := string(operationID)
	invalidUTF8 := append([]byte(`{"operation_id":"`+validID+`","feedback":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	tests := []struct {
		name string
		body []byte
	}{
		{name: "duplicate", body: []byte(`{"operation_id":"` + validID + `","feedback":"first","feedback":"second"}`)},
		{name: "unknown", body: []byte(`{"operation_id":"` + validID + `","feedback":"exact","action":"replan"}`)},
		{name: "inexact field case", body: []byte(`{"operation_id":"` + validID + `","Feedback":"exact"}`)},
		{name: "trailing document", body: []byte(`{"operation_id":"` + validID + `","feedback":"exact"} {}`)},
		{name: "null feedback", body: []byte(`{"operation_id":"` + validID + `","feedback":null}`)},
		{name: "blank feedback", body: []byte(`{"operation_id":"` + validID + `","feedback":" \n\t "}`)},
		{name: "NUL feedback", body: []byte(`{"operation_id":"` + validID + `","feedback":"bad\u0000value"}`)},
		{name: "invalid operation", body: []byte(`{"operation_id":"lifecycle_operation_NOT_CANONICAL","feedback":"exact"}`)},
		{name: "invalid UTF-8", body: invalidUTF8},
		{name: "oversized feedback", body: []byte(`{"operation_id":"` + validID + `","feedback":"` + strings.Repeat("x", maxLifecycleControlTextBytes+1) + `"}`)},
		{name: "oversized transport", body: []byte(`{"operation_id":"` + validID + `","feedback":"` + strings.Repeat("x", int(maxLifecycleControlBodyBytes)) + `"}`)},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(testCase.body))
			response := httptest.NewRecorder()
			if _, err := decodeLifecycleFeedbackRequest(response, request); err == nil {
				t.Fatalf("invalid body accepted: %q", testCase.body)
			}
		})
	}
}

func TestLifecycleControlRoutesRejectLooseJSONBeforeRepositoryAccess(t *testing.T) {
	operationID, err := queue.NewLifecycleOperationID("http-route-inexact", "41")
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{}
	for _, testCase := range []struct {
		name string
		body string
		run  func(http.ResponseWriter, *http.Request)
	}{
		{name: "feedback", body: `{"operation_id":"` + string(operationID) + `","feedback":"exact","extra":true}`, run: func(w http.ResponseWriter, r *http.Request) {
			server.submitJobFeedback(w, r, 41)
		}},
		{name: "interrupt", body: `{"operation_id":"` + string(operationID) + `","feedback":"exact"} {}`, run: func(w http.ResponseWriter, r *http.Request) {
			server.interruptJob(w, r, 41)
		}},
		{name: "replan", body: `{"operation_id":"` + string(operationID) + `","feedback":"first","feedback":"second"}`, run: func(w http.ResponseWriter, r *http.Request) {
			server.replanJob(w, r, 41)
		}},
		{name: "cancel", body: `{"operation_id":"` + string(operationID) + `","Reason":"exact"}`, run: func(w http.ResponseWriter, r *http.Request) {
			server.cancelJob(w, r, 41)
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(testCase.body))
			response := httptest.NewRecorder()
			testCase.run(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestLifecycleRoutesRejectAliasesAndQueryAuthorityBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()
	server := &Server{}
	validID, err := queue.NewLifecycleOperationID("http-route-canonical", "41")
	if err != nil {
		t.Fatal(err)
	}
	body := `{"operation_id":"` + string(validID) + `","feedback":"exact"}`
	for _, target := range []string{
		"/v1/jobs/041/replan",
		"/v1/jobs/+41/replan",
		"/v1/jobs/%34%31/replan",
		"/v1/jobs/41//replan",
		"/v1/jobs/41/replan/",
		"/v1/jobs/41/replan/extra",
		"/v1/jobs/41/unknown",
	} {
		request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
		response := httptest.NewRecorder()
		server.handleJobByID(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("alias %q status=%d body=%s", target, response.Code, response.Body.String())
		}
	}

	for _, action := range []string{"feedback", "interrupt", "replan", "cancel"} {
		actionBody := body
		if action == "cancel" {
			actionBody = `{"operation_id":"` + string(validID) + `","reason":"exact"}`
		}
		request := httptest.NewRequest(
			http.MethodPost, "/v1/jobs/41/"+action+"?action="+action,
			strings.NewReader(actionBody),
		)
		response := httptest.NewRecorder()
		server.handleJobByID(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("action=%s status=%d body=%s", action, response.Code, response.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/41?include=all", nil)
	response := httptest.NewRecorder()
	server.handleJobByID(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("job GET query status=%d body=%s", response.Code, response.Body.String())
	}
}
