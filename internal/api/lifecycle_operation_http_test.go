package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestLifecycleControlEndpointsRequireExplicitOperationIdentity(t *testing.T) {
	server := &Server{}
	cases := []struct {
		name  string
		field string
		run   func(http.ResponseWriter, *http.Request)
	}{
		{name: "feedback", field: "feedback", run: func(writer http.ResponseWriter, request *http.Request) {
			server.submitJobFeedback(writer, request, 41)
		}},
		{name: "interrupt", field: "feedback", run: func(writer http.ResponseWriter, request *http.Request) {
			server.interruptJob(writer, request, 41)
		}},
		{name: "replan", field: "feedback", run: func(writer http.ResponseWriter, request *http.Request) {
			server.replanJob(writer, request, 41)
		}},
		{name: "cancel", field: "reason", run: func(writer http.ResponseWriter, request *http.Request) {
			server.cancelJob(writer, request, 41)
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"`+testCase.field+`":"Continue."}`))
			response := httptest.NewRecorder()
			testCase.run(response, request)
			if response.Code != http.StatusBadRequest ||
				!strings.Contains(response.Body.String(), "lifecycle operation ID") {
				t.Fatalf("response code=%d body=%q", response.Code, response.Body.String())
			}
		})
	}
}

func TestCancelLifecycleConflictsUseHTTPConflict(t *testing.T) {
	for _, err := range []error{
		queue.ErrLifecycleOperationConflict,
		queue.ErrStepNotWritable,
		fmt.Errorf("wrapped: %w", queue.ErrLifecycleOperationConflict),
	} {
		if status := cancelJobHTTPStatus(err); status != http.StatusConflict {
			t.Fatalf("error=%v status=%d", err, status)
		}
	}
	if status := cancelJobHTTPStatus(errors.New("invalid cancellation")); status != http.StatusBadRequest {
		t.Fatalf("ordinary validation status=%d", status)
	}
}
