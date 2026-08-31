package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestEnqueueCodingSendsOnlyClientWorkspaceRoot(t *testing.T) {
	var payload struct {
		Instruction string            `json:"instruction"`
		Pipeline    string            `json:"pipeline"`
		Metadata    map[string]string `json:"metadata"`
	}
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/jobs" {
			return nil, fmt.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			return nil, fmt.Errorf("decode request payload: %w", err)
		}
		encoded, err := json.Marshal(map[string]any{"job": model.Job{
			ID: 1, Instruction: payload.Instruction, Pipeline: payload.Pipeline,
		}})
		if err != nil {
			return nil, fmt.Errorf("encode response payload: %w", err)
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(encoded)),
		}, nil
	})

	client := &Client{
		baseURL:    "http://client.test",
		httpClient: &http.Client{Transport: transport},
	}
	if _, err := client.EnqueueCoding(
		context.Background(), "make the requested change", "/host/projects/example", "session-1",
	); err != nil {
		t.Fatalf("enqueue coding: %v", err)
	}
	if payload.Metadata["client_cwd"] != "/host/projects/example" {
		t.Fatalf("client_cwd=%q, want exact client workspace", payload.Metadata["client_cwd"])
	}
	if _, exists := payload.Metadata["host_env_cwd"]; exists {
		t.Fatal("enqueue payload retained duplicate host_env_cwd")
	}
}
