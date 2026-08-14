package api

import (
	"context"
	"fmt"
)

func (s *Server) scrumFlowSummaryFromRepository(ctx context.Context, projectID int64) (ScrumFlowProjectSummary, error) {
	if s == nil || s.repo == nil || ctx == nil || projectID <= 0 {
		return ScrumFlowProjectSummary{}, fmt.Errorf("repository, context, and project are required for Scrum flow summary")
	}
	stored, err := s.repo.ScrumFlowSummary(ctx, projectID)
	if err != nil {
		return ScrumFlowProjectSummary{}, err
	}
	return ScrumFlowProjectSummary{
		TotalCards:           stored.TotalCards,
		LikelyIncomplete:     stored.LikelyIncomplete,
		LikelyComplete:       stored.LikelyComplete,
		Uncertain:            stored.Uncertain,
		AssignedReturnsTotal: stored.AssignedReturnsTotal,
		LongConversations:    stored.LongConversations,
	}, nil
}
