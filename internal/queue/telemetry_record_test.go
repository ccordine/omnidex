package queue

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestTelemetryRecordMethodsCoverWriteSideTables(t *testing.T) {
	required := []string{
		"func (r *Repository) RecordTelemetryRun",
		"func (r *Repository) CompleteTelemetryRun",
		"func (r *Repository) RecordTelemetryEvent",
		"func (r *Repository) RecordTelemetryModelCall",
		"func (r *Repository) RecordTelemetryToolCall",
		"func (r *Repository) RecordTelemetryCommandObservation",
		"func (r *Repository) RecordTelemetryObjective",
		"func (r *Repository) RecordTelemetryRecovery",
		"func (r *Repository) RecordTelemetryPlaybookUsage",
		"func (r *Repository) RecordTelemetryBenchmarkResult",
		"ON CONFLICT (run_id, command_id)",
	}
	blob, err := os.ReadFile("telemetry.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(blob)
	for _, want := range required {
		if !strings.Contains(source, want) {
			t.Fatalf("telemetry write side missing %q", want)
		}
	}
}

func TestTelemetryJSONParamsAreRedactionFriendlyDefaults(t *testing.T) {
	if got := string(jsonParam(nil)); got != "{}" {
		t.Fatalf("jsonParam(nil)=%s want {}", got)
	}
	if got := string(jsonArrayParam(nil)); got != "[]" {
		t.Fatalf("jsonArrayParam(nil)=%s want []", got)
	}
	raw := json.RawMessage(`{"ok":true}`)
	if got := string(jsonParam(raw)); got != `{"ok":true}` {
		t.Fatalf("jsonParam(raw)=%s", got)
	}
}
