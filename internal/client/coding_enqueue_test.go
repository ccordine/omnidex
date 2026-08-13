package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCodingClientUsesOnlyExactCodingTransport(t *testing.T) {
	t.Parallel()
	exact := "  preserve the coding instruction\t "
	client := &Client{
		baseURL: "http://omnidex.test",
		httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodPost || request.URL.Path != "/v1/jobs" {
				t.Fatalf("request=%s %s", request.Method, request.URL.Path)
			}
			var payload struct {
				Instruction string         `json:"instruction"`
				Pipeline    string         `json:"pipeline"`
				Metadata    map[string]any `json:"metadata"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Instruction != exact || payload.Pipeline != model.PipelineCoding || payload.Metadata["key"] != "value" {
				t.Fatalf("payload=%+v", payload)
			}
			response, err := json.Marshal(map[string]any{"job": model.Job{
				ID: 17, Instruction: exact, Pipeline: model.PipelineCoding,
			}})
			if err != nil {
				t.Fatal(err)
			}
			return &http.Response{
				StatusCode: http.StatusCreated,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string(response))),
			}, nil
		})},
	}
	job, err := client.EnqueueCoding(context.Background(), exact, map[string]any{"key": "value"})
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != 17 || job.Instruction != exact || job.Pipeline != model.PipelineCoding {
		t.Fatalf("job=%+v", job)
	}
}

func TestCodingClientRejectsBlankInstructionBeforeHTTP(t *testing.T) {
	t.Parallel()
	client := &Client{httpClient: http.DefaultClient}
	if _, err := client.EnqueueCoding(context.Background(), " \n\t ", nil); err == nil {
		t.Fatal("blank coding instruction was accepted")
	}
}
