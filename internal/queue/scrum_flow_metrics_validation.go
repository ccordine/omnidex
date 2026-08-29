package queue

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxScrumFlowWideCounter = int64(9007199254740991)
	maxScrumFlowSignals     = 64
	maxScrumFlowSignalBytes = 1024
)

func validateScrumFlowMetrics(metrics ScrumFlowMetrics) error {
	for name, value := range map[string]int{
		"assigned_returns": metrics.AssignedReturns,
		"review_bounces":   metrics.ReviewBounces,
		"regression_count": metrics.RegressionCount,
		"play_runs":        metrics.PlayRuns,
		"incomplete_score": metrics.IncompleteScore,
	} {
		if value < 0 || int64(value) > math.MaxInt32 {
			return fmt.Errorf("Scrum flow metric %s is outside its registered integer domain", name)
		}
	}
	for name, value := range map[string]int64{
		"channel_messages":   metrics.ChannelMessages,
		"conversation_chars": metrics.ConversationChars,
	} {
		if value < 0 || value > maxScrumFlowWideCounter {
			return fmt.Errorf("Scrum flow metric %s is outside exact transport authority", name)
		}
	}
	switch metrics.CompletionStatus {
	case "uncertain", "likely_incomplete", "likely_complete":
	default:
		return fmt.Errorf("Scrum flow completion status %q is not registered", metrics.CompletionStatus)
	}
	if len(metrics.Signals) > maxScrumFlowSignals {
		return fmt.Errorf("Scrum flow signals exceed %d items", maxScrumFlowSignals)
	}
	seen := make(map[string]struct{}, len(metrics.Signals))
	for index, signal := range metrics.Signals {
		if !utf8.ValidString(signal) || strings.ContainsRune(signal, '\x00') ||
			strings.TrimSpace(signal) != signal || signal == "" || len(signal) > maxScrumFlowSignalBytes {
			return fmt.Errorf("Scrum flow signal %d is outside its registered text domain", index+1)
		}
		if _, exists := seen[signal]; exists {
			return fmt.Errorf("Scrum flow signals contain duplicate %q", signal)
		}
		seen[signal] = struct{}{}
	}
	if metrics.LastPlayOutcome != "" && metrics.LastPlayOutcome != "success" && metrics.LastPlayOutcome != "failed" {
		return fmt.Errorf("Scrum flow outcome %q is not registered", metrics.LastPlayOutcome)
	}
	switch metrics.ReviewGate {
	case "", "passed", "failed", "pending", "running":
	default:
		return fmt.Errorf("Scrum flow review gate %q is not registered", metrics.ReviewGate)
	}
	if metrics.Column != "" {
		if _, err := ParseScrumCardColumn(metrics.Column); err != nil {
			return err
		}
	}
	if metrics.UpdatedAt != "" {
		observed, err := time.Parse(time.RFC3339Nano, metrics.UpdatedAt)
		if err != nil || observed.Location() != time.UTC ||
			!observed.Equal(observed.Truncate(time.Microsecond)) ||
			observed.Format(time.RFC3339Nano) != metrics.UpdatedAt {
			return fmt.Errorf("Scrum flow updated_at is not canonical UTC microsecond time")
		}
	}
	return nil
}
