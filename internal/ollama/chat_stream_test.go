package ollama

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestChatStreamPreservesExactChunksAndRequiresDone(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: request, Body: io.NopCloser(strings.NewReader(
			`{"message":{"role":"assistant","content":" first\n"},"done":false}` + "\n" +
				`{"message":{"role":"assistant","content":"second "},"done":true}` + "\n",
		))}, nil
	})
	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Transport: transport}
	var chunks []string
	result, err := client.ChatStream(context.Background(), "llama3.2", "system", "user", func(value string) error {
		chunks = append(chunks, value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != " first\nsecond " || len(chunks) != 2 || chunks[0] != " first\n" || chunks[1] != "second " {
		t.Fatalf("result=%q chunks=%#v", result, chunks)
	}
}

func TestChatStreamFailsLoudlyOnInvalidOrIncompleteFrames(t *testing.T) {
	t.Parallel()
	for name, response := range map[string]string{
		"malformed": `{"message":` + "\n",
		"duplicate": `{"message":{"content":"a"},"message":{"content":"b"},"done":true}` + "\n",
		"alias":     `{"Message":{"content":"a"},"done":true}` + "\n",
		"no done":   `{"message":{"content":"a"},"done":false}` + "\n",
	} {
		response := response
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Request: request,
					Body: io.NopCloser(strings.NewReader(response))}, nil
			})
			client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
			client.httpClient = &http.Client{Transport: transport}
			if _, err := client.ChatStream(context.Background(), "llama3.2", "system", "user", nil); err == nil {
				t.Fatal("invalid stream frame was silently accepted")
			}
		})
	}
}
