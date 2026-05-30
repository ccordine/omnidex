package api

import (
	"net/http/httptest"
	"testing"
)

func TestTerminalUseDirectBridgeDefaultsToProxy(t *testing.T) {
	t.Setenv("HOST_BRIDGE_PUBLIC_WS_URL", "")
	t.Setenv("HOST_BRIDGE_PUBLIC_URL", "")
	t.Setenv("OMNI_TERMINAL_DIRECT", "")
	t.Setenv("OMNI_TERMINAL_VIA_CORE", "")

	req := httptest.NewRequest("GET", "http://192.168.1.50:8090/v1/host/terminal/preflight", nil)
	if terminalUseDirectBridge(req, "http://192.168.1.50:8090") {
		t.Fatal("expected proxy mode by default")
	}
}

func TestTerminalUseDirectBridgeExplicitPublicURL(t *testing.T) {
	t.Setenv("HOST_BRIDGE_PUBLIC_WS_URL", "ws://192.168.1.50:8091")
	t.Setenv("OMNI_TERMINAL_DIRECT", "")

	req := httptest.NewRequest("GET", "http://192.168.1.50:8090/v1/host/terminal/preflight", nil)
	if !terminalUseDirectBridge(req, "http://192.168.1.50:8090") {
		t.Fatal("expected direct mode when HOST_BRIDGE_PUBLIC_WS_URL is set")
	}
}

func TestBrowserBridgeWSBaseUsesRequestHost(t *testing.T) {
	t.Setenv("HOST_BRIDGE_PUBLIC_WS_URL", "")
	req := httptest.NewRequest("GET", "http://192.168.1.77:8090/v1/host/terminal/preflight", nil)
	got := browserBridgeWSBase(req, "http://host.docker.internal:8090")
	if got != "ws://192.168.1.77:8091" {
		t.Fatalf("got %q", got)
	}
}
