package api

import (
	"context"
	"log"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

const scrumPilotContextShrinkSource = "scrum_pilot"

type scrumPilotContextShrinkReport struct {
	RawChars         int
	ShrunkChars      int
	ChatMessages     int
	SelectedChunks   int
	ToolMessages     int
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
	shrunkChars := len(strings.TrimSpace(shrunkPrompt))
	timeline := buildPilotChannelTimeline(card.Chat)
	if err := s.repo.RecordContextShrinkMetric(ctx, queue.ContextShrinkMetricRecord{
		Source:         scrumPilotContextShrinkSource,
		CardID:         card.ID,
		ProjectID:      projectID,
		RawChars:       raw.RawChars,
		ShrunkChars:    shrunkChars,
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
	}); err != nil {
		log.Printf(
			"Scrum pilot context telemetry persistence failed project=%d card=%q raw_chars=%d shrunk_chars=%d: %v",
			projectID,
			card.ID,
			raw.RawChars,
			shrunkChars,
			err,
		)
	}
}

func contextShrinkSavedPct(raw, shrunk int) float64 {
	if raw <= 0 {
		return 0
	}
	saved := float64(raw-shrunk) / float64(raw) * 100
	if saved < 0 {
		return 0
	}
	if saved > 100 {
		return 100
	}
	return saved
}
