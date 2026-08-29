package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRetiredExternalAgentEventsHaveNoRuntimeWriterOrMetricDictionary(t *testing.T) {
	for _, path := range []string{"telemetry_bridge.go", "operations_metrics.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"external_agent_started", "external_agent_failed", "external_agent_unavailable",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s still accepts retired event %q", path, forbidden)
			}
		}
	}
}
