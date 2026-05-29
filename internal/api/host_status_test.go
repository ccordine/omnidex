package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func TestHostBridgeStatusUnconfigured(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	status := server.collectHostBridgeStatus(context.Background())
	if status.Configured || status.Reachable || status.PickerReady {
		t.Fatalf("expected unconfigured status, got %#v", status)
	}
	if len(status.Suggestions) == 0 {
		t.Fatal("expected remediation suggestions")
	}
}

func TestHostBridgeStatusReachable(t *testing.T) {
	agent := httptest.NewServer((&hostbridge.Server{}).Handler())
	defer agent.Close()

	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		HostAgentURL: agent.URL,
	})
	status := server.collectHostBridgeStatus(context.Background())
	if !status.Configured || !status.Reachable || !status.PickerReady {
		t.Fatalf("expected reachable status, got %#v", status)
	}
	if len(status.Suggestions) != 0 {
		t.Fatalf("expected no suggestions when reachable, got %#v", status.Suggestions)
	}
}

func TestHostBridgeStatusUnreachable(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		HostAgentURL: "http://127.0.0.1:1",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	status := server.collectHostBridgeStatus(ctx)
	if !status.Configured || status.Reachable {
		t.Fatalf("expected configured-but-unreachable, got %#v", status)
	}
	if status.Error == "" || len(status.Suggestions) == 0 {
		t.Fatalf("expected error and suggestions, got %#v", status)
	}
}

func TestHandleHostBridgeStatusJSON(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/host/status", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload hostBridgeStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Reachable {
		t.Fatal("expected unreachable in default server")
	}
}
