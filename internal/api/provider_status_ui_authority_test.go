package api

import (
	"os"
	"strings"
	"testing"
)

func TestResearchStatusHasNoClientRendererFallback(t *testing.T) {
	renderSource, err := os.ReadFile("web/src/lib/render.ts")
	if err != nil {
		t.Fatalf("read client render source: %v", err)
	}
	if strings.Contains(string(renderSource), "function renderResearchStatus") {
		t.Fatal("legacy client-side research status renderer remains")
	}
	coordinatorSource, err := os.ReadFile("web/src/lib/chat_system_coordinator.ts")
	if err != nil {
		t.Fatalf("read system coordinator source: %v", err)
	}
	if strings.Contains(string(coordinatorSource), "renderResearchStatus(") {
		t.Fatal("system coordinator still routes through the client-side research status renderer")
	}
	if !strings.Contains(string(coordinatorSource), `payload.html`) {
		t.Fatal("system coordinator does not require the server-rendered status fragment")
	}
}
