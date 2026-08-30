package queue

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestScrumChannelCardProjectionRejectsRetiredFlowMetric(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"planning_messages": json.RawMessage(`{"planning_messages":2}`),
		"review_gate":       json.RawMessage(`{"assigned_returns":0,"review_bounces":0,"regression_count":0,"play_runs":0,"channel_messages":0,"conversation_chars":0,"incomplete_score":0,"completion_status":"uncertain","signals":[],"review_gate":"pending"}`),
	} {
		t.Run(name, func(t *testing.T) {
			card := DBScrumCard{
				ID: "projection-card", ProjectID: 7, Title: "Projection", Column: "assigned",
				Checklist: json.RawMessage(`[]`), RefFiles: json.RawMessage(`[]`),
				Tags: json.RawMessage(`[]`), TestCriteria: json.RawMessage(`[]`),
				FlowMetrics: raw,
				CreatedAt:   time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
				UpdatedAt:   time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
			}
			if _, err := encodeScrumChannelCardProjection(card); err == nil ||
				!strings.Contains(err.Error(), name) {
				t.Fatalf("retired flow metric encode error=%v", err)
			}
		})
	}
}

func TestScrumChannelCardProjectionRejectsUnboundedOrNoncanonicalFlowMetrics(t *testing.T) {
	base := ScrumFlowMetrics{CompletionStatus: "uncertain", Signals: []string{}}
	for name, mutate := range map[string]func(*ScrumFlowMetrics){
		"transition overflow": func(metrics *ScrumFlowMetrics) { metrics.PlayRuns = 2147483647; metrics.PlayRuns++ },
		"wide overflow":       func(metrics *ScrumFlowMetrics) { metrics.ChannelMessages = maxScrumFlowWideCounter + 1 },
		"duplicate signal":    func(metrics *ScrumFlowMetrics) { metrics.Signals = []string{"same", "same"} },
		"noncanonical signal": func(metrics *ScrumFlowMetrics) { metrics.Signals = []string{" padded"} },
		"unknown outcome":     func(metrics *ScrumFlowMetrics) { metrics.LastPlayOutcome = "agent_done" },
		"unknown column":      func(metrics *ScrumFlowMetrics) { metrics.Column = "complete" },
	} {
		t.Run(name, func(t *testing.T) {
			metrics := base
			mutate(&metrics)
			raw, err := json.Marshal(metrics)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := canonicalScrumFlowMetrics(raw); err == nil {
				t.Fatal("invalid flow metrics were accepted")
			}
		})
	}
}

func TestScrumChannelCardProjectionBindsFlowTimestampToCardRevision(t *testing.T) {
	revision := time.Date(2026, 8, 13, 12, 0, 0, 123456000, time.UTC)
	metrics := ScrumFlowMetrics{
		CompletionStatus: "uncertain", Signals: []string{},
		UpdatedAt: "2026-08-13T12:00:00Z",
	}
	raw, err := json.Marshal(metrics)
	if err != nil {
		t.Fatal(err)
	}
	card := DBScrumCard{
		ID: "projection-revision", ProjectID: 7, Title: "Projection", Column: "assigned",
		Checklist: json.RawMessage(`[]`), RefFiles: json.RawMessage(`[]`),
		Tags: json.RawMessage(`[]`), TestCriteria: json.RawMessage(`[]`),
		FlowMetrics: raw, CreatedAt: revision, UpdatedAt: revision,
	}
	if _, err := encodeScrumChannelCardProjection(card); err == nil ||
		!strings.Contains(err.Error(), "differs from card revision") {
		t.Fatalf("mismatched flow/card revision error=%v", err)
	}
}

func TestScrumChannelCardProjectionRejectsExplicitEmptyOptionalFlowIdentity(t *testing.T) {
	for _, field := range []string{"last_play_outcome", "column", "updated_at"} {
		t.Run(field, func(t *testing.T) {
			raw := json.RawMessage(`{"assigned_returns":0,"review_bounces":0,"regression_count":0,"play_runs":0,"channel_messages":0,"conversation_chars":0,"incomplete_score":0,"completion_status":"uncertain","signals":[],"` + field + `":""}`)
			if _, err := canonicalScrumFlowMetrics(raw); err == nil || !strings.Contains(err.Error(), "must be absent") {
				t.Fatalf("explicit empty %s error=%v", field, err)
			}
		})
	}
}

func TestScrumChannelCardProjectionRequiresEveryCoreFlowMetricField(t *testing.T) {
	required := []string{
		"assigned_returns", "review_bounces", "regression_count", "play_runs",
		"channel_messages", "conversation_chars", "incomplete_score", "completion_status", "signals",
	}
	for _, missing := range required {
		t.Run(missing, func(t *testing.T) {
			fields := map[string]any{
				"assigned_returns": 0, "review_bounces": 0, "regression_count": 0, "play_runs": 0,
				"channel_messages": 0, "conversation_chars": 0, "incomplete_score": 0,
				"completion_status": "uncertain", "signals": []string{},
			}
			delete(fields, missing)
			raw, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := canonicalScrumFlowMetrics(raw); err == nil || !strings.Contains(err.Error(), "requires exact field") {
				t.Fatalf("missing %s error=%v", missing, err)
			}
		})
	}
}

func TestScrumChannelCardProjectionRejectsExplicitNullFlowMetricFields(t *testing.T) {
	fields := []string{
		"assigned_returns", "review_bounces", "regression_count", "play_runs",
		"channel_messages", "conversation_chars", "incomplete_score", "completion_status", "signals",
		"last_play_outcome", "column", "updated_at",
	}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			metric := map[string]any{
				"assigned_returns": 0, "review_bounces": 0, "regression_count": 0, "play_runs": 0,
				"channel_messages": 0, "conversation_chars": 0, "incomplete_score": 0,
				"completion_status": "uncertain", "signals": []string{}, field: nil,
			}
			raw, err := json.Marshal(metric)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := canonicalScrumFlowMetrics(raw); err == nil || !strings.Contains(err.Error(), "null") {
				t.Fatalf("explicit null %s error=%v", field, err)
			}
		})
	}
}

func TestScrumChannelCardProjectionRejectsAbsentRawFlowMetrics(t *testing.T) {
	if _, err := canonicalScrumFlowMetrics(nil); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("absent raw flow metrics error=%v", err)
	}
	if canonical, err := canonicalScrumFlowMetrics(json.RawMessage(`{}`)); err != nil || string(canonical) != `{}` {
		t.Fatalf("explicit empty flow metrics canonical=%s error=%v", canonical, err)
	}
}
