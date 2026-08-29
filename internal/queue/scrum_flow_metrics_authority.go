package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

func updateScrumFlowMetricsTx(
	ctx context.Context,
	tx pgx.Tx,
	projectID int64,
	cardID string,
	deltas ...ScrumFlowMetricDelta,
) error {
	var checklist, storedMetrics json.RawMessage
	var channelMessageCount, channelContentBytes int64
	var column string
	var updatedAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT checklist,flow_metrics,channel_message_count,channel_content_bytes,
		       column_name,updated_at
		FROM scrum_cards WHERE project_id=$1 AND id=$2
	`, projectID, cardID).Scan(
		&checklist, &storedMetrics, &channelMessageCount, &channelContentBytes,
		&column, &updatedAt,
	); err != nil {
		return fmt.Errorf("load Scrum card fields for flow metrics: %w", err)
	}
	metrics := ScrumFlowMetrics{
		CompletionStatus: "uncertain",
		Signals:          []string{},
	}
	if len(storedMetrics) != 0 && string(storedMetrics) != "{}" {
		if err := exactjson.ValidateObject(storedMetrics, ScrumFlowMetrics{}, "Scrum flow metrics"); err != nil {
			return err
		}
		if err := json.Unmarshal(storedMetrics, &metrics); err != nil {
			return fmt.Errorf("decode Scrum flow metrics: %w", err)
		}
	}
	if metrics.Signals == nil {
		metrics.Signals = []string{}
	}
	if err := validateScrumFlowMetrics(metrics); err != nil {
		return fmt.Errorf("validate stored Scrum flow metrics: %w", err)
	}
	for _, delta := range deltas {
		if err := applyScrumFlowMetricDelta(&metrics, delta); err != nil {
			return err
		}
	}
	checklistIncomplete, err := scrumChecklistIncomplete(checklist)
	if err != nil {
		return err
	}
	metrics.ChannelMessages = channelMessageCount
	metrics.ConversationChars = channelContentBytes
	metrics.Column = column
	metrics.UpdatedAt = updatedAt.UTC().Format("2006-01-02T15:04:05.999999Z")
	metrics.IncompleteScore = 0
	metrics.CompletionStatus = "uncertain"
	metrics.Signals = []string{}
	if err := metrics.score(checklistIncomplete); err != nil {
		return err
	}
	raw, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("encode Scrum flow metrics: %w", err)
	}
	canonical, err := canonicalScrumFlowMetrics(raw)
	if err != nil {
		return fmt.Errorf("validate next Scrum flow metrics: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE scrum_cards SET flow_metrics=$3::jsonb WHERE project_id=$1 AND id=$2`,
		projectID, cardID, string(canonical))
	if err != nil {
		return fmt.Errorf("persist atomic Scrum flow metrics: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("persist atomic Scrum flow metrics: affected=%d, expected 1", tag.RowsAffected())
	}
	return nil
}

func applyScrumFlowMetricDelta(metrics *ScrumFlowMetrics, delta ScrumFlowMetricDelta) error {
	increment := func(name string, value *int) error {
		if *value >= math.MaxInt32 {
			return fmt.Errorf("Scrum flow metric %s cannot exceed %d", name, math.MaxInt32)
		}
		*value++
		return nil
	}
	switch delta.Kind {
	case ScrumFlowMetricColumnMove:
		if _, err := ParseScrumCardColumn(delta.FromColumn); err != nil {
			return err
		}
		if _, err := ParseScrumCardColumn(delta.ToColumn); err != nil {
			return err
		}
		if delta.FromColumn == delta.ToColumn || delta.Outcome != "" {
			return fmt.Errorf("Scrum column metric delta requires distinct columns and no outcome")
		}
		if delta.ToColumn == "assigned" {
			switch delta.FromColumn {
			case "in_progress", "review", "blocked", "done":
				if err := increment("assigned_returns", &metrics.AssignedReturns); err != nil {
					return err
				}
			}
		}
		if delta.FromColumn == "review" && delta.ToColumn == "in_progress" {
			if err := increment("review_bounces", &metrics.ReviewBounces); err != nil {
				return err
			}
		}
		if scrumFlowColumnRank(delta.FromColumn) > scrumFlowColumnRank(delta.ToColumn) {
			if err := increment("regression_count", &metrics.RegressionCount); err != nil {
				return err
			}
		}
	case ScrumFlowMetricPlayStarted:
		if delta.FromColumn != "" || delta.ToColumn != "" || delta.Outcome != "" {
			return fmt.Errorf("Scrum play-start metric delta contains unrelated state")
		}
		if err := increment("play_runs", &metrics.PlayRuns); err != nil {
			return err
		}
	case ScrumFlowMetricPlayFinish:
		if delta.FromColumn != "" || delta.ToColumn != "" ||
			(delta.Outcome != "success" && delta.Outcome != "failed") {
			return fmt.Errorf("Scrum play-finish metric delta requires one registered outcome")
		}
		metrics.LastPlayOutcome = delta.Outcome
	default:
		return fmt.Errorf("Scrum flow metric delta %q is not registered", delta.Kind)
	}
	return nil
}

func scrumFlowColumnRank(column string) int {
	switch column {
	case "backlog":
		return 0
	case "ready":
		return 1
	case "assigned":
		return 2
	case "in_progress":
		return 3
	case "review", "blocked":
		return 4
	case "done":
		return 5
	default:
		return -1
	}
}

func scrumChecklistIncomplete(raw json.RawMessage) (bool, error) {
	type checklistItem struct {
		ID   string `json:"id"`
		Text string `json:"text"`
		Done bool   `json:"done"`
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil || entries == nil {
		if err == nil {
			err = fmt.Errorf("expected JSON array")
		}
		return false, fmt.Errorf("decode Scrum checklist for flow metrics: %w", err)
	}
	for index, itemRaw := range entries {
		if err := exactjson.ValidateObject(itemRaw, checklistItem{}, fmt.Sprintf("Scrum checklist item %d", index+1)); err != nil {
			return false, err
		}
		var item checklistItem
		if err := json.Unmarshal(itemRaw, &item); err != nil {
			return false, fmt.Errorf("decode Scrum checklist item %d: %w", index+1, err)
		}
		if !item.Done {
			return true, nil
		}
	}
	return false, nil
}

func (metrics *ScrumFlowMetrics) score(checklistIncomplete bool) error {
	add := func(signal string, weight int64) error {
		if weight <= 0 {
			return nil
		}
		if weight > math.MaxInt32-int64(metrics.IncompleteScore) {
			return fmt.Errorf("Scrum flow incomplete score exceeds %d", math.MaxInt32)
		}
		if len(metrics.Signals) >= maxScrumFlowSignals || len(signal) > maxScrumFlowSignalBytes {
			return fmt.Errorf("Scrum flow derived signals exceed bounded authority")
		}
		metrics.IncompleteScore += int(weight)
		metrics.Signals = append(metrics.Signals, signal)
		return nil
	}
	if metrics.AssignedReturns > 0 {
		if err := add(fmt.Sprintf("returned to assigned %d time(s) after review or later", metrics.AssignedReturns), int64(metrics.AssignedReturns)*25); err != nil {
			return err
		}
	}
	if metrics.ReviewBounces > 0 {
		if err := add(fmt.Sprintf("bounced out of review %d time(s)", metrics.ReviewBounces), int64(metrics.ReviewBounces)*20); err != nil {
			return err
		}
	}
	if extra := metrics.RegressionCount - metrics.AssignedReturns - metrics.ReviewBounces; extra > 0 {
		if err := add(fmt.Sprintf("%d other column regression(s)", extra), int64(extra)*10); err != nil {
			return err
		}
	}
	if metrics.PlayRuns > 2 {
		if err := add(fmt.Sprintf("played %d times", metrics.PlayRuns), int64(metrics.PlayRuns-2)*10); err != nil {
			return err
		}
	}
	if metrics.ChannelMessages >= 30 && metrics.Column != "done" {
		if err := add(fmt.Sprintf("long conversation (%d messages) without reaching done", metrics.ChannelMessages), 15); err != nil {
			return err
		}
	}
	if metrics.ConversationChars >= 10000 && metrics.Column != "done" {
		if err := add(fmt.Sprintf("heavy discussion (~%dk chars) still open", metrics.ConversationChars/1000), 10); err != nil {
			return err
		}
	}
	if metrics.Column == "blocked" {
		if err := add("currently blocked", 20); err != nil {
			return err
		}
	}
	if checklistIncomplete && (metrics.Column == "review" || metrics.Column == "done") {
		if err := add("checklist still incomplete in review/done", 15); err != nil {
			return err
		}
	}
	if metrics.LastPlayOutcome == "failed" {
		if err := add("last play outcome: failed", 15); err != nil {
			return err
		}
	}
	switch {
	case metrics.IncompleteScore >= 50:
		metrics.CompletionStatus = "likely_incomplete"
	case metrics.IncompleteScore <= 15 && metrics.Column == "done" && metrics.AssignedReturns == 0:
		metrics.CompletionStatus = "likely_complete"
	default:
		metrics.CompletionStatus = "uncertain"
	}
	return nil
}
