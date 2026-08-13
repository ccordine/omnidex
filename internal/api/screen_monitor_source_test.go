package api

import (
	"os"
	"strings"
	"testing"
)

func TestScreenMonitorProductionUsesTypedBoundedPage(t *testing.T) {
	for _, path := range []string{"screen_handlers.go", "ui_screen_component.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{"proxyHostBridgeJSON", `payload["monitors"]`, "[]any"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s contains untyped monitor path %q", path, forbidden)
			}
		}
	}
}
