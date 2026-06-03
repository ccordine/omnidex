package api

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
)

const scrumCardChatDefaultLimit = 5

type scrumCardChatPage struct {
	HTML         string
	Cursor       string
	BeforeCursor string
	HasMore      bool
}

func scrumCardChatComponentID(cardID string) string {
	cardID = strings.TrimSpace(cardID)
	if cardID == "" {
		return ""
	}
	return "scrum-card-" + cardID
}

func scrumCardChatCursor(card ScrumCard) string {
	messages := displayScrumChannelMessages(card)
	if len(messages) == 0 {
		return ""
	}
	last := messages[len(messages)-1]
	return scrumChatMessageID(last)
}

func scrumCardChatBusy(card ScrumCard) bool {
	switch strings.ToLower(strings.TrimSpace(card.PlayState)) {
	case "running", "queued", "reviewing":
		return true
	default:
		return false
	}
}

func scrumCardChatLimit(raw string) int {
	limit, _ := strconv.Atoi(strings.TrimSpace(raw))
	if limit <= 0 {
		return scrumCardChatDefaultLimit
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func scrumCardChatPageFor(card ScrumCard, limit int, before string) scrumCardChatPage {
	messages := displayScrumChannelMessages(card)
	if len(messages) == 0 {
		return scrumCardChatPage{
			HTML: `<div class="flex h-full min-h-[12rem] items-center justify-center px-4 py-8 text-center text-sm text-zinc-500">Play this card to watch the agent work - commands, file edits, diffs, thinking, and replies stream here in real time.</div>`,
		}
	}
	end := len(messages)
	before = strings.TrimSpace(before)
	if before != "" {
		for i, msg := range messages {
			if scrumChatMessageID(msg) == before || strings.TrimSpace(msg.ID) == before {
				end = i
				break
			}
		}
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	page := messages[start:end]
	if len(page) == 0 {
		return scrumCardChatPage{}
	}
	var b strings.Builder
	for _, msg := range page {
		b.WriteString(renderScrumCardChatMessageHTML(msg))
	}
	if before == "" && scrumCardChatBusy(card) {
		b.WriteString(`<article class="message-grid message-assistant message-pending" data-chat-component-working-message aria-live="polite"><div class="message-shell border border-cyan-300/20 bg-cyan-300/5"><div class="message-meta"><span>assistant</span><time>now</time></div><div class="message-body flex items-center gap-2 text-sm text-cyan-100"><span class="inline-block h-2 w-2 animate-pulse rounded-full bg-cyan-300"></span><span>Agent working...</span></div></div></article>`)
	}
	if before == "" {
		b.WriteString(`<div data-scrum-channel-anchor class="h-px w-full shrink-0" aria-hidden="true"></div>`)
	}
	first := page[0]
	return scrumCardChatPage{
		HTML:         b.String(),
		Cursor:       scrumCardChatCursor(card),
		BeforeCursor: scrumChatMessageID(first),
		HasMore:      start > 0,
	}
}

func renderScrumCardChatHTML(card ScrumCard) string {
	return scrumCardChatPageFor(card, scrumCardChatDefaultLimit, "").HTML
}

func scrumCardChatResponseCard(card ScrumCard) ScrumCard {
	return scrumCardBoardSummary(card)
}

func renderScrumCardChatMessageHTML(msg ScrumChatMessage) string {
	role := normalizeScrumChannelRole(msg.Role)
	id := scrumChatMessageID(msg)
	rawContent := strings.TrimSpace(msg.Content)
	if activity, ok := parseChannelActivity(rawContent); ok {
		return renderScrumCardActivityMessageHTML(msg, id, activity)
	}
	if summary, detail, ok := renderScrumStructuredJSON(rawContent); ok {
		return renderScrumCardStructuredMessageHTML(msg, id, summary, detail)
	}
	content := html.EscapeString(rawContent)
	at := html.EscapeString(strings.TrimSpace(msg.CreatedAt))
	attrs := fmt.Sprintf(` data-chat-message-id="%s" data-recyclr-sink="chat-message-%s"`, html.EscapeString(id), html.EscapeString(id))
	switch role {
	case "system":
		return fmt.Sprintf(`<article class="message-grid message-system"%s><div class="message-shell"><div class="message-body text-center text-[11px] leading-5 text-zinc-500">%s</div></div></article>`, attrs, content)
	case "thinking":
		return fmt.Sprintf(`<article class="message-grid message-thinking"%s><div class="message-shell"><div class="message-meta"><span>thinking</span><time>%s</time></div><div class="message-body whitespace-pre-wrap text-xs italic leading-5 text-zinc-500">%s</div></div></article>`, attrs, at, content)
	case "user":
		return fmt.Sprintf(`<article class="message-grid message-user"%s><div class="message-shell"><div class="message-meta"><span>you</span><time>%s</time></div><div class="message-body whitespace-pre-wrap text-sm leading-6 text-cyan-50">%s</div></div></article>`, attrs, at, content)
	case "error":
		return fmt.Sprintf(`<article class="message-grid message-error"%s><div class="message-shell"><div class="message-meta"><span>error</span><time>%s</time></div><div class="message-body whitespace-pre-wrap text-sm leading-6 text-rose-200">%s</div></div></article>`, attrs, at, content)
	default:
		return fmt.Sprintf(`<article class="message-grid message-assistant"%s><div class="message-shell"><div class="message-meta"><span>agent</span><time>%s</time></div><div class="message-body channel-agent-reply whitespace-pre-wrap text-[0.9375rem] leading-7 text-zinc-50">%s</div></div></article>`, attrs, at, content)
	}
}

func renderScrumCardActivityMessageHTML(msg ScrumChatMessage, id string, activity ChannelActivity) string {
	at := html.EscapeString(strings.TrimSpace(msg.CreatedAt))
	attrs := scrumChatMessageAttrs(id) + ` data-chat-message-detail-card="true" data-chat-message-detail-title="` + html.EscapeString(scrumActivityKindLabel(activity)) + `"`
	status := scrumActivityStatusLabel(activity.Status)
	statusClass := scrumActivityStatusClass(activity.Status)
	kind := scrumActivityKindLabel(activity)
	title := firstNonEmpty(strings.TrimSpace(activity.Title), kind)
	pathLine := ""
	if strings.TrimSpace(activity.Path) != "" {
		pathLine = fmt.Sprintf(`<div class="mt-1 font-mono text-[11px] text-zinc-400">%s</div>`, html.EscapeString(strings.TrimSpace(activity.Path)))
	}
	commandBlock := ""
	if strings.TrimSpace(activity.Command) != "" {
		commandBlock = fmt.Sprintf(`<pre class="mt-2 overflow-x-auto rounded-md border border-white/10 bg-zinc-950/80 p-2 font-mono text-[11px] leading-5 text-cyan-100">%s</pre>`, html.EscapeString(strings.TrimSpace(activity.Command)))
	}
	detailLine := scrumActivityInlineDetail(activity)
	if detailLine != "" {
		detailLine = fmt.Sprintf(`<div class="mt-2 line-clamp-3 whitespace-pre-wrap text-xs leading-5 text-zinc-400">%s</div>`, html.EscapeString(detailLine))
	}
	files := renderScrumActivityFiles(activity.Files, 3)
	detailHTML := renderScrumActivityDetailHTML(activity)
	return fmt.Sprintf(
		`<article class="message-grid message-tool cursor-pointer"%s><div class="message-shell channel-activity-shell"><div class="message-meta"><span>%s</span><time>%s</time></div><div class="message-body"><div class="flex flex-wrap items-center gap-2"><span class="text-sm font-medium text-zinc-100">%s</span><span class="rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide %s">%s</span><button type="button" data-chat-message-detail-open class="rounded border border-white/10 px-2 py-0.5 text-[10px] uppercase tracking-wide text-zinc-400 hover:border-cyan-300/40 hover:text-cyan-100">Details</button></div>%s%s%s%s</div></div><template data-chat-message-detail>%s</template></article>`,
		attrs,
		html.EscapeString(kind),
		at,
		html.EscapeString(title),
		statusClass,
		html.EscapeString(status),
		pathLine,
		files,
		commandBlock,
		detailLine,
		detailHTML,
	)
}

func renderScrumCardStructuredMessageHTML(msg ScrumChatMessage, id, summary, detailHTML string) string {
	role := normalizeScrumChannelRole(msg.Role)
	at := html.EscapeString(strings.TrimSpace(msg.CreatedAt))
	label := "agent"
	if role == "tool" {
		label = "tool"
	} else if role == "system" {
		label = "system"
	}
	attrs := scrumChatMessageAttrs(id) + ` data-chat-message-detail-card="true" data-chat-message-detail-title="Structured message"`
	return fmt.Sprintf(
		`<article class="message-grid message-%s cursor-pointer"%s><div class="message-shell"><div class="message-meta"><span>%s</span><time>%s</time></div><div class="message-body whitespace-pre-wrap text-sm leading-6 text-zinc-100"><div>%s</div><button type="button" data-chat-message-detail-open class="mt-2 rounded border border-white/10 px-2 py-1 text-[10px] uppercase tracking-wide text-zinc-400 hover:border-cyan-300/40 hover:text-cyan-100">Details</button></div></div><template data-chat-message-detail>%s</template></article>`,
		html.EscapeString(role),
		attrs,
		html.EscapeString(label),
		at,
		html.EscapeString(summary),
		detailHTML,
	)
}

func scrumChatMessageAttrs(id string) string {
	escaped := html.EscapeString(id)
	return fmt.Sprintf(` data-chat-message-id="%s" data-recyclr-sink="chat-message-%s"`, escaped, escaped)
}

func scrumActivityKindLabel(activity ChannelActivity) string {
	switch strings.TrimSpace(activity.Activity) {
	case "command":
		return "Command"
	case "file_change":
		return "File change"
	case "patch":
		return "Patch"
	case "tool_call":
		return firstNonEmpty(strings.TrimSpace(activity.Tool), "Tool")
	case "output":
		return "Output"
	default:
		return firstNonEmpty(strings.TrimSpace(activity.Title), "Activity")
	}
}

func scrumActivityStatusLabel(status string) string {
	switch normalizeActivityStatus(status) {
	case "running":
		return "Running"
	case "failed":
		return "Failed"
	default:
		return "Done"
	}
}

func scrumActivityStatusClass(status string) string {
	switch normalizeActivityStatus(status) {
	case "running":
		return "border-amber-300/30 bg-amber-300/10 text-amber-100"
	case "failed":
		return "border-rose-400/30 bg-rose-400/10 text-rose-200"
	default:
		return "border-emerald-400/25 bg-emerald-400/10 text-emerald-200"
	}
}

func scrumActivityInlineDetail(activity ChannelActivity) string {
	detail := strings.TrimSpace(activity.Detail)
	if activity.Activity == "command" {
		return ""
	}
	if looksLikeStructuredActivityDetail(detail) {
		return summarizeStructuredActivityDetail(detail)
	}
	if len(detail) > 220 {
		return detail[:217] + "..."
	}
	return detail
}

func renderScrumActivityFiles(files []string, limit int) string {
	if len(files) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(files) {
		limit = len(files)
	}
	var b strings.Builder
	b.WriteString(`<ul class="mt-2 space-y-1">`)
	for _, file := range files[:limit] {
		b.WriteString(fmt.Sprintf(`<li class="flex items-center gap-2 font-mono text-[11px] text-zinc-300"><span class="text-emerald-300">●</span>%s</li>`, html.EscapeString(strings.TrimSpace(file))))
	}
	if len(files) > limit {
		b.WriteString(fmt.Sprintf(`<li class="text-[11px] text-zinc-500">+%d more</li>`, len(files)-limit))
	}
	b.WriteString(`</ul>`)
	return b.String()
}

func renderScrumActivityDetailHTML(activity ChannelActivity) string {
	detailRows := renderScrumActivityDetailRows(activity.Detail)
	rows := []string{
		renderScrumDetailRow("Activity", activity.Activity),
		renderScrumDetailRow("Status", scrumActivityStatusLabel(activity.Status)),
		renderScrumDetailRow("Title", activity.Title),
		renderScrumDetailRow("Tool", activity.Tool),
		renderScrumDetailRow("Path", activity.Path),
		renderScrumDetailRow("Command", activity.Command),
		detailRows,
	}
	files := ""
	if len(activity.Files) > 0 {
		files = renderScrumDetailList("Files", activity.Files)
	}
	diff := ""
	if strings.TrimSpace(activity.Diff) != "" {
		diff = fmt.Sprintf(`<section class="space-y-2"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Diff</h3><pre class="max-h-[50vh] overflow-auto rounded-md border border-white/10 bg-zinc-950/80 p-3 font-mono text-xs leading-5 text-zinc-200">%s</pre></section>`, html.EscapeString(strings.TrimSpace(activity.Diff)))
	}
	return fmt.Sprintf(`<div class="space-y-4">%s%s%s</div>`, strings.Join(rows, ""), files, diff)
}

func renderScrumActivityDetailRows(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return ""
	}
	if _, structured, ok := renderScrumStructuredJSON(detail); ok {
		return fmt.Sprintf(`<section class="space-y-2"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Detail</h3>%s</section>`, structured)
	}
	if looksLikeStructuredActivityDetail(detail) {
		return renderScrumDetailRow("Detail", summarizeStructuredActivityDetail(detail)) +
			fmt.Sprintf(`<section class="space-y-1"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Technical detail</h3><pre class="max-h-[40vh] overflow-auto rounded-md border border-white/10 bg-zinc-950/80 p-3 font-mono text-xs leading-5 text-zinc-300">%s</pre></section>`, html.EscapeString(detail))
	}
	return renderScrumDetailRow("Detail", detail)
}

func looksLikeStructuredActivityDetail(detail string) bool {
	detail = strings.TrimSpace(detail)
	return strings.HasPrefix(detail, "map[") || strings.HasPrefix(detail, "{") || strings.HasPrefix(detail, "[")
}

func summarizeStructuredActivityDetail(detail string) string {
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "status:success") || strings.Contains(lower, `"status":"success"`) || strings.Contains(lower, `"status": "success"`):
		return "Completed successfully. Open details for the structured output."
	case strings.Contains(lower, "status:failed") || strings.Contains(lower, `"status":"failed"`) || strings.Contains(lower, `"status": "failed"`):
		return "Failed. Open details for the structured output."
	case strings.Contains(lower, "status:running") || strings.Contains(lower, `"status":"running"`) || strings.Contains(lower, `"status": "running"`):
		return "Running. Open details for the structured output."
	case strings.Contains(lower, "matches:") || strings.Contains(lower, `"matches"`):
		return "Search results are available in details."
	default:
		return "Structured output is available in details."
	}
}

func renderScrumDetailRow(label, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return fmt.Sprintf(`<section class="space-y-1"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">%s</h3><div class="whitespace-pre-wrap rounded-md border border-white/10 bg-white/[.03] p-3 text-sm leading-6 text-zinc-100">%s</div></section>`, html.EscapeString(label), html.EscapeString(value))
}

func renderScrumDetailList(label string, values []string) string {
	var items strings.Builder
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		items.WriteString(fmt.Sprintf(`<li class="font-mono text-xs text-zinc-200">%s</li>`, html.EscapeString(value)))
	}
	if items.Len() == 0 {
		return ""
	}
	return fmt.Sprintf(`<section class="space-y-2"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">%s</h3><ul class="space-y-1 rounded-md border border-white/10 bg-white/[.03] p-3">%s</ul></section>`, html.EscapeString(label), items.String())
}

func renderScrumStructuredJSON(raw string) (summary string, detailHTML string, ok bool) {
	var payload any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return "", "", false
	}
	summary = summarizeScrumJSONPayload(payload)
	detailHTML = renderScrumJSONDetailHTML(payload)
	return summary, detailHTML, summary != "" && detailHTML != ""
}

func summarizeScrumJSONPayload(payload any) string {
	switch typed := payload.(type) {
	case map[string]any:
		message := firstNonEmpty(jsonStringField(typed, "message"), jsonStringField(typed, "summary"), jsonStringField(typed, "title"), jsonStringField(typed, "content"), jsonStringField(typed, "error"))
		prefix := firstNonEmpty(jsonStringField(typed, "type"), jsonStringField(typed, "event"), jsonStringField(typed, "status"), "Structured update")
		if message != "" {
			return humanizeStepEventType(prefix) + ": " + trimScrumSummary(message, 220)
		}
		return humanizeStepEventType(prefix)
	case []any:
		return fmt.Sprintf("Structured update: %d items", len(typed))
	default:
		return trimScrumSummary(fmt.Sprint(typed), 220)
	}
}

func jsonStringField(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64, bool:
		return strings.TrimSpace(fmt.Sprint(typed))
	default:
		return ""
	}
}

func trimScrumSummary(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit-3] + "..."
}

func renderScrumJSONDetailHTML(payload any) string {
	return fmt.Sprintf(`<div class="space-y-3">%s</div>`, renderScrumJSONValueHTML(payload))
}

func renderScrumJSONValueHTML(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		var b strings.Builder
		b.WriteString(`<dl class="space-y-3">`)
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			item := typed[key]
			b.WriteString(fmt.Sprintf(`<div class="rounded-md border border-white/10 bg-white/[.03] p-3"><dt class="mb-2 text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">%s</dt><dd>%s</dd></div>`, html.EscapeString(humanizeStepEventType(key)), renderScrumJSONValueHTML(item)))
		}
		b.WriteString(`</dl>`)
		return b.String()
	case []any:
		var b strings.Builder
		b.WriteString(`<ol class="space-y-2">`)
		for i, item := range typed {
			b.WriteString(fmt.Sprintf(`<li class="rounded-md border border-white/10 bg-zinc-950/50 p-3"><div class="mb-2 text-[11px] uppercase tracking-[.16em] text-zinc-500">Item %d</div>%s</li>`, i+1, renderScrumJSONValueHTML(item)))
		}
		b.WriteString(`</ol>`)
		return b.String()
	case string:
		return fmt.Sprintf(`<div class="whitespace-pre-wrap text-sm leading-6 text-zinc-100">%s</div>`, html.EscapeString(typed))
	case float64, bool:
		return fmt.Sprintf(`<span class="font-mono text-sm text-cyan-100">%s</span>`, html.EscapeString(fmt.Sprint(typed)))
	case nil:
		return `<span class="text-sm text-zinc-500">null</span>`
	default:
		return fmt.Sprintf(`<span class="text-sm text-zinc-100">%s</span>`, html.EscapeString(fmt.Sprint(typed)))
	}
}

func renderScrumCardChatBundle(card ScrumCard) string {
	componentID := scrumCardChatComponentID(card.ID)
	if componentID == "" {
		return ""
	}
	return fmt.Sprintf(
		`<template data-recyclr-target="chat-%s-messages" data-recyclr-location="innerHTML">%s</template>`,
		html.EscapeString(componentID),
		renderScrumCardChatHTML(card),
	)
}
