package api

import (
	"context"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

const scrumPilotContextShrinkSource = "scrum_pilot"

type scrumPilotContextShrinkReport struct {
	RawChars       int
	ShrunkChars    int
	ChatMessages   int
	SelectedChunks int
	ToolMessages   int
	ThinkingMessages int
}

func measureScrumPilotRawContext(board ScrumBoard, card ScrumCard, userMessage string) scrumPilotContextShrinkReport {
	report := scrumPilotContextShrinkReport{
		ChatMessages: len(card.Chat) + 1,
	}
	total := len(strings.TrimSpace(userMessage))
	total += len(strings.TrimSpace(card.Title))
	total += len(strings.TrimSpace(card.Column))
	total += len(strings.TrimSpace(board.ProjectDirectory))
	total += len(strings.TrimSpace(card.Description))
	for _, ref := range card.RefFiles {
		total += len(strings.TrimSpace(ref)) + 2
	}
	for _, item := range card.Checklist {
		total += len(strings.TrimSpace(item.Text)) + 8
	}
	for _, msg := range card.Chat {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		role := normalizeScrumChannelRole(msg.Role)
		switch role {
		case "tool":
			report.ToolMessages++
		case "thinking":
			report.ThinkingMessages++
		}
		total += len(role) + len(content) + 4
	}
	report.RawChars = total
	return report
}

func (s *Server) recordScrumPilotContextShrink(
	ctx context.Context,
	projectID int64,
	card ScrumCard,
	board ScrumBoard,
	userMessage string,
	pilotContext scrumPilotPromptContext,
	shrunkPrompt string,
) {
	if s.repo == nil {
		return
	}
	raw := measureScrumPilotRawContext(board, card, userMessage)
	timeline := buildPilotChannelTimeline(card.Chat)
	_ = s.repo.RecordContextShrinkMetric(ctx, queue.ContextShrinkMetricRecord{
		Source:         scrumPilotContextShrinkSource,
		CardID:         card.ID,
		ProjectID:      projectID,
		RawChars:       raw.RawChars,
		ShrunkChars:    len(strings.TrimSpace(shrunkPrompt)),
		ChatMessages:   raw.ChatMessages,
		SelectedChunks: pilotContext.SelectedChunks,
		Metadata: map[string]any{
			"tool_messages":     raw.ToolMessages,
			"thinking_messages": raw.ThinkingMessages,
			"timeline_chunks":   len(timeline),
			"memory_lines":      len(pilotContext.MemoryLines),
			"channel_facts":     len(pilotContext.ChannelFacts),
			"card_title":        strings.TrimSpace(card.Title),
		},
	})
}
