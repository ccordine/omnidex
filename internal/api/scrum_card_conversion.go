package api

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func dbScrumCardToAPI(card queue.DBScrumCard) (ScrumCard, error) {
	out := ScrumCard{
		ID: card.ID, Title: card.Title, Description: card.Description, Column: card.Column,
		JobID: card.JobID, PlayState: card.PlayState, QueueOrder: card.QueueOrder,
		BoardOrder: card.BoardOrder, SyncJobID: card.SyncJobID, StepContextCursor: card.StepContextCursor,
		CardTicket: card.CardTicket, CardPrompt: card.CardPrompt,
		ChatCount: card.ChannelMessageCount, ChannelContentBytes: card.ChannelContentBytes,
		CreatedAt: card.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: card.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Checklist: []ScrumChecklistItem{}, RefFiles: []string{}, Chat: []ScrumChatMessage{},
		Tags: []string{}, TestCriteria: []ScrumChecklistItem{}, FlowMetrics: card.FlowMetrics,
	}
	fields := []struct {
		name   string
		raw    json.RawMessage
		target any
	}{
		{"checklist", card.Checklist, &out.Checklist},
		{"ref_files", card.RefFiles, &out.RefFiles},
		{"tags", card.Tags, &out.Tags},
		{"test_criteria", card.TestCriteria, &out.TestCriteria},
	}
	for _, field := range fields {
		if err := json.Unmarshal(field.raw, field.target); err != nil {
			return ScrumCard{}, fmt.Errorf("%s must contain valid typed JSON: %w", field.name, err)
		}
	}
	var flowMetrics map[string]any
	if err := json.Unmarshal(card.FlowMetrics, &flowMetrics); err != nil || flowMetrics == nil {
		if err == nil {
			err = fmt.Errorf("expected JSON object")
		}
		return ScrumCard{}, fmt.Errorf("flow_metrics must contain valid typed JSON: %w", err)
	}
	return out, nil
}
