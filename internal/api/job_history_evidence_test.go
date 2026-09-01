package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestParseJobHistoryRequestRejectsRawEvidenceStreams(t *testing.T) {
	t.Parallel()
	for _, stream := range []queue.JobHistoryStream{
		"station_calls",
		"verification_commands",
	} {
		stream := stream
		t.Run(string(stream), func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(
				"GET", "/v1/jobs/1/history?stream="+string(stream)+"&limit=7", nil,
			)
			if _, err := parseJobHistoryRequest(request); err == nil {
				t.Fatalf("raw evidence stream %q was exposed on public job history", stream)
			}
		})
	}
}

func TestJobHistoryResponsesAreNeverCached(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(
		"GET", "/v1/jobs/1/history?stream=station_calls", nil,
	)
	response := httptest.NewRecorder()
	(&Server{}).jobHistory(response, request, 1)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", response.Code, http.StatusBadRequest)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control=%q want no-store", got)
	}
}
