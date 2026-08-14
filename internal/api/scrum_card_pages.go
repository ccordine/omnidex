package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

const scrumCardUIPageSize = 20

func (s *Server) scrumCardColumnPage(ctx context.Context, projectID int64, column string, offset int) ([]ScrumCard, bool, error) {
	column = normalizeScrumColumn(column)
	if column == "" {
		return nil, false, fmt.Errorf("Scrum card page requires a registered column")
	}
	page, err := s.repo.ListScrumCardPage(ctx, projectID, queue.ScrumCardPageRequest{
		Column: column, Limit: scrumCardUIPageSize, Offset: offset,
	})
	if err != nil {
		return nil, false, err
	}
	cards := make([]ScrumCard, 0, len(page.Items))
	for _, item := range page.Items {
		card, err := dbScrumCardSummaryToAPI(item)
		if err != nil {
			return nil, false, err
		}
		cards = append(cards, card)
	}
	return cards, page.HasMore, nil
}

func dbScrumCardSummaryToAPI(stored queue.DBScrumCardSummary) (ScrumCard, error) {
	tags := []string{}
	if err := json.Unmarshal(stored.Tags, &tags); err != nil {
		return ScrumCard{}, fmt.Errorf("decode Scrum card %q tags: %w", stored.ID, err)
	}
	if tags == nil {
		return ScrumCard{}, fmt.Errorf("decode Scrum card %q tags: expected JSON array", stored.ID)
	}
	var flowMetrics map[string]any
	if err := json.Unmarshal(stored.FlowMetrics, &flowMetrics); err != nil || flowMetrics == nil {
		if err == nil {
			err = fmt.Errorf("expected JSON object")
		}
		return ScrumCard{}, fmt.Errorf("decode Scrum card %q flow_metrics: %w", stored.ID, err)
	}
	return ScrumCard{
		ID: stored.ID, Title: stored.Title, Description: stored.Description, Column: stored.Column,
		Summary: true, ChecklistDone: stored.ChecklistDone, ChecklistTotal: stored.ChecklistTotal,
		RefFileCount: stored.RefFileCount, ChatCount: stored.ChatCount,
		TestCriteriaDone: stored.TestCriteriaDone,
		TestCriteriaTotal: stored.TestCriteriaTotal, HasCardTicket: stored.HasCardTicket,
		Tags: tags, FlowMetrics: stored.FlowMetrics, JobID: stored.JobID,
		PlayState: stored.PlayState, QueueOrder: stored.QueueOrder, BoardOrder: stored.BoardOrder,
		CreatedAt: stored.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: stored.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Checklist: []ScrumChecklistItem{}, RefFiles: []string{}, Chat: []ScrumChatMessage{},
		TestCriteria: []ScrumChecklistItem{},
	}, nil
}

func (s *Server) scrumColumnCountsFromRepository(ctx context.Context, projectID int64) (map[string]int, error) {
	stored, err := s.repo.CountScrumCardsByColumn(ctx, projectID)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(scrumColumns))
	for _, column := range scrumColumns {
		counts[column] = stored[column]
	}
	return counts, nil
}
