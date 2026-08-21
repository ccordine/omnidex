package api

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestParseTelemetryNotifyPayloadRequiresTypedJSON(t *testing.T) {
	if _, err := parseTelemetryNotifyPayload("step_output"); err == nil {
		t.Fatal("legacy raw telemetry notification must be rejected")
	}
	if _, err := parseTelemetryNotifyPayload(`{"run_id":"run-1"}`); err == nil {
		t.Fatal("telemetry notification without event_type must be rejected")
	}
	payload, err := parseTelemetryNotifyPayload(`{"event_type":" step_error ","run_id":" run-1 ","payload":{"job_id":42,"message":" tool failed "}}`)
	if err != nil {
		t.Fatalf("parse typed telemetry notification: %v", err)
	}
	if payload.EventType != "step_error" || payload.RunID != "run-1" || payload.JobID != 42 || payload.Message != "tool failed" {
		t.Fatalf("unexpected normalized payload: %+v", payload)
	}
}

func TestTelemetryJobProgressSignalsIntermediateAndTerminalState(t *testing.T) {
	phase, summary, ok := telemetryJobProgress(telemetryNotifyPayload{EventType: "tool_call_complete", JobID: 42})
	if !ok || phase != realtimeJobChanged || summary != "tool call complete" {
		t.Fatalf("phase=%q summary=%q ok=%t", phase, summary, ok)
	}
	for eventType, expectedSummary := range map[string]string{
		"run_completed": "Job completed",
		"run_failed":    "Job failed",
		"run_cancelled": "Job canceled",
	} {
		phase, summary, ok = telemetryJobProgress(telemetryNotifyPayload{EventType: eventType, JobID: 42})
		if !ok || phase != realtimeJobFinished || summary != expectedSummary {
			t.Fatalf("terminal event=%q phase=%q summary=%q ok=%t", eventType, phase, summary, ok)
		}
	}
	if _, _, ok := telemetryJobProgress(telemetryNotifyPayload{EventType: "tool_call_begin"}); ok {
		t.Fatal("telemetry without job id must not publish job progress")
	}
	_, summary, ok = telemetryJobProgress(telemetryNotifyPayload{
		EventType: "tool_call_begin",
		JobID:     42,
		Message:   "Inspecting the workspace",
	})
	if !ok || summary != "Inspecting the workspace" {
		t.Fatalf("meaningful worker summary=%q ok=%t", summary, ok)
	}
}

func TestRenderMetricsNavBadgesHTML(t *testing.T) {
	html := renderMetricsNavBadgesHTML(queue.TelemetryGlanceSummary{
		LiveRuns:     2,
		RecentErrors: 3,
	})
	if !strings.Contains(html, `data-recyclr-target="metrics-nav-badges"`) {
		t.Fatalf("expected recyclr target wrapper, got %q", html)
	}
	if !strings.Contains(html, ">2<") || !strings.Contains(html, ">3<") {
		t.Fatalf("expected live and error counts in html, got %q", html)
	}
}

func TestRenderMetricsNavBadgesHTMLEmpty(t *testing.T) {
	html := renderMetricsNavBadgesHTML(queue.TelemetryGlanceSummary{})
	if !strings.Contains(html, ">05<") {
		t.Fatalf("expected keyboard hint fallback, got %q", html)
	}
}

func TestBuildMetricsGlanceRealtimeMessageToast(t *testing.T) {
	s := &Server{}
	msg := s.buildMetricsGlanceRealtimeMessage(queue.TelemetryGlanceSummary{RecentErrors: 1}, telemetryNotifyPayload{
		EventType: "step_error",
		Message:   "tool failed",
	})
	if msg.Toast == "" {
		t.Fatal("expected toast for struggle event")
	}
	if msg.EventName != "metrics-glance" {
		t.Fatalf("eventName = %q", msg.EventName)
	}
}
