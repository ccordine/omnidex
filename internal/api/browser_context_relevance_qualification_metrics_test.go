package api

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/station"
)

type browserContextQualificationReport struct {
	Schema                string                                  `json:"schema"`
	CreatedAt             time.Time                               `json:"created_at"`
	Station               station.ID                              `json:"station"`
	Provider              string                                  `json:"provider"`
	Model                 string                                  `json:"model"`
	Browser               string                                  `json:"browser"`
	CorpusVersion         string                                  `json:"corpus_version"`
	CorpusSHA256          string                                  `json:"corpus_sha256"`
	MedianLatencyTargetMS int64                                   `json:"median_latency_target_ms"`
	Passed                bool                                    `json:"passed"`
	MeasuredLatencyMS     browserContextQualificationLatency      `json:"measured_latency_ms"`
	MeasuredQuality       browserContextQualificationQuality      `json:"measured_quality"`
	Qualified             bool                                    `json:"qualified"`
	Cases                 []browserContextQualificationCaseResult `json:"cases"`
}

type browserContextQualificationLatency struct {
	Median  int64 `json:"median"`
	Maximum int64 `json:"maximum"`
	Total   int64 `json:"total"`
}

type browserContextQualificationQuality struct {
	CaseCount      int     `json:"case_count"`
	PassedCases    int     `json:"passed_cases"`
	ExactMatchRate float64 `json:"exact_match_rate"`
	MicroPrecision float64 `json:"micro_precision"`
	MicroRecall    float64 `json:"micro_recall"`
}

type browserContextQualificationCaseResult struct {
	Name                 string   `json:"name"`
	ExpectedCandidateIDs []string `json:"expected_candidate_ids"`
	ActualCandidateIDs   []string `json:"actual_candidate_ids"`
	LatencyMS            int64    `json:"latency_ms"`
	Passed               bool     `json:"passed"`
	Error                string   `json:"error,omitempty"`
}

func TestBrowserContextQualificationMetricsAreExactAndOrderIndependent(t *testing.T) {
	if !sameOpaqueIDSet([]string{"CTX_1", "CTX_2"}, []string{"CTX_2", "CTX_1"}) {
		t.Fatal("the same opaque ID set was not an exact match")
	}
	if sameOpaqueIDSet([]string{"CTX_1"}, []string{"CTX_2"}) {
		t.Fatal("different opaque ID sets were accepted")
	}
	tp, fp, fn := selectionCounts([]string{"CTX_1", "CTX_2"}, []string{"CTX_2", "CTX_3"})
	if tp != 1 || fp != 1 || fn != 1 {
		t.Fatalf("selection counts=%d/%d/%d", tp, fp, fn)
	}
	latency := summarizeQualificationLatencies([]int64{9, 3, 5})
	if latency.Median != 5 || latency.Maximum != 9 || latency.Total != 17 {
		t.Fatalf("latency=%#v", latency)
	}
}

func qualificationCorpusSHA256() string {
	digest := sha256.Sum256(browserContextQualificationCorpusJSON)
	return hex.EncodeToString(digest[:])
}

func sameOpaqueIDSet(expected, actual []string) bool {
	if len(expected) != len(actual) {
		return false
	}
	tp, fp, fn := selectionCounts(expected, actual)
	return tp == len(expected) && fp == 0 && fn == 0
}

func selectionCounts(expected, actual []string) (int, int, int) {
	want := make(map[string]struct{}, len(expected))
	for _, id := range expected {
		want[id] = struct{}{}
	}
	tp, fp := 0, 0
	for _, id := range actual {
		if _, exists := want[id]; exists {
			tp++
		} else {
			fp++
		}
	}
	return tp, fp, len(expected) - tp
}

func summarizeQualificationLatencies(values []int64) browserContextQualificationLatency {
	if len(values) == 0 {
		return browserContextQualificationLatency{}
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	summary := browserContextQualificationLatency{
		Median: ordered[len(ordered)/2], Maximum: ordered[len(ordered)-1],
	}
	for _, value := range values {
		summary.Total += value
	}
	return summary
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 1
	}
	return float64(numerator) / float64(denominator)
}
