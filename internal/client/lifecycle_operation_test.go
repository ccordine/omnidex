package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestJobControlClientCarriesExactLifecycleOperationIdentity(t *testing.T) {
	id, err := queue.NewLifecycleOperationID("client-test", "operation")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		path       string
		contentKey string
		content    string
		call       func(*Client) error
	}{
		{name: "feedback", path: "/v1/jobs/41/feedback", contentKey: "feedback", content: "Continue.", call: func(client *Client) error {
			_, err := client.SubmitFeedback(context.Background(), 41, id, "Continue.")
			return err
		}},
		{name: "interrupt", path: "/v1/jobs/41/interrupt", contentKey: "feedback", content: "Continue.", call: func(client *Client) error {
			_, err := client.Interrupt(context.Background(), 41, id, "Continue.")
			return err
		}},
		{name: "replan", path: "/v1/jobs/41/replan", contentKey: "feedback", content: "Continue.", call: func(client *Client) error {
			_, err := client.Replan(context.Background(), 41, id, "Continue.")
			return err
		}},
		{name: "cancel", path: "/v1/jobs/41/cancel", contentKey: "reason", content: "Stop.", call: func(client *Client) error {
			_, err := client.Cancel(context.Background(), queue.CancelJobCommand{
				OperationID: id, JobID: 41, Reason: "Stop.",
			})
			return err
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := &Client{
				baseURL: "http://omnidex.test",
				httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					if request.Method != http.MethodPost || request.URL.Path != testCase.path {
						t.Fatalf("request=%s %s", request.Method, request.URL.Path)
					}
					var payload map[string]any
					if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					if payload["operation_id"] != string(id) || payload[testCase.contentKey] != testCase.content {
						t.Fatalf("payload=%+v", payload)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(`{"job":{"id":41,"current_generation":1}}`)),
					}, nil
				})},
			}
			if err := testCase.call(client); err != nil {
				t.Fatal(err)
			}
		})
	}
}
