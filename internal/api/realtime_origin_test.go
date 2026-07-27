package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestRealtimeOriginRequiresSameHost(t *testing.T) {
	request := httptest.NewRequest("GET", "http://omni.test/v1/realtime/ws", nil)
	request.Host = "omni.test"
	request.Header.Set("Origin", "https://evil.test")
	if realtimeOriginAllowed(request) {
		t.Fatal("cross-origin realtime websocket must be rejected")
	}
	request.Header.Set("Origin", "https://omni.test")
	if !realtimeOriginAllowed(request) {
		t.Fatal("same-host realtime websocket must be accepted")
	}
}

func TestRealtimeHandlerRejectsCrossOriginBeforeSubscribing(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	request := httptest.NewRequest(http.MethodGet, "http://omni.test/v1/realtime/ws", nil)
	request.Host = "omni.test"
	request.Header.Set("Origin", "https://evil.test")
	recorder := httptest.NewRecorder()

	server.handleRealtimeWS(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d", recorder.Code, http.StatusForbidden)
	}
	server.realtimeHub.mu.Lock()
	clientCount := len(server.realtimeHub.clients)
	server.realtimeHub.mu.Unlock()
	if clientCount != 0 {
		t.Fatalf("realtime clients=%d want 0 after rejected origin", clientCount)
	}
}

func TestRealtimeWebSocketIsAvailableWithoutQueueRepository(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	host := httptest.NewServer(server.Handler())
	defer host.Close()

	wsURL := "ws" + strings.TrimPrefix(host.URL, "http") + "/v1/realtime/ws"
	connection, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		if response != nil {
			t.Fatalf("websocket dial status=%d: %v", response.StatusCode, err)
		}
		t.Fatalf("websocket dial: %v", err)
	}
	defer connection.Close()

	var connected realtimeMessage
	if err := connection.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected message: %v", err)
	}
	if connected.EventName != "realtime-connected" {
		raw, _ := json.Marshal(connected)
		t.Fatalf("connected message=%s want realtime-connected", raw)
	}
}

func TestRealtimeHandlerDoesNotIgnoreSocketDeadlineFailures(t *testing.T) {
	source := readAPISource(t, "realtime_handlers.go")
	for _, forbidden := range []string{
		"_ = conn.SetReadDeadline",
		"_ = conn.SetWriteDeadline",
		"_ = writeMessage(websocket.CloseMessage",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("realtime handler silently ignores transport failure %q", forbidden)
		}
	}
}
