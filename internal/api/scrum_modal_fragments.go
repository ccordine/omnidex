package api

import (
	"fmt"
	"html"
	"strings"
)

func scrumCardTagsLLMPending(card ScrumCard) bool {
	return strings.TrimSpace(card.TagsJobID) != ""
}

func scrumCardTicketLLMPending(card ScrumCard) bool {
	return strings.TrimSpace(card.TicketJobID) != ""
}

func scrumCardAnyLLMPending(card ScrumCard) bool {
	return scrumCardTagsLLMPending(card) || scrumCardTicketLLMPending(card)
}

func renderScrumPendingStatusHTML(key, tone string, pending bool, busyLabel, statusMessage string) string {
	if !pending && strings.TrimSpace(statusMessage) == "" {
		return ""
	}
	spinnerTone := "border-violet-300/25 border-t-violet-200"
	textTone := "text-zinc-500"
	if tone == "cyan" {
		spinnerTone = "border-cyan-300/25 border-t-cyan-200"
	}
	if pending {
		textTone = "text-cyan-200"
	}
	message := strings.TrimSpace(statusMessage)
	if pending && message == "" {
		message = busyLabel
	}
	hidden := "hidden"
	if pending || message != "" {
		hidden = ""
	}
	spinnerHidden := "hidden"
	if pending {
		spinnerHidden = ""
	}
	return fmt.Sprintf(
		`<span data-scrum-pending-status="%s" class="inline-flex items-center gap-1.5 text-[11px] %s %s" aria-live="polite"><span class="inline-block h-3 w-3 shrink-0 animate-spin rounded-full border-2 %s %s"></span><span data-scrum-pending-text>%s</span></span>`,
		html.EscapeString(key),
		textTone,
		hidden,
		spinnerTone,
		spinnerHidden,
		html.EscapeString(message),
	)
}

func renderScrumSectionFeedbackHTML(key string, pending bool, message, tone string) string {
	classes := "mt-2 hidden rounded-md border px-3 py-2 text-xs leading-5"
	switch tone {
	case "error":
		classes = "mt-2 rounded-md border border-rose-400/30 bg-rose-400/10 px-3 py-2 text-xs leading-5 text-rose-100"
	case "ok":
		classes = "mt-2 rounded-md border border-emerald-400/30 bg-emerald-400/10 px-3 py-2 text-xs leading-5 text-emerald-100"
	case "busy":
		classes = "mt-2 rounded-md border border-cyan-300/30 bg-cyan-300/10 px-3 py-2 text-xs leading-5 text-cyan-100"
	}
	if !pending && strings.TrimSpace(message) == "" {
		return fmt.Sprintf(`<p data-scrum-section-feedback="%s" class="mt-2 hidden rounded-md border px-3 py-2 text-xs leading-5" role="status" aria-live="polite"></p>`, html.EscapeString(key))
	}
	hidden := ""
	if strings.TrimSpace(message) == "" {
		hidden = " hidden"
	}
	return fmt.Sprintf(`<p data-scrum-section-feedback="%s" class="%s%s" role="status" aria-live="polite">%s</p>`, html.EscapeString(key), classes, hidden, html.EscapeString(message))
}

func renderScrumTagPillsHTML(card ScrumCard, editable bool) string {
	if len(card.Tags) == 0 {
		return `<p class="text-xs text-zinc-600">No tags yet. Add or suggest tags to build project memory.</p>`
	}
	var b strings.Builder
	b.WriteString(`<div class="flex flex-wrap gap-1.5">`)
	for _, tag := range card.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		remove := ""
		if editable {
			remove = fmt.Sprintf(
				`<button type="button" data-action="scrum#removeCardTag" data-card-id="%s" data-tag="%s" class="ml-1 rounded-full px-1 text-zinc-400 hover:bg-rose-400/20 hover:text-rose-200" title="Remove tag">×</button>`,
				html.EscapeString(card.ID),
				html.EscapeString(tag),
			)
		}
		b.WriteString(fmt.Sprintf(
			`<span class="inline-flex items-center rounded-full border border-cyan-300/25 bg-cyan-300/10 px-2 py-0.5 text-[10px] font-medium text-cyan-100">%s%s</span>`,
			html.EscapeString(tag),
			remove,
		))
	}
	b.WriteString(`</div>`)
	return b.String()
}

func renderScrumModalCardTicketHTML(card ScrumCard) string {
	ticketPending := scrumCardTicketLLMPending(card)
	generateBusy := "Generating…"
	iterateBusy := "Iterating…"
	generateStatus := ""
	iterateStatus := ""
	if ticketPending {
		generateStatus = "Card ticket job running in background…"
		iterateStatus = generateStatus
	}
	savedBadge := ""
	if strings.TrimSpace(card.CardTicket) != "" {
		savedBadge = `<span class="rounded-full border border-emerald-400/30 bg-emerald-400/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-emerald-200">Saved</span>`
	}
	sectionClass := "rounded-lg border border-white/10 bg-zinc-950/50 p-4 transition-[box-shadow,opacity] duration-200"
	if ticketPending {
		sectionClass += " ring-2 ring-violet-300/25 opacity-90"
	}
	ticketPlaceholder := "Generated card ticket markdown appears here…"
	if ticketPending && strings.TrimSpace(card.CardTicket) == "" {
		ticketPlaceholder = "Generating card ticket — you can keep editing other fields…"
	}
	ticketReadonly := ""
	if ticketPending {
		ticketReadonly = ` readonly`
	}
	return fmt.Sprintf(`
    <section class="%s" data-scrum-card-ticket-section data-scrum-section="card-ticket">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Card ticket draft</h3>
            %s
          </div>
          <p class="mt-1 text-xs text-zinc-500">Generate a work ticket from a prompt. The server saves results when the job finishes.</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button type="button" data-action="scrum#generateCardTicket" data-card-id="%s" data-scrum-pending="card-ticket-generate" data-scrum-pending-label="Generate" class="rounded-md bg-violet-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 hover:bg-violet-200 disabled:cursor-not-allowed disabled:opacity-60"%s>%s</button>
          <button type="button" data-action="scrum#iterateCardTicket" data-card-id="%s" data-scrum-pending="card-ticket-iterate" data-scrum-pending-label="Iterate" class="rounded-md border border-violet-300/30 bg-violet-300/10 px-3 py-1.5 text-xs font-semibold text-violet-100 hover:bg-violet-300/20 disabled:cursor-not-allowed disabled:opacity-60"%s>%s</button>
          <button type="button" data-action="scrum#saveCardTicket" data-card-id="%s" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 hover:border-cyan-300/40"%s>Save draft</button>
          %s
          %s
        </div>
      </div>
      %s
      %s
      <textarea data-scrum-field="cardPromptDraft" rows="3" placeholder="Card prompt — what should the ticket cover?" class="scrollbar mt-3 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-violet-300/40">%s</textarea>
      <textarea data-scrum-field="cardIterateNotes" rows="2" placeholder="Iterate notes — what to change in the draft below?" class="scrollbar mt-3 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-violet-300/40"></textarea>
      <textarea data-scrum-field="cardTicket" rows="12" placeholder="%s" class="scrollbar mt-3 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs leading-5 text-zinc-100 outline-none focus:border-violet-300/40"%s>%s</textarea>
    </section>
  `,
		sectionClass,
		savedBadge,
		html.EscapeString(card.ID),
		disabledAttr(ticketPending),
		buttonLabel("Generate", ticketPending, generateBusy),
		html.EscapeString(card.ID),
		disabledAttr(ticketPending),
		buttonLabel("Iterate", ticketPending, iterateBusy),
		html.EscapeString(card.ID),
		disabledAttr(ticketPending),
		renderScrumPendingStatusHTML("card-ticket-generate", "violet", ticketPending, generateBusy, generateStatus),
		renderScrumPendingStatusHTML("card-ticket-iterate", "violet", ticketPending, iterateBusy, iterateStatus),
		renderScrumSectionFeedbackHTML("card-ticket-generate", ticketPending, generateStatus, "busy"),
		renderScrumSectionFeedbackHTML("card-ticket-iterate", ticketPending, iterateStatus, "busy"),
		html.EscapeString(card.CardPrompt),
		html.EscapeString(ticketPlaceholder),
		ticketReadonly,
		html.EscapeString(card.CardTicket),
	)
}

func renderScrumModalTagsPanelHTML(card ScrumCard) string {
	tagsPending := scrumCardTagsLLMPending(card)
	statusMessage := ""
	busyLabel := "Suggest"
	if tagsPending {
		statusMessage = "Tag suggestion running in background…"
		busyLabel = "Suggesting…"
	}
	sectionClass := "rounded-lg border border-white/10 bg-zinc-950/50 p-4 transition-[box-shadow,opacity] duration-200"
	if tagsPending {
		sectionClass += " ring-2 ring-cyan-300/25 opacity-90"
	}
	pillsClass := ""
	if tagsPending {
		pillsClass = ` class="animate-pulse"`
	}
	return fmt.Sprintf(`
    <section class="%s" data-scrum-section="tags">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Tags</h3>
          <p class="mt-1 text-[11px] leading-5 text-zinc-500">Stack labels for memory, research, and similar work later.</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button type="button" data-action="scrum#suggestCardTags" data-card-id="%s" data-scrum-pending="tags-suggest" data-scrum-pending-label="Suggest" class="rounded-md border border-violet-300/30 bg-violet-300/10 px-2.5 py-1 text-[11px] font-semibold text-violet-100 hover:bg-violet-300/20 disabled:cursor-not-allowed disabled:opacity-60"%s>%s</button>
          %s
        </div>
      </div>
      %s
      <div class="mt-3" data-recyclr-sink="scrum-card-tags"%s>%s</div>
      <form data-action="submit->scrum#addCardTag" data-card-id="%s" class="mt-3 flex gap-2">
        <input data-scrum-field="tagInput" type="text" list="scrum-tag-suggestions" placeholder="Search or add tag…" autocomplete="off" data-action="input->scrum#filterTagSuggestions" class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
        <datalist id="scrum-tag-suggestions" data-recyclr-sink="scrum-tag-suggestions"></datalist>
        <button type="submit" class="rounded-md bg-cyan-300 px-3 py-2 text-xs font-semibold text-zinc-950 hover:bg-cyan-200">Add</button>
      </form>
    </section>
  `,
		sectionClass,
		html.EscapeString(card.ID),
		disabledAttr(tagsPending),
		buttonLabel("Suggest", tagsPending, busyLabel),
		renderScrumPendingStatusHTML("tags-suggest", "violet", tagsPending, busyLabel, statusMessage),
		renderScrumSectionFeedbackHTML("tags-suggest", tagsPending, statusMessage, "busy"),
		pillsClass,
		renderScrumTagPillsHTML(card, true),
		html.EscapeString(card.ID),
	)
}

func renderScrumCardLLMSectionBundle(card ScrumCard) string {
	return renderRecyclrTemplateHTML("scrum-card-ticket", renderScrumModalCardTicketHTML(card), "innerHTML") +
		renderRecyclrTemplateHTML("scrum-card-tags", renderScrumModalTagsPanelHTML(card), "innerHTML")
}

func disabledAttr(disabled bool) string {
	if disabled {
		return ` disabled`
	}
	return ""
}

func buttonLabel(idle string, pending bool, busy string) string {
	if pending {
		return html.EscapeString(busy)
	}
	return html.EscapeString(idle)
}
