package queue

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScrumFlowMetricDeltaRejectsUnregisteredAuthority(t *testing.T) {
	t.Parallel()
	tests := []ScrumFlowMetricDelta{
		{},
		{Kind: "conversation"},
		{Kind: ScrumFlowMetricColumnMove, FromColumn: "review", ToColumn: " assigned"},
		{Kind: ScrumFlowMetricColumnMove, FromColumn: "review", ToColumn: "review"},
		{Kind: ScrumFlowMetricColumnMove, FromColumn: "review", ToColumn: "in_progress", Outcome: "failed"},
		{Kind: ScrumFlowMetricPlayStarted, Outcome: "success"},
		{Kind: ScrumFlowMetricPlayFinish, Outcome: "blocked"},
	}
	for index, delta := range tests {
		metrics := ScrumFlowMetrics{}
		if err := applyScrumFlowMetricDelta(&metrics, delta); err == nil {
			t.Fatalf("case %d unexpectedly accepted: %+v", index, delta)
		}
	}
}

func TestScrumFlowMetricsAdvanceFromTypedDeltasWithoutHistoryScan(t *testing.T) {
	t.Parallel()
	metrics := ScrumFlowMetrics{AssignedReturns: 2, ReviewBounces: 1, RegressionCount: 3, PlayRuns: 4}
	if err := applyScrumFlowMetricDelta(&metrics, ScrumFlowMetricDelta{
		Kind: ScrumFlowMetricColumnMove, FromColumn: "review", ToColumn: "in_progress",
	}); err != nil {
		t.Fatal(err)
	}
	if err := applyScrumFlowMetricDelta(&metrics, ScrumFlowMetricDelta{
		Kind: ScrumFlowMetricPlayFinish, Outcome: "failed",
	}); err != nil {
		t.Fatal(err)
	}
	if metrics.AssignedReturns != 2 || metrics.ReviewBounces != 2 ||
		metrics.RegressionCount != 4 || metrics.PlayRuns != 4 || metrics.LastPlayOutcome != "failed" {
		t.Fatalf("incremental metrics=%+v", metrics)
	}
	for _, path := range []string{"scrum_flow_authority.go", "scrum_flow_metrics_authority.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"scrum_flow_events", "INSERT INTO scrum_flow", "COUNT(*) FILTER"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s retains flow-ledger authority %q", path, forbidden)
			}
		}
	}
}

func TestScrumRuntimeHasNoLiveFlowLedgerOrDuplicatePauseRail(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob("scrum*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"scrum_flow_events", "PauseQueuedScrumCards"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s retains retired Scrum authority %q", path, forbidden)
			}
		}
	}
	if _, err := os.Stat("scrum_play_pause.go"); !os.IsNotExist(err) {
		t.Fatalf("retired duplicate pause rail still exists: %v", err)
	}
}

func TestScrumFlowMetricsRejectCounterAndScoreOverflow(t *testing.T) {
	t.Parallel()
	metrics := ScrumFlowMetrics{PlayRuns: math.MaxInt32}
	if err := applyScrumFlowMetricDelta(&metrics, ScrumFlowMetricDelta{Kind: ScrumFlowMetricPlayStarted}); err == nil || !strings.Contains(err.Error(), "cannot exceed") {
		t.Fatalf("transition counter overflow error=%v", err)
	}
	metrics = ScrumFlowMetrics{AssignedReturns: math.MaxInt32}
	if err := metrics.score(false); err == nil || !strings.Contains(err.Error(), "score exceeds") {
		t.Fatalf("incomplete score overflow error=%v", err)
	}
}

func TestScrumFlowMetricsUseSemanticSignalBoundsInsteadOfEncodedAggregateSize(t *testing.T) {
	t.Parallel()
	metrics := ScrumFlowMetrics{CompletionStatus: "uncertain", Signals: make([]string, maxScrumFlowSignals)}
	for index := range metrics.Signals {
		prefix := fmt.Sprintf("%02d:", index)
		metrics.Signals[index] = prefix + strings.Repeat("x", maxScrumFlowSignalBytes-len(prefix))
	}
	encoded, err := json.Marshal(metrics)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) <= 64<<10 {
		t.Fatalf("regression fixture encodes to %d bytes, expected more than the retired aggregate bound", len(encoded))
	}
	if err := validateScrumFlowMetrics(metrics); err != nil {
		t.Fatalf("semantic signal maximum rejected because of serialization size: %v", err)
	}
	if _, err := canonicalScrumFlowMetrics(encoded); err != nil {
		t.Fatalf("raw semantic signal maximum rejected because of serialization size: %v", err)
	}
	metrics.Signals = append(metrics.Signals, "one too many")
	if err := validateScrumFlowMetrics(metrics); err == nil || !strings.Contains(err.Error(), "64 items") {
		t.Fatalf("signal count overflow error=%v", err)
	}
	metrics.Signals = []string{strings.Repeat("x", maxScrumFlowSignalBytes+1)}
	if err := validateScrumFlowMetrics(metrics); err == nil || !strings.Contains(err.Error(), "text domain") {
		t.Fatalf("signal item byte overflow error=%v", err)
	}
}
