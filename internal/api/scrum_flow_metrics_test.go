package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestScrumFlowHasNoPostCommitParallelWriter(t *testing.T) {
	for _, path := range []string{"scrum_flow_metrics.go", "scrum_flow_metrics_service.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"trackScrumCardFlow", "recordScrumFlowEvent", "computeScrumFlowMetrics",
			"RecordScrumFlowEvent", "UpdateScrumCardFlowMetrics", "ListScrumFlowEvents",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("%s retains parallel flow authority %q", path, forbidden)
			}
		}
	}
}

func TestSummarizeScrumFlowMetrics(t *testing.T) {
	cards := []ScrumCard{
		{FlowMetrics: mustJSON(ScrumFlowMetrics{CompletionStatus: "likely_incomplete", AssignedReturns: 1})},
		{FlowMetrics: mustJSON(ScrumFlowMetrics{CompletionStatus: "likely_complete"})},
	}
	summary, err := summarizeScrumFlowMetrics(cards)
	if err != nil {
		t.Fatal(err)
	}
	if summary.LikelyIncomplete != 1 || summary.LikelyComplete != 1 {
		t.Fatalf("summary=%+v", summary)
	}
}

func TestScrumFlowMetricsRejectMalformedStoredAuthority(t *testing.T) {
	for _, raw := range []json.RawMessage{
		nil,
		json.RawMessage(`{"completion_status":"uncertain"`),
		json.RawMessage(`{"planning_messages":2}`),
		json.RawMessage(`{"assigned_returns":0,"review_bounces":0,"regression_count":0,"play_runs":0,"channel_messages":0,"conversation_chars":0,"incomplete_score":0,"completion_status":"invented","signals":[]}`),
		json.RawMessage(`{"assigned_returns":0,"review_bounces":0,"regression_count":0,"play_runs":0,"channel_messages":0,"conversation_chars":0,"incomplete_score":0,"completion_status":"uncertain","signals":[],"updated_at":"2026-08-13T08:00:00-04:00"}`),
		json.RawMessage(`{"assigned_returns":0,"review_bounces":0,"regression_count":0,"play_runs":0,"channel_messages":0,"conversation_chars":0,"incomplete_score":0,"completion_status":"uncertain","signals":[],"updated_at":"2026-08-13T12:00:00.1234567Z"}`),
		json.RawMessage(`{"assigned_returns":0,"review_bounces":0,"regression_count":0,"play_runs":0,"channel_messages":0,"conversation_chars":0,"incomplete_score":0,"completion_status":"uncertain","signals":[],"updated_at":""}`),
		json.RawMessage(`{"assigned_returns":0,"review_bounces":0,"regression_count":0,"play_runs":0,"channel_messages":0,"conversation_chars":0,"incomplete_score":0,"completion_status":"uncertain","signals":[],"last_play_outcome":""}`),
		json.RawMessage(`{"assigned_returns":0,"review_bounces":0,"regression_count":0,"play_runs":0,"channel_messages":0,"conversation_chars":0,"incomplete_score":0,"completion_status":"uncertain","signals":[],"review_gate":""}`),
		json.RawMessage(`{"assigned_returns":0,"review_bounces":0,"regression_count":0,"play_runs":0,"channel_messages":0,"conversation_chars":0,"incomplete_score":0,"completion_status":"uncertain","signals":[],"column":""}`),
	} {
		if _, err := parseScrumFlowMetrics(raw); err == nil {
			t.Fatalf("parseScrumFlowMetrics(%s) unexpectedly accepted invalid durable state", raw)
		}
	}
}

func TestScrumFragmentsPropagateMalformedStoredFlowMetrics(t *testing.T) {
	card := ScrumCard{ID: "card-1", Column: "assigned", FlowMetrics: json.RawMessage(`{"planning_messages":2}`)}
	if _, err := renderScrumCardHTML(card); err == nil {
		t.Fatal("renderScrumCardHTML unexpectedly invented presentation for invalid durable flow metrics")
	}
}

func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
