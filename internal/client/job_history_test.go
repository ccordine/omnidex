package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestJobHistoryClientUsesExactBoundedEndpoint(t *testing.T) {
	t.Parallel()

	requestSeen := ""
	client := &Client{
		baseURL: "http://omnidex.test",
		httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestSeen = request.Method + " " + request.URL.RequestURI()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"job_id":17,"stream":"steps","steps":[],"next_cursor":"next"}`,
				)),
			}, nil
		})},
	}

	page, err := client.JobHistory(context.Background(), 17, queue.JobHistoryRequest{
		Stream: queue.JobHistorySteps, Limit: 2, Cursor: "opaque",
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.JobID != 17 || page.Stream != queue.JobHistorySteps || page.NextCursor != "next" {
		t.Fatalf("page=%+v", page)
	}
	if requestSeen != "GET /v1/jobs/17/history?cursor=opaque&limit=2&stream=steps" {
		t.Fatalf("request=%q", requestSeen)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
