package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type ScrumFlowMetricDeltaKind string

const (
	ScrumFlowMetricColumnMove  ScrumFlowMetricDeltaKind = "column_move"
	ScrumFlowMetricPlayStarted ScrumFlowMetricDeltaKind = "play_started"
	ScrumFlowMetricPlayFinish  ScrumFlowMetricDeltaKind = "play_finished"
)

// ScrumFlowMetricDelta is an in-memory input to the card-held metrics
// projection. It is never persisted as a second transition ledger.
type ScrumFlowMetricDelta struct {
	Kind       ScrumFlowMetricDeltaKind
	FromColumn string
	ToColumn   string
	Outcome    string
}

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
	Column            string   `json:"column,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
}

func applyScrumFlowMetricDeltasTx(
	ctx context.Context,
	tx pgx.Tx,
	projectID int64,
	cardID string,
	deltas ...ScrumFlowMetricDelta,
) error {
	if tx == nil || projectID <= 0 || cardID == "" {
		return fmt.Errorf("Scrum flow metrics require a transaction, project, and card")
	}
	return updateScrumFlowMetricsTx(ctx, tx, projectID, cardID, deltas...)
}

func refreshScrumFlowMetricsTx(ctx context.Context, tx pgx.Tx, projectID int64, cardID string) error {
	return updateScrumFlowMetricsTx(ctx, tx, projectID, cardID)
}

func applyScrumCardStateMetricsTx(
	ctx context.Context,
	tx pgx.Tx,
	previous DBScrumCard,
	next DBScrumCard,
	outcome string,
) error {
	if previous.ProjectID != next.ProjectID || previous.ID != next.ID {
		return fmt.Errorf("Scrum flow transition changed card identity")
	}
	deltas := make([]ScrumFlowMetricDelta, 0, 2)
	if previous.Column != next.Column {
		deltas = append(deltas, ScrumFlowMetricDelta{
			Kind: ScrumFlowMetricColumnMove, FromColumn: previous.Column, ToColumn: next.Column,
		})
	}
	if previous.PlayState != next.PlayState {
		switch {
		case next.PlayState == "running":
			deltas = append(deltas, ScrumFlowMetricDelta{Kind: ScrumFlowMetricPlayStarted})
		case next.PlayState == "paused", next.PlayState == "queued":
			// These states change the card-held completion projection but do not
			// advance a transition counter.
		case previous.PlayState == "running" && next.PlayState == "" && outcome != "":
			deltas = append(deltas, ScrumFlowMetricDelta{Kind: ScrumFlowMetricPlayFinish, Outcome: outcome})
		default:
			return fmt.Errorf("Scrum flow transition %q to %q is not registered", previous.PlayState, next.PlayState)
		}
	}
	return applyScrumFlowMetricDeltasTx(ctx, tx, next.ProjectID, next.ID, deltas...)
}
