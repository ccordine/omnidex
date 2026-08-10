package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestJobHistoryEndpointRejectsUnknownStreamAndUnboundedLimit(t *testing.T) {
	t.Parallel()

	server := &Server{}
	for _, target := range []string{
		"/v1/jobs/7/history?stream=unknown&limit=1",
		"/v1/jobs/7/history?stream=steps&limit=0",
		"/v1/jobs/7/history?stream=steps&limit=" + strings.Repeat("9", 20),
		"/v1/jobs/7/history?stream=steps&limit=1&cursor=not-a-cursor",
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		response := httptest.NewRecorder()
		server.handleJobByID(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", target, response.Code, response.Body.String())
		}
	}
}

func TestJobHistoryEndpointRejectsMutationMethods(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost,
		"/v1/jobs/7/history?stream=steps&limit=1", nil)
	response := httptest.NewRecorder()
	(&Server{}).handleJobByID(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLegacyAggregateInspectionEndpointIsNotRouted(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/7/inspection", nil)
	response := httptest.NewRecorder()
	(&Server{}).handleJobByID(response, request)
	if response.Code == http.StatusOK {
		t.Fatalf("legacy aggregate inspection endpoint remained active: %s", response.Body.String())
	}
}

func TestParseJobHistoryRequestUsesBoundedDefaultOnlyWhenLimitIsAbsent(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/?stream=generations", nil)
	parsed, err := parseJobHistoryRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Stream != queue.JobHistoryGenerations || parsed.Limit != defaultJobHistoryPageSize {
		t.Fatalf("request=%+v", parsed)
	}

	request = httptest.NewRequest(http.MethodGet, "/?stream=generations&limit=", nil)
	if _, err := parseJobHistoryRequest(request); err == nil {
		t.Fatal("explicit empty limit was silently defaulted")
	}
}
