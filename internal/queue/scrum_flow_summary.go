package queue

import (
	"context"
	"fmt"
)

// ScrumFlowSummary is the exact database-owned aggregate for a growing board.
// It deliberately contains no card inventory.
type ScrumFlowSummary struct {
	TotalCards           int
	LikelyIncomplete     int
	LikelyComplete       int
	Uncertain            int
	AssignedReturnsTotal int
	LongConversations    int
}

func (r *Repository) ScrumFlowSummary(ctx context.Context, projectID int64) (ScrumFlowSummary, error) {
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 {
		return ScrumFlowSummary{}, fmt.Errorf("PostgreSQL, context, and project are required for Scrum flow summary")
	}
	var summary ScrumFlowSummary
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE COALESCE(flow_metrics->>'completion_status','uncertain')='likely_incomplete'),
		       COUNT(*) FILTER (WHERE COALESCE(flow_metrics->>'completion_status','uncertain')='likely_complete'),
		       COUNT(*) FILTER (WHERE COALESCE(flow_metrics->>'completion_status','uncertain') NOT IN ('likely_incomplete','likely_complete')),
		       COALESCE(SUM(CASE
		           WHEN COALESCE(flow_metrics->>'assigned_returns','') ~ '^[0-9]+$'
		           THEN (flow_metrics->>'assigned_returns')::integer ELSE 0 END), 0),
		       COUNT(*) FILTER (WHERE CASE
		           WHEN COALESCE(flow_metrics->>'channel_messages','') ~ '^[0-9]+$'
		           THEN (flow_metrics->>'channel_messages')::integer >= 30 ELSE false END)
		FROM scrum_cards
		WHERE project_id=$1
	`, projectID).Scan(
		&summary.TotalCards,
		&summary.LikelyIncomplete,
		&summary.LikelyComplete,
		&summary.Uncertain,
		&summary.AssignedReturnsTotal,
		&summary.LongConversations,
	)
	return summary, err
}
