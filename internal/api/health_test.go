package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestHealthIncludesDependencyStatus(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	payload := requestHealthPayload(t, server)

	if payload["status"] != "ok" {
		t.Fatalf("status=%#v want ok", payload["status"])
	}
	dependencies := healthDependencies(t, payload)
	for _, name := range []string{"postgres", "redis", "host_bridge"} {
		dependency := dependencies[name]
		if dependency == nil {
			t.Fatalf("missing dependency %q in %#v", name, dependencies)
		}
		if dependency["status"] != "not_configured" {
			t.Fatalf("%s status=%#v want not_configured", name, dependency["status"])
		}
	}
}

func TestHealthDegradedForInvalidRedisConfig(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		RedisURL:        "http://redis.example.invalid:6379",
		UIRedisRequired: true,
	})
	payload := requestHealthPayload(t, server)

	if payload["status"] != "degraded" {
		t.Fatalf("status=%#v want degraded", payload["status"])
	}
	redis := healthDependencies(t, payload)["redis"]
	if redis["status"] != "error" {
		t.Fatalf("redis status=%#v want error", redis["status"])
	}
	if redis["error"] == "" {
		t.Fatalf("expected redis error, got %#v", redis)
	}
}

func TestHealthDegradedForPostgresPingFailure(t *testing.T) {
	server := NewServer(queue.New(nil), &fakeLLMClient{})
	payload := requestHealthPayload(t, server)

	if payload["status"] != "degraded" {
		t.Fatalf("status=%#v want degraded", payload["status"])
	}
	postgres := healthDependencies(t, payload)["postgres"]
	if postgres["status"] != "error" {
		t.Fatalf("postgres status=%#v want error", postgres["status"])
	}
	if postgres["error"] == "" {
		t.Fatalf("expected postgres error, got %#v", postgres)
	}
}

func TestReadinessFailsWhenRequiredDependencyIsUnreachable(t *testing.T) {
	server := NewServer(queue.New(nil), &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "degraded" {
		t.Fatalf("status=%#v want degraded", payload["status"])
	}
}

func TestReadinessSucceedsWithNoRequiredDependencies(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func requestHealthPayload(t *testing.T, server *Server) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func healthDependencies(t *testing.T, payload map[string]any) map[string]map[string]any {
	t.Helper()
	raw, ok := payload["dependencies"].(map[string]any)
	if !ok {
		t.Fatalf("missing dependencies in %#v", payload)
	}
	out := map[string]map[string]any{}
	for name, value := range raw {
		item, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("dependency %q is not an object: %#v", name, value)
		}
		out[name] = item
	}
	return out
}
