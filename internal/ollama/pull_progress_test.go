package ollama

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPullModelProgressStreamsCanonicalProviderState(t *testing.T) {
	client := New("http://ollama.invalid", "", "", 0, 0)
	client.httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/pull" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != `{"name":"dolphin3:8b","model":"dolphin3:8b","stream":true}` {
			t.Fatalf("body=%s", raw)
		}
		return jsonHTTPResponse(http.StatusOK, strings.Join([]string{
			`{"status":"pulling manifest"}`,
			`{"status":"downloading","digest":"sha256:abc","total":100,"completed":25}`,
			`{"status":"success"}`,
		}, "\n")+"\n"), nil
	})

	var events []PullProgress
	err := client.PullModelProgress(context.Background(), "dolphin3:8b", func(event PullProgress) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[1].Digest != "sha256:abc" || events[1].Completed != 25 || events[1].Total != 100 {
		t.Fatalf("events=%+v", events)
	}
}

func TestPullModelProgressRejectsMalformedOrProviderErrorState(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "duplicate field", body: `{"status":"one","status":"two"}` + "\n"},
		{name: "trailing value", body: `{"status":"success"} true` + "\n"},
		{name: "provider error", body: `{"status":"pulling","error":"manifest unavailable"}` + "\n"},
		{name: "invalid progress", body: `{"status":"downloading","total":10,"completed":11}` + "\n"},
		{name: "missing terminal", body: `{"status":"pulling manifest"}` + "\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := New("http://ollama.invalid", "", "", 0, 0)
			client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonHTTPResponse(http.StatusOK, test.body), nil
			})
			if err := client.PullModelProgress(context.Background(), "model:tag", func(PullProgress) error { return nil }); err == nil {
				t.Fatal("expected invalid pull stream to fail")
			}
		})
	}
}
