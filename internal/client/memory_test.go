package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestAddMemoriesUsesOneExactBatchRequest(t *testing.T) {
	requests := 0
	transport := memoryRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Path != "/v1/memory/batch" || r.Method != http.MethodPost {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Memories []model.MemoryInput `json:"memories"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		memories := make([]model.MemoryChunk, len(body.Memories))
		for index, input := range body.Memories {
			memories[index] = model.MemoryChunk{
				ID: int64(index + 1), Scope: input.Scope,
				Source: input.Source, Kind: input.Kind, Content: input.Content,
			}
		}
		encoded, err := json.Marshal(map[string]any{"memories": memories})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusCreated, Header: make(http.Header),
			Body: io.NopCloser(bytes.NewReader(encoded)), Request: r,
		}, nil
	})
	inputs := []model.MemoryInput{
		{Scope: model.MemoryScope{ProjectID: 1, ChannelID: "channel-one"}, Source: "source:1", Kind: "reference", Content: "first"},
		{Scope: model.MemoryScope{ProjectID: 1, ChannelID: "channel-one"}, Source: "source:2", Kind: "reference", Content: "second"},
	}
	client := &Client{baseURL: "http://memory.test", httpClient: &http.Client{Transport: transport}}
	if _, err := client.AddMemories(context.Background(), inputs); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("memory batch made %d HTTP requests", requests)
	}
}

func TestAddMemoriesRejectsInvalidLaterInputBeforeHTTP(t *testing.T) {
	requests := 0
	transport := memoryRoundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, nil
	})
	client := &Client{baseURL: "http://memory.test", httpClient: &http.Client{Transport: transport}}
	_, err := client.AddMemories(context.Background(), []model.MemoryInput{
		{Scope: model.MemoryScope{ProjectID: 1, ChannelID: "channel-one"}, Source: "source:1", Kind: "reference", Content: "first"},
		{Scope: model.MemoryScope{ProjectID: 1, ChannelID: "channel-one"}, Source: "source:2", Kind: "unknown", Content: "second"},
	})
	if err == nil || requests != 0 {
		t.Fatalf("error=%v requests=%d", err, requests)
	}
}

type memoryRoundTripFunc func(*http.Request) (*http.Response, error)

func (function memoryRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
