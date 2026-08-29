package api

import (
	"os"
	"strings"
	"testing"
)

func TestResearchStatusHasNoClientRendererFallback(t *testing.T) {
	if _, err := os.Stat("web/src/lib/render.ts"); err == nil {
		t.Fatal("legacy client-side component renderer remains")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	coordinatorSource, err := os.ReadFile("web/src/lib/chat_system_coordinator.ts")
	if err != nil {
		t.Fatalf("read system coordinator source: %v", err)
	}
	if strings.Contains(string(coordinatorSource), "renderResearchStatus(") {
		t.Fatal("system coordinator still routes through the client-side research status renderer")
	}
	for _, required := range []string{`requireServerComponentBundle(payload, "Research status")`, "renderComponentBundle("} {
		if !strings.Contains(string(coordinatorSource), required) {
			t.Fatalf("system coordinator lacks server-rendered status authority %q", required)
		}
	}
}
