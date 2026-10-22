package inferenceapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNativeProviderConsumesExactBoundedInputOnce(t *testing.T) {
	for _, input := range []Request{
		{Question: "Summarize the supplied note", Context: "The meeting moved to Tuesday.", MaxOutputTokens: 32},
		{Question: "Classify the sentiment", Context: `{"text":"I enjoyed this."}`, MaxOutputTokens: 4},
	} {
		var calls atomic.Int32
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Error("missing backend credential")
			}
			if r.URL.Path == "/healthz" {
				fmt.Fprint(w, `{"status":"ready","model":"native-test","other_capability":true}`)
				return
			}
			if r.URL.Path != "/infer" || r.Method != "POST" {
				t.Error("unexpected inference path")
			}
			var received Request
			if json.NewDecoder(r.Body).Decode(&received) != nil || received != input {
				t.Error("native input was changed")
			}
			calls.Add(1)
			fmt.Fprint(w, `{"text":"result","model":"native-test","output_tokens":2,"finish_reason":"stop"}`)
		}))
		provider, err := NewHTTPProvider(backend.URL, token, "native-test")
		if err != nil {
			t.Fatal(err)
		}
		if err := provider.Check(context.Background()); err != nil {
			t.Fatal(err)
		}
		s, _ := New(provider, "native-test", token)
		body, _ := json.Marshal(input)
		r := httptest.NewRequest("POST", "/v1/integrations/infer", strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		backend.Close()
		if w.Code != 200 || calls.Load() != 1 || !strings.Contains(w.Body.String(), `"text":"result"`) || strings.Contains(w.Body.String(), "context") {
			t.Fatalf("invalid stateless native response: %d %s calls=%d", w.Code, w.Body, calls.Load())
		}
	}
}

func TestNativeProviderRejectsInvalidOrTruncatedOutputWithoutRetry(t *testing.T) {
	for _, body := range []string{
		`{"text":"private input","model":"native-test","output_tokens":3,"finish_reason":"length"}`,
		`{"text":"private input","model":"native-test","output_tokens":50,"finish_reason":"stop"}`,
		`{"text":"private input","model":"other","output_tokens":3,"finish_reason":"stop"}`,
		`{"text":"private input","model":"native-test","finish_reason":"stop"}`,
		`{"text":"private input","model":"native-test","output_tokens":3,"finish_reason":"stop","extra":true}`,
		`{"error":"private input"}`, `{} {}`, strings.Repeat("x", 25000),
	} {
		var calls atomic.Int32
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, body) }))
		provider, _ := NewHTTPProvider(backend.URL, token, "native-test")
		s, _ := New(provider, "native-test", token)
		r := httptest.NewRequest("POST", "/v1/integrations/infer", strings.NewReader(`{"question":"q","context":"x","max_output_tokens":8}`))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		backend.Close()
		if w.Code != 502 || calls.Load() != 1 || strings.Contains(w.Body.String(), "private input") {
			t.Fatalf("invalid failure: %d %s calls=%d", w.Code, w.Body, calls.Load())
		}
	}
}

func TestNativeProviderRejectsRedirectsAndInsecureNetworkOrigins(t *testing.T) {
	var reached atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true) }))
	defer destination.Close()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer backend.Close()
	provider, _ := NewHTTPProvider(backend.URL, token, "native-test")
	if _, err := provider.Predict(context.Background(), Request{Question: "q", MaxOutputTokens: 4}); err == nil || reached.Load() {
		t.Fatal("native inference followed a redirect")
	}
	for _, address := range []string{"http://model.example", "https://user:secret@model.example", "https://model.example?x=1", "file:///tmp/model"} {
		if _, err := NewHTTPProvider(address, token, "native-test"); err == nil {
			t.Fatalf("accepted %s", address)
		}
	}
}
