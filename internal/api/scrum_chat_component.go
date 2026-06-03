package api

import (
	"fmt"
	"html"
	"strings"
)

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

func renderScrumCardChatHTML(card ScrumCard) string {
	messages := displayScrumChannelMessages(card)
	if len(messages) == 0 {
		return `<div class="flex h-full min-h-[12rem] items-center justify-center px-4 py-8 text-center text-sm text-zinc-500">Play this card to watch the agent work - commands, file edits, diffs, thinking, and replies stream here in real time.</div>`
	}
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(renderScrumCardChatMessageHTML(msg))
	}
	if scrumCardChatBusy(card) {
		b.WriteString(`<article class="message-grid message-assistant message-pending" data-chat-component-working-message aria-live="polite"><div class="message-shell border border-cyan-300/20 bg-cyan-300/5"><div class="message-meta"><span>assistant</span><time>now</time></div><div class="message-body flex items-center gap-2 text-sm text-cyan-100"><span class="inline-block h-2 w-2 animate-pulse rounded-full bg-cyan-300"></span><span>Agent working...</span></div></div></article>`)
	}
	b.WriteString(`<div data-scrum-channel-anchor class="h-px w-full shrink-0" aria-hidden="true"></div>`)
	return b.String()
}

func renderScrumCardChatMessageHTML(msg ScrumChatMessage) string {
	role := normalizeScrumChannelRole(msg.Role)
	id := scrumChatMessageID(msg)
	content := html.EscapeString(strings.TrimSpace(msg.Content))
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
