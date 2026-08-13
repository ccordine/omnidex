package api

import (
	"fmt"
	"html"
	"strings"
)

type scrumBoardFragments struct {
	Board       string `json:"board"`
	Columns     string `json:"columns"`
	Focus       string `json:"focus"`
	FlowSummary string `json:"flow_summary"`
	Pagination  string `json:"pagination"`
	Bundle      string `json:"bundle"`
}

type scrumCardPageState struct {
	Offset  int
	Count   int
	HasMore bool
}

func renderScrumBoardFragments(board ScrumBoard, cardsByCol map[string][]ScrumCard, fullBoard ScrumBoard, visibleColumn string, columnCounts map[string]int, playQueue map[string]any, autoWorkEnabled bool, autoWork ScrumAutoWorkConfig, flowSummary ScrumFlowProjectSummary, page scrumCardPageState) scrumBoardFragments {
	columns := fullBoard.Columns
	if len(columns) == 0 {
		columns = append([]string(nil), scrumColumns...)
	}
	activeColumn := normalizeScrumColumn(visibleColumn)
	if activeColumn == "" {
		activeColumn = "assigned"
	}
	fragments := scrumBoardFragments{
		Board:       renderScrumBoardHTML(board, cardsByCol, playQueue),
		Columns:     renderScrumColumnNavHTML(columns, activeColumn, columnCounts),
		Focus:       renderScrumFocusBarHTML(fullBoard, cardsByColumn(fullBoard), playQueue, autoWorkEnabled, autoWork),
		FlowSummary: renderScrumFlowSummaryHTML(flowSummary),
		Pagination:  renderScrumCardPaginationHTML(page),
	}
	fragments.Bundle = renderScrumBoardBundleHTML(fragments)
	return fragments
}

func renderScrumBoardBundleHTML(fragments scrumBoardFragments) string {
	return renderRecyclrTemplateHTML("scrum-board", fragments.Board, "innerHTML") +
		renderRecyclrTemplateHTML("scrum-columns", fragments.Columns, "innerHTML") +
		renderRecyclrTemplateHTML("scrum-focus", fragments.Focus, "innerHTML") +
		renderRecyclrTemplateHTML("scrum-flow-summary", fragments.FlowSummary, "innerHTML") +
		renderRecyclrTemplateHTML("scrum-pagination", fragments.Pagination, "innerHTML")
}

func renderScrumCardPaginationHTML(page scrumCardPageState) string {
	if page.Offset == 0 && !page.HasMore {
		return ""
	}
	var body strings.Builder
	body.WriteString(`<nav class="flex items-center justify-end gap-2" aria-label="Scrum card pages">`)
	if page.Offset > 0 {
		previous := page.Offset - scrumCardUIPageSize
		if previous < 0 {
			previous = 0
		}
		body.WriteString(fmt.Sprintf(`<button type="button" data-action="scrum#loadCardPage" data-card-offset="%d" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-300">Previous</button>`, previous))
	}
	if page.HasMore {
		body.WriteString(fmt.Sprintf(`<button type="button" data-action="scrum#loadCardPage" data-card-offset="%d" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-300">Next</button>`, page.Offset+page.Count))
	}
	body.WriteString(`</nav>`)
	return body.String()
}

func renderRecyclrTemplateHTML(target string, inner string, location string) string {
	if strings.TrimSpace(target) == "" {
		panic("recyclr template target is required")
	}
	if !isSupportedRecyclrLocation(location) {
		panic(fmt.Sprintf("unsupported recyclr template location: %q", location))
	}
	return fmt.Sprintf(`<template data-recyclr-target="%s" data-recyclr-location="%s">%s</template>`, html.EscapeString(target), html.EscapeString(location), inner)
}

func isSupportedRecyclrLocation(location string) bool {
	switch location {
	case "innerHTML", "outerHTML", "beforebegin", "afterbegin", "beforeend", "afterend":
		return true
	default:
		return false
	}
}

func renderScrumBoardHTML(board ScrumBoard, cardsByCol map[string][]ScrumCard, playQueue map[string]any) string {
	columns := board.Columns
	if len(columns) == 0 {
		columns = append([]string(nil), scrumColumns...)
	}
	var b strings.Builder
	for _, column := range columns {
		b.WriteString(renderScrumColumnHTML(column, cardsByCol[normalizeScrumColumn(column)], playQueue))
	}
	return b.String()
}

func renderScrumColumnHTML(column string, cards []ScrumCard, playQueue map[string]any) string {
	column = normalizeScrumColumn(column)
	label := scrumColumnLabel(column)
	items := `<p class="scrum-column-empty rounded-md border border-dashed border-white/10 px-3 py-6 text-center text-xs text-zinc-500">Drop cards here</p>`
	if len(cards) > 0 {
		var b strings.Builder
		for _, card := range cards {
			b.WriteString(renderScrumCardHTML(card, playQueue))
		}
		items = b.String()
	}
	return fmt.Sprintf(`
    <div class="scrum-column min-w-[280px] rounded-xl border %s p-3" data-column="%s" data-scrum-dropzone="%s">
      <header class="mb-3 flex shrink-0 items-center justify-between gap-2">
        <div class="flex items-center gap-2 min-w-0">
          <h3 class="truncate text-xs font-semibold uppercase tracking-[.16em] text-zinc-200">%s</h3>
          <span class="rounded-full bg-black/30 px-2 py-0.5 font-mono text-[11px] text-zinc-400">%d</span>
        </div>
        <button type="button" data-action="click->scrum#stopCardClick scrum#openCreateCardModal" data-column="%s" class="shrink-0 rounded border border-white/10 px-2 py-0.5 text-[11px] text-zinc-400 transition hover:border-cyan-300/40 hover:text-cyan-200" title="Add card">+</button>
      </header>
      <div class="scrum-column-dropzone scrollbar space-y-3 pr-1">%s</div>
    </div>
  `, scrumColumnAccent(column), html.EscapeString(column), html.EscapeString(column), html.EscapeString(label), len(cards), html.EscapeString(column), items)
}

func renderScrumCardHTML(card ScrumCard, playQueue map[string]any) string {
	done, total := card.ChecklistDone, card.ChecklistTotal
	if total == 0 && len(card.Checklist) > 0 {
		total = len(card.Checklist)
		done = 0
		for _, item := range card.Checklist {
			if item.Done {
				done++
			}
		}
	}
	refCount := card.RefFileCount
	if refCount == 0 {
		refCount = len(card.RefFiles)
	}
	chatCount := card.ChatCount
	if chatCount == 0 {
		chatCount = len(card.Chat)
	}
	progress := ""
	if total > 0 {
		progress = fmt.Sprintf(`<span class="rounded border border-white/10 px-1.5 py-0.5">%d/%d</span>`, done, total)
	}
	refs := ""
	if refCount > 0 {
		refs = fmt.Sprintf(`<span class="rounded border border-white/10 px-1.5 py-0.5">%d refs</span>`, refCount)
	}
	chats := ""
	if chatCount > 0 {
		chats = fmt.Sprintf(`<span class="rounded border border-white/10 px-1.5 py-0.5">%d msgs</span>`, chatCount)
	}
	description := ""
	if strings.TrimSpace(card.Description) != "" {
		description = fmt.Sprintf(`<p class="mt-2 text-xs leading-relaxed text-zinc-400">%s</p>`, html.EscapeString(trimScrumText(card.Description, 140)))
	}
	job := ""
	if strings.TrimSpace(card.JobID) != "" {
		job = fmt.Sprintf(`<span class="rounded bg-cyan-300/15 px-1.5 py-0.5 font-mono text-[10px] text-cyan-200">#%s</span>`, html.EscapeString(card.JobID))
	}
	return fmt.Sprintf(`
    <article class="scrum-card scrum-card-draggable group cursor-grab rounded-lg border border-white/10 bg-zinc-950/70 p-3 shadow-[0_10px_30px_rgba(0,0,0,.22)] transition hover:border-cyan-300/30 active:cursor-grabbing" data-card-id="%s" data-scrum-column="%s" data-action="click->scrum#openCard">
      <div class="flex items-start justify-between gap-2">
        <h4 class="text-sm font-semibold leading-snug text-zinc-100">%s</h4>
        <div class="flex shrink-0 flex-col items-end gap-1">
          %s
          %s
          %s
        </div>
      </div>
      %s
      <div class="mt-3 flex flex-wrap gap-1.5 text-[10px] uppercase tracking-wide text-zinc-500">
        %s
        %s
        %s
      </div>
    </article>
  `, html.EscapeString(card.ID), html.EscapeString(card.Column), html.EscapeString(card.Title), renderScrumFlowBadgeHTML(card), renderScrumPlayStateBadgeHTML(card), job, description, progress, refs, chats)
}

func renderScrumColumnNavHTML(columns []string, activeColumn string, counts map[string]int) string {
	var b strings.Builder
	for _, raw := range columns {
		column := normalizeScrumColumn(raw)
		if column == "" {
			continue
		}
		classes := "border-white/10 text-zinc-400 hover:border-cyan-300/30 hover:text-zinc-100"
		pressed := "false"
		if column == activeColumn {
			classes = "border-cyan-300/40 bg-cyan-300/10 text-cyan-100"
			pressed = "true"
		}
		b.WriteString(fmt.Sprintf(`
      <button type="button" data-action="scrum#selectColumn" data-column="%s" class="inline-flex items-center gap-2 rounded-md border px-3 py-1.5 text-xs font-medium transition %s" aria-pressed="%s">
        <span>%s</span>
        <span class="rounded bg-black/30 px-1.5 py-0.5 font-mono text-[10px]">%d</span>
      </button>
    `, html.EscapeString(column), classes, pressed, html.EscapeString(scrumColumnLabel(column)), counts[column]))
	}
	return fmt.Sprintf(`<nav class="scrollbar flex max-w-full gap-2 overflow-x-auto" aria-label="Scrum columns">%s</nav>`, b.String())
}

func renderScrumFocusBarHTML(board ScrumBoard, cardsByCol map[string][]ScrumCard, playQueue map[string]any, autoWorkEnabled bool, autoWork ScrumAutoWorkConfig) string {
	autoStatus := renderScrumAutoWorkStatusHTML(autoWorkEnabled, scrumAutoWorkUIComplete(cardsByCol))
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
	hasActiveRunner := strings.TrimSpace(playQueueString(playQueue, "running_card_id")) != ""
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

func renderScrumFlowSummaryHTML(summary ScrumFlowProjectSummary) string {
	if summary.TotalCards == 0 {
		return ""
	}
	chips := []string{}
	if summary.LikelyIncomplete > 0 {
		chips = append(chips, fmt.Sprintf(`<span class="rounded border border-amber-300/30 bg-amber-300/10 px-2 py-1 text-[11px] text-amber-100">%d likely incomplete</span>`, summary.LikelyIncomplete))
	}
	if summary.LongConversations > 0 {
		chips = append(chips, fmt.Sprintf(`<span class="rounded border border-violet-300/25 bg-violet-300/10 px-2 py-1 text-[11px] text-violet-100">%d long conversations</span>`, summary.LongConversations))
	}
	if summary.AssignedReturnsTotal > 0 {
		chips = append(chips, fmt.Sprintf(`<span class="rounded border border-rose-300/25 bg-rose-300/10 px-2 py-1 text-[11px] text-rose-100">%d assigned returns</span>`, summary.AssignedReturnsTotal))
	}
	if summary.LikelyComplete > 0 {
		chips = append(chips, fmt.Sprintf(`<span class="rounded border border-emerald-300/25 bg-emerald-300/10 px-2 py-1 text-[11px] text-emerald-100">%d likely complete</span>`, summary.LikelyComplete))
	}
	if len(chips) == 0 {
		return ""
	}
	return `<div class="flex flex-wrap items-center gap-2">` + strings.Join(chips, "") + `</div>`
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

func renderScrumFlowBadgeHTML(card ScrumCard) string {
	metrics := parseScrumFlowMetrics(card.FlowMetrics)
	if metrics.CompletionStatus == "" || metrics.CompletionStatus == "likely_complete" {
		return ""
	}
	tone := "border-zinc-400/30 bg-zinc-400/10 text-zinc-300"
	label := "Uncertain"
	if metrics.CompletionStatus == "likely_incomplete" {
		tone = "border-amber-300/35 bg-amber-300/10 text-amber-100"
		label = "Incomplete"
		if metrics.AssignedReturns > 0 {
			label = fmt.Sprintf("Incomplete - %d returns", metrics.AssignedReturns)
		}
	}
	title := strings.Join(firstScrumStrings(metrics.Signals, 3), " - ")
	return fmt.Sprintf(`<span class="rounded border px-1.5 py-0.5 text-[10px] font-medium normal-case tracking-normal %s" title="%s">%s</span>`, tone, html.EscapeString(title), html.EscapeString(label))
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

func pickScrumFocusCardForHTML(board ScrumBoard, cardsByCol map[string][]ScrumCard, playQueue map[string]any, autoWorkEnabled bool, autoWork ScrumAutoWorkConfig) *ScrumCard {
	runningID := playQueueString(playQueue, "running_card_id")
	if runningID != "" {
		if card := findScrumCard(board, runningID); card != nil {
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
	columns := autoWork.SourceColumns
	if len(columns) == 0 {
		columns = []string{"assigned"}
	}
	for _, raw := range columns {
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

func scrumAutoWorkUIComplete(cardsByCol map[string][]ScrumCard) bool {
	total := 0
	for _, cards := range cardsByCol {
		for _, card := range cards {
			total++
			switch card.Column {
			case "done":
			case "review":
			default:
				return false
			}
		}
	}
	return total > 0
}

func playQueueString(playQueue map[string]any, key string) string {
	if playQueue == nil {
		return ""
	}
	value, _ := playQueue[key].(string)
	return strings.TrimSpace(value)
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

func firstScrumStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func scrumBoardFragmentsForPayload(payload map[string]any, fullBoard ScrumBoard) {
	board, _ := payload["board"].(ScrumBoard)
	cardsByCol, _ := payload["cards_by_col"].(map[string][]ScrumCard)
	columnCounts, _ := payload["column_counts"].(map[string]int)
	playQueue, _ := payload["play_queue"].(map[string]any)
	autoWork, _ := payload["auto_work"].(ScrumAutoWorkConfig)
	flowSummary, _ := payload["flow_summary"].(ScrumFlowProjectSummary)
	visibleColumn, _ := payload["visible_column"].(string)
	cardOffset, _ := payload["card_offset"].(int)
	cardHasMore, _ := payload["card_has_more"].(bool)
	payload["html"] = renderScrumBoardFragments(board, cardsByCol, fullBoard, visibleColumn, columnCounts, playQueue, autoWork.Enabled, autoWork, flowSummary, scrumCardPageState{
		Offset: cardOffset, Count: len(board.Cards), HasMore: cardHasMore,
	})
}
