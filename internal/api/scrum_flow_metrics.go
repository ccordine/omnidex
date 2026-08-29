package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	maxScrumFlowTransitionCount = 2_147_483_647
	maxScrumFlowWideCount       = 9_007_199_254_740_991
	maxScrumFlowSignals         = 64
	maxScrumFlowSignalBytes     = 1024
	maxScrumFlowSignalsJSON     = 64 * 1024
	maxScrumFlowMetricsBytes    = 64 * 1024
)

type ScrumFlowMetrics struct {
	AssignedReturns   int      `json:"assigned_returns"`
	ReviewBounces     int      `json:"review_bounces"`
	RegressionCount   int      `json:"regression_count"`
	PlayRuns          int      `json:"play_runs"`
	ChannelMessages   int64    `json:"channel_messages"`
	ConversationChars int64    `json:"conversation_chars"`
	IncompleteScore   int      `json:"incomplete_score"`
	CompletionStatus  string   `json:"completion_status"`
	Signals           []string `json:"signals"`
	LastPlayOutcome   string   `json:"last_play_outcome,omitempty"`
	ReviewGate        string   `json:"review_gate,omitempty"`
	Column            string   `json:"column,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
}

type ScrumFlowProjectSummary struct {
	TotalCards           int `json:"total_cards"`
	LikelyIncomplete     int `json:"likely_incomplete"`
	Uncertain            int `json:"uncertain"`
	LikelyComplete       int `json:"likely_complete"`
	AssignedReturnsTotal int `json:"assigned_returns_total"`
	LongConversations    int `json:"long_conversations"`
}

func summarizeScrumFlowMetrics(cards []ScrumCard) (ScrumFlowProjectSummary, error) {
	summary := ScrumFlowProjectSummary{TotalCards: len(cards)}
	for _, card := range cards {
		metrics, err := parseScrumFlowMetrics(card.FlowMetrics)
		if err != nil {
			return ScrumFlowProjectSummary{}, fmt.Errorf("card %q flow metrics: %w", card.ID, err)
		}
		summary.AssignedReturnsTotal += metrics.AssignedReturns
		if metrics.ChannelMessages >= 30 {
			summary.LongConversations++
		}
		switch metrics.CompletionStatus {
		case "likely_incomplete":
			summary.LikelyIncomplete++
		case "likely_complete":
			summary.LikelyComplete++
		default:
			summary.Uncertain++
		}
	}
	return summary, nil
}

func parseScrumFlowMetrics(raw json.RawMessage) (ScrumFlowMetrics, error) {
	out := ScrumFlowMetrics{Signals: []string{}}
	if len(raw) == 0 {
		return ScrumFlowMetrics{}, fmt.Errorf("payload is required; use an explicit empty object when no metrics exist")
	}
	if len(raw) > maxScrumFlowMetricsBytes {
		return ScrumFlowMetrics{}, fmt.Errorf("payload exceeds %d bytes", maxScrumFlowMetricsBytes)
	}
	if !utf8.Valid(raw) {
		return ScrumFlowMetrics{}, fmt.Errorf("payload must be valid UTF-8")
	}
	if err := exactjson.ValidateObject(raw, &out, "scrum flow metrics"); err != nil {
		return ScrumFlowMetrics{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ScrumFlowMetrics{}, fmt.Errorf("decode field inventory: %w", err)
	}
	if len(fields) == 0 {
		return out, nil
	}
	for _, field := range []string{
		"assigned_returns", "review_bounces", "regression_count", "play_runs", "channel_messages",
		"conversation_chars", "incomplete_score", "completion_status", "signals",
	} {
		if _, ok := fields[field]; !ok {
			return ScrumFlowMetrics{}, fmt.Errorf("missing required field %q", field)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return ScrumFlowMetrics{}, fmt.Errorf("decode payload: %w", err)
	}
	if decoder.More() {
		return ScrumFlowMetrics{}, fmt.Errorf("payload contains trailing data")
	}
	if out.Signals == nil {
		out.Signals = []string{}
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "last_play_outcome", value: out.LastPlayOutcome},
		{name: "review_gate", value: out.ReviewGate},
		{name: "column", value: out.Column},
		{name: "updated_at", value: out.UpdatedAt},
	} {
		if _, present := fields[field.name]; present && field.value == "" {
			return ScrumFlowMetrics{}, fmt.Errorf("%s must be omitted when empty", field.name)
		}
	}
	if err := validateScrumFlowMetrics(out); err != nil {
		return ScrumFlowMetrics{}, err
	}
	return out, nil
}

func validateScrumFlowMetrics(metrics ScrumFlowMetrics) error {
	transitionCounts := map[string]int{
		"assigned_returns": metrics.AssignedReturns, "review_bounces": metrics.ReviewBounces,
		"regression_count": metrics.RegressionCount, "play_runs": metrics.PlayRuns,
		"incomplete_score": metrics.IncompleteScore,
	}
	for field, value := range transitionCounts {
		if value < 0 || int64(value) > maxScrumFlowTransitionCount {
			return fmt.Errorf("%s is outside its registered domain", field)
		}
	}
	for field, value := range map[string]int64{"channel_messages": metrics.ChannelMessages, "conversation_chars": metrics.ConversationChars} {
		if value < 0 || value > maxScrumFlowWideCount {
			return fmt.Errorf("%s is outside its registered domain", field)
		}
	}
	if metrics.CompletionStatus != "likely_complete" && metrics.CompletionStatus != "likely_incomplete" && metrics.CompletionStatus != "uncertain" {
		return fmt.Errorf("completion_status is not registered")
	}
	if len(metrics.Signals) > maxScrumFlowSignals {
		return fmt.Errorf("signals exceeds %d items", maxScrumFlowSignals)
	}
	seenSignals := make(map[string]struct{}, len(metrics.Signals))
	for index, signal := range metrics.Signals {
		if !utf8.ValidString(signal) || strings.ContainsRune(signal, '\x00') || strings.TrimSpace(signal) == "" || strings.TrimSpace(signal) != signal || len(signal) > maxScrumFlowSignalBytes {
			return fmt.Errorf("signals[%d] is outside its registered domain", index)
		}
		if _, exists := seenSignals[signal]; exists {
			return fmt.Errorf("signals contains duplicate value %q", signal)
		}
		seenSignals[signal] = struct{}{}
	}
	encodedSignals, err := json.Marshal(metrics.Signals)
	if err != nil {
		return fmt.Errorf("encode signals: %w", err)
	}
	if len(encodedSignals) > maxScrumFlowSignalsJSON {
		return fmt.Errorf("signals exceeds its %d-byte aggregate bound", maxScrumFlowSignalsJSON)
	}
	if metrics.LastPlayOutcome != "" && metrics.LastPlayOutcome != "success" && metrics.LastPlayOutcome != "failed" {
		return fmt.Errorf("last_play_outcome is not registered")
	}
	if metrics.ReviewGate != "" && metrics.ReviewGate != "passed" && metrics.ReviewGate != "failed" && metrics.ReviewGate != "pending" && metrics.ReviewGate != "running" {
		return fmt.Errorf("review_gate is not registered")
	}
	if metrics.Column != "" && normalizeScrumColumn(metrics.Column) != metrics.Column {
		return fmt.Errorf("column is not registered")
	}
	if metrics.UpdatedAt != "" {
		observed, err := time.Parse(time.RFC3339Nano, metrics.UpdatedAt)
		if err != nil || observed.Location() != time.UTC ||
			!observed.Equal(observed.Truncate(time.Microsecond)) ||
			observed.Format(time.RFC3339Nano) != metrics.UpdatedAt {
			return fmt.Errorf("updated_at is not canonical UTC microsecond time")
		}
	}
	return nil
}
