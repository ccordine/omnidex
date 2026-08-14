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

func renderScrumBoardFragments(board ScrumBoard, cardsByCol map[string][]ScrumCard, focusBoard ScrumBoard, visibleColumn string, columnCounts map[string]int, playQueue map[string]any, autoWorkEnabled bool, autoWork ScrumAutoWorkConfig, autoWorkComplete bool, flowSummary ScrumFlowProjectSummary, page scrumCardPageState) (scrumBoardFragments, error) {
	queueView, err := decodeScrumPlayQueueView(playQueue)
	if err != nil {
		return scrumBoardFragments{}, err
	}
	validatedAutoWork, err := validateScrumAutoWorkConfig(autoWork)
	if err != nil {
		return scrumBoardFragments{}, fmt.Errorf("validate Scrum fragment auto-work authority: %w", err)
	}
	if validatedAutoWork.Enabled != autoWorkEnabled {
		return scrumBoardFragments{}, fmt.Errorf("Scrum fragment auto-work flag contradicts its typed configuration")
	}
	autoWork = validatedAutoWork
	columns := focusBoard.Columns
	if len(columns) == 0 {
		columns = append([]string(nil), scrumColumns...)
	}
	activeColumn := normalizeScrumColumn(visibleColumn)
	if activeColumn == "" {
		activeColumn = "assigned"
	}
	focusByColumn, err := cardsByColumn(focusBoard)
	if err != nil {
		return scrumBoardFragments{}, err
	}
	boardHTML, err := renderScrumBoardHTML(board, cardsByCol)
	if err != nil {
		return scrumBoardFragments{}, err
	}
	fragments := scrumBoardFragments{
		Board:       boardHTML,
		Columns:     renderScrumColumnNavHTML(columns, activeColumn, columnCounts),
		Focus:       renderScrumFocusBarHTML(focusBoard, focusByColumn, queueView, autoWorkEnabled, autoWork, autoWorkComplete),
		FlowSummary: renderScrumFlowSummaryHTML(flowSummary),
		Pagination:  renderScrumCardPaginationHTML(page),
	}
	fragments.Bundle = renderScrumBoardBundleHTML(fragments)
	return fragments, nil
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

func renderScrumBoardHTML(board ScrumBoard, cardsByCol map[string][]ScrumCard) (string, error) {
	columns := board.Columns
	if len(columns) == 0 {
		columns = append([]string(nil), scrumColumns...)
	}
	var b strings.Builder
	for _, column := range columns {
		columnHTML, err := renderScrumColumnHTML(column, cardsByCol[normalizeScrumColumn(column)])
		if err != nil {
			return "", err
		}
		b.WriteString(columnHTML)
	}
	return b.String(), nil
}

func renderScrumColumnHTML(column string, cards []ScrumCard) (string, error) {
	column = normalizeScrumColumn(column)
	label := scrumColumnLabel(column)
	items := `<p class="scrum-column-empty rounded-md border border-dashed border-white/10 px-3 py-6 text-center text-xs text-zinc-500">Drop cards here</p>`
	if len(cards) > 0 {
		var b strings.Builder
		for _, card := range cards {
			cardHTML, err := renderScrumCardHTML(card)
			if err != nil {
				return "", err
			}
			b.WriteString(cardHTML)
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
  `, scrumColumnAccent(column), html.EscapeString(column), html.EscapeString(column), html.EscapeString(label), len(cards), html.EscapeString(column), items), nil
}

func renderScrumCardHTML(card ScrumCard) (string, error) {
	flowBadge, err := renderScrumFlowBadgeHTML(card)
	if err != nil {
		return "", fmt.Errorf("render card %q: %w", card.ID, err)
	}
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
		chatCount = int64(len(card.Chat))
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
  `, html.EscapeString(card.ID), html.EscapeString(card.Column), html.EscapeString(card.Title), flowBadge, renderScrumPlayStateBadgeHTML(card), job, description, progress, refs, chats), nil
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
