package queue

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestTelemetryRecordMethodsExposeOnlyCurrentModelCallWriter(t *testing.T) {
	blob, err := os.ReadFile("telemetry.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(blob)
	if !strings.Contains(source, "func (r *Repository) RecordTelemetryModelCall") {
		t.Fatal("current model-call telemetry writer is missing")
	}
	for _, retired := range []string{
		"RecordTelemetryRun", "CompleteTelemetryRun", "RecordTelemetryEvent",
		"RecordTelemetryToolCall", "RecordTelemetryCommandObservation",
		"RecordTelemetryObjective", "RecordTelemetryRecovery",
		"RecordTelemetryPlaybookUsage", "RecordTelemetryBenchmarkResult",
	} {
		if strings.Contains(source, retired) {
			t.Fatalf("retired telemetry write side remains: %q", retired)
		}
	}
}

func TestTelemetryJSONEncodingIsExplicitAndCanonical(t *testing.T) {
	got, err := encodeTelemetryJSON("optional metadata", nil)
	if err != nil || string(got) != "{}" {
		t.Fatalf("encode nil telemetry JSON=%s error=%v want {}", got, err)
	}
	raw := json.RawMessage(`{"ok":true}`)
	got, err = encodeTelemetryJSON("canonical metadata", raw)
	if err != nil || string(got) != `{"ok":true}` {
		t.Fatalf("encode canonical telemetry JSON=%s error=%v", got, err)
	}

	for name, value := range map[string]any{
		"blank label":       map[string]any{"ok": true},
		"empty raw":         json.RawMessage{},
		"invalid raw":       json.RawMessage(`{"ok":`),
		"null raw":          json.RawMessage(`null`),
		"whitespace raw":    json.RawMessage(" {\"ok\":true}"),
		"unordered raw":     json.RawMessage(`{"z":1,"a":2}`),
		"duplicate-key raw": json.RawMessage(`{"ok":true,"ok":false}`),
		"marshal failure":   make(chan int),
	} {
		label := name
		if name == "blank label" {
			label = ""
		}
		if encoded, err := encodeTelemetryJSON(label, value); err == nil {
			t.Errorf("encodeTelemetryJSON(%s) accepted %s", name, encoded)
		}
	}
}

func TestTelemetryWritersRejectUnencodableJSONBeforeDatabaseAccess(t *testing.T) {
	repository := &Repository{}
	invalid := make(chan int)
	if err := repository.RecordTelemetryModelCall(
		context.Background(), TelemetryModelCallRecord{Metadata: invalid},
	); err == nil || !strings.Contains(err.Error(), "model-call metadata") {
		t.Fatalf("model-call telemetry encoding error=%v", err)
	}
	if err := repository.RecordContextShrinkMetric(
		context.Background(), ContextShrinkMetricRecord{Source: "test", Metadata: invalid},
	); err == nil || !strings.Contains(err.Error(), "context shrink metadata") {
		t.Fatalf("context shrink telemetry encoding error=%v", err)
	}
	if err := repository.RecordLLMContextUsage(
		context.Background(), LLMContextUsageRecord{Source: "test", Metadata: invalid},
	); err == nil || !strings.Contains(err.Error(), "LLM context usage metadata") {
		t.Fatalf("LLM context telemetry encoding error=%v", err)
	}
	if err := repository.RecordTelemetryJobEventNow(
		context.Background(), 1, "test_event", invalid,
	); err == nil || !strings.Contains(err.Error(), "job event payload") {
		t.Fatalf("job-event telemetry encoding error=%v", err)
	}
	if err := completeTelemetryRunForJob(
		context.Background(), nil, 1, "completed", invalid, map[string]any{},
	); err == nil || !strings.Contains(err.Error(), "run completion summary") {
		t.Fatalf("run-completion telemetry encoding error=%v", err)
	}
	if err := recordTelemetryJobEvent(
		context.Background(), nil, 1, "test_event", invalid,
	); err == nil || !strings.Contains(err.Error(), "job event payload") {
		t.Fatalf("transactional job-event telemetry encoding error=%v", err)
	}
}
