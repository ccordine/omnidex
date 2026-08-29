package hostbridge

import (
	"os"
	"strings"
	"testing"
)

func TestScreenMonitorProductionHasNoFullListMaterialization(t *testing.T) {
	for _, path := range []string{"screen_linux.go", "screen_monitor_linux.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) && path == "screen_monitor_linux.go" {
				t.Fatalf("bounded monitor scanner is missing")
			}
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{"json.Unmarshal(", "strings.Split(", "listScreenMonitors("} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s contains forbidden full-list path %q", path, forbidden)
			}
		}
	}
}
