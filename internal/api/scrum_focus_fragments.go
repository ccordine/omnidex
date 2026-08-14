package api

import (
	"fmt"
	"html"
	"strings"
)

type scrumPlayQueueView struct {
	RunningCardID string
	QueuedCount   int
	QueuedCardIDs []string
	QueuedHasMore bool
}

func renderScrumFocusBarHTML(board ScrumBoard, cardsByCol map[string][]ScrumCard, playQueue scrumPlayQueueView, autoWorkEnabled bool, autoWork ScrumAutoWorkConfig, autoWorkComplete bool) string {
	autoStatus := renderScrumAutoWorkStatusHTML(autoWorkEnabled, autoWorkComplete)
	focus := pickScrumFocusCardForHTML(board, cardsByCol, playQueue, autoWorkEnabled, autoWork)
	if focus == nil {
		message := "Nothing in Assigned or In Progress"
		if autoWorkEnabled {
			message = "Auto-play complete - every card is in Review"
		}
		return fmt.Sprintf(`
      <div class="flex items-center justify-center gap-3 rounded-xl border border-dashed border-white/10 bg-zinc-950/40 px-4 py-2.5 text-center">
        %s
        <span class="text-xs text-zinc-500">%s</span>
      </div>
    `, autoStatus, html.EscapeString(message))
	}
	isRunning := focus.PlayState == "running"
	isQueued := focus.PlayState == "queued"
	hasActiveRunner := playQueue.RunningCardID != ""
	playLabel := "Play"
	if hasActiveRunner && !isRunning {
		playLabel = "Queue"
	}
	playButton := ""
	if !isRunning && !isQueued {
		playButton = fmt.Sprintf(`<button type="button" data-action="scrum#play" data-card-id="%s" class="rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 transition hover:bg-cyan-200" title="Play this card">&#9654; %s</button>`, html.EscapeString(focus.ID), html.EscapeString(playLabel))
	}
	pauseButton := ""
	if isRunning {
		pauseButton = fmt.Sprintf(`<button type="button" data-action="scrum#pausePlay" data-card-id="%s" class="rounded-md border border-amber-300/40 bg-amber-300/10 px-3 py-1.5 text-xs font-semibold text-amber-100 transition hover:bg-amber-300/20" title="Pause play">Pause</button>`, html.EscapeString(focus.ID))
	}
	pivotButton := ""
	if hasActiveRunner && !isRunning && !isQueued {
		pivotButton = fmt.Sprintf(`<button type="button" data-action="scrum#pivotPlay" data-card-id="%s" class="rounded-md border border-violet-300/30 bg-violet-300/10 px-3 py-1.5 text-xs font-semibold text-violet-100 transition hover:bg-violet-300/20" title="Play this card now">Play now</button>`, html.EscapeString(focus.ID))
	}
	nowPlayingLabel := "Now playing"
	if autoWorkEnabled && !isRunning && !isQueued {
		nowPlayingLabel = "Up next"
	}
	return fmt.Sprintf(`
    <div class="flex max-w-2xl items-center gap-3 rounded-xl border border-white/10 bg-zinc-950/70 px-4 py-2.5 shadow-[0_10px_30px_rgba(0,0,0,.18)]">
      %s
      <div class="min-w-0 flex-1">
        <p class="text-[10px] font-semibold uppercase tracking-[.18em] text-zinc-500">%s</p>
        <button type="button" data-action="scrum#openCard" data-card-id="%s" class="mt-0.5 block max-w-full truncate text-left text-sm font-semibold text-zinc-100 transition hover:text-cyan-200" title="%s">%s</button>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        %s
        %s
        %s
        %s
      </div>
    </div>
  `, autoStatus, html.EscapeString(nowPlayingLabel), html.EscapeString(focus.ID), html.EscapeString(focus.Title), html.EscapeString(focus.Title), renderScrumFocusStateBadgeHTML(*focus), playButton, pivotButton, pauseButton)
}

func renderScrumPlayStateBadgeHTML(card ScrumCard) string {
	switch card.PlayState {
	case "running":
		return `<span class="rounded-full border border-amber-300/40 bg-amber-300/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-200">Running</span>`
	case "queued":
		suffix := ""
		if card.QueueOrder > 0 {
			suffix = fmt.Sprintf(" #%d", card.QueueOrder)
		}
		return fmt.Sprintf(`<span class="rounded-full border border-violet-300/40 bg-violet-300/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-violet-200">Queued%s</span>`, suffix)
	case "paused":
		return `<span class="rounded-full border border-zinc-400/40 bg-zinc-400/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-zinc-300">Paused</span>`
	default:
		return ""
	}
}

func renderScrumFocusStateBadgeHTML(card ScrumCard) string {
	if card.PlayState == "running" || card.PlayState == "queued" {
		return renderScrumPlayStateBadgeHTML(card)
	}
	return fmt.Sprintf(`<span class="rounded-full border border-white/10 bg-zinc-900/80 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">%s</span>`, html.EscapeString(scrumColumnLabel(card.Column)))
}

func renderScrumAutoWorkStatusHTML(enabled bool, complete bool) string {
	if !enabled {
		return ""
	}
	completeNote := ""
	if complete {
		completeNote = `<span class="rounded-full border border-emerald-300/35 bg-emerald-300/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-emerald-200">All in review</span>`
	}
	return fmt.Sprintf(`
    <div class="flex flex-wrap items-center gap-2">
      <span class="rounded-full border border-cyan-300/35 bg-cyan-300/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-cyan-100">
        Auto-work on
      </span>
      %s
    </div>
  `, completeNote)
}

func pickScrumFocusCardForHTML(board ScrumBoard, cardsByCol map[string][]ScrumCard, playQueue scrumPlayQueueView, autoWorkEnabled bool, autoWork ScrumAutoWorkConfig) *ScrumCard {
	if playQueue.RunningCardID != "" {
		if card := findScrumCard(board, playQueue.RunningCardID); card != nil {
			return card
		}
	}
	if cards := cardsByCol["in_progress"]; len(cards) > 0 {
		for i := range cards {
			if cards[i].PlayState == "running" {
				return &cards[i]
			}
		}
		if !autoWorkEnabled {
			return &cards[0]
		}
	}
	if !autoWorkEnabled {
		if cards := cardsByCol["assigned"]; len(cards) > 0 {
			return &cards[0]
		}
		return nil
	}
	for _, raw := range autoWork.SourceColumns {
		column := normalizeScrumColumn(raw)
		for i := range cardsByCol[column] {
			card := cardsByCol[column][i]
			if card.PlayState != "running" && card.PlayState != "queued" {
				return &card
			}
		}
	}
	if cards := cardsByCol["in_progress"]; len(cards) > 0 {
		return &cards[0]
	}
	if cards := cardsByCol["assigned"]; len(cards) > 0 {
		return &cards[0]
	}
	return nil
}

func decodeScrumPlayQueueView(playQueue map[string]any) (scrumPlayQueueView, error) {
	if playQueue == nil {
		return scrumPlayQueueView{}, fmt.Errorf("Scrum fragment play queue is required")
	}
	if len(playQueue) != 4 {
		return scrumPlayQueueView{}, fmt.Errorf("Scrum fragment play queue must contain exactly four typed fields")
	}
	runningCardID, ok := playQueue["running_card_id"].(string)
	if !ok || runningCardID != strings.TrimSpace(runningCardID) {
		return scrumPlayQueueView{}, fmt.Errorf("Scrum fragment play queue running_card_id must be exact text")
	}
	queuedCount, ok := playQueue["queued_count"].(int)
	if !ok || queuedCount < 0 {
		return scrumPlayQueueView{}, fmt.Errorf("Scrum fragment play queue queued_count must be a non-negative integer")
	}
	queuedCardIDs, ok := playQueue["queued_card_ids"].([]string)
	if !ok || len(queuedCardIDs) > scrumQueuedSummaryLimit {
		return scrumPlayQueueView{}, fmt.Errorf("Scrum fragment play queue queued_card_ids is not a bounded string list")
	}
	seen := make(map[string]struct{}, len(queuedCardIDs))
	for _, cardID := range queuedCardIDs {
		if cardID == "" || cardID != strings.TrimSpace(cardID) {
			return scrumPlayQueueView{}, fmt.Errorf("Scrum fragment play queue contains a noncanonical card ID")
		}
		if _, duplicate := seen[cardID]; duplicate {
			return scrumPlayQueueView{}, fmt.Errorf("Scrum fragment play queue contains duplicate card ID %q", cardID)
		}
		seen[cardID] = struct{}{}
	}
	queuedHasMore, ok := playQueue["queued_has_more"].(bool)
	if !ok || queuedCount < len(queuedCardIDs) || queuedHasMore != (queuedCount > len(queuedCardIDs)) {
		return scrumPlayQueueView{}, fmt.Errorf("Scrum fragment play queue has contradictory count authority")
	}
	return scrumPlayQueueView{
		RunningCardID: runningCardID, QueuedCount: queuedCount,
		QueuedCardIDs: append([]string(nil), queuedCardIDs...), QueuedHasMore: queuedHasMore,
	}, nil
}

func scrumColumnLabel(column string) string {
	switch normalizeScrumColumn(column) {
	case "backlog":
		return "Backlog"
	case "ready":
		return "Ready"
	case "assigned":
		return "Assigned"
	case "in_progress":
		return "In Progress"
	case "review":
		return "Review"
	case "blocked":
		return "Blocked"
	case "error":
		return "Error"
	case "done":
		return "Done"
	default:
		return column
	}
}

func scrumColumnAccent(column string) string {
	switch normalizeScrumColumn(column) {
	case "backlog":
		return "border-zinc-500/40 bg-zinc-900/50"
	case "ready":
		return "border-sky-400/35 bg-sky-950/30"
	case "assigned":
		return "border-violet-400/35 bg-violet-950/25"
	case "in_progress":
		return "border-amber-400/35 bg-amber-950/25"
	case "review":
		return "border-cyan-400/35 bg-cyan-950/25"
	case "blocked":
		return "border-rose-400/35 bg-rose-950/25"
	case "error":
		return "border-red-400/40 bg-red-950/30"
	case "done":
		return "border-emerald-400/35 bg-emerald-950/25"
	default:
		return "border-white/10 bg-zinc-900/40"
	}
}

func trimScrumText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
