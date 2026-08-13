package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const (
	channelTranscriptMessagesTarget   = "channel-transcript-messages"
	channelTranscriptPaginationTarget = "channel-transcript-pagination"
)

type channelTranscriptHTML struct {
	Bundle string `json:"bundle"`
}

type channelTranscriptResponse struct {
	ChannelID    model.ChannelID       `json:"channel_id"`
	NextBeforeID *int64                `json:"next_before_id,omitempty"`
	HasMore      bool                  `json:"has_more"`
	HTML         channelTranscriptHTML `json:"html"`
}

func channelTranscriptResponseFor(
	channelID model.ChannelID,
	page model.ChannelMessagePage,
	prepend bool,
) (channelTranscriptResponse, error) {
	if err := channelID.Validate(); err != nil {
		return channelTranscriptResponse{}, err
	}
	if page.HasMore != (page.NextBeforeID != nil) {
		return channelTranscriptResponse{}, fmt.Errorf("channel transcript pagination is contradictory")
	}
	if page.NextBeforeID != nil && *page.NextBeforeID < 1 {
		return channelTranscriptResponse{}, fmt.Errorf("channel transcript cursor must be positive")
	}
	if page.NextBeforeID != nil && (len(page.Messages) == 0 || *page.NextBeforeID != page.Messages[0].ID) {
		return channelTranscriptResponse{}, fmt.Errorf("channel transcript cursor must identify the first rendered message")
	}
	markup, err := renderChannelTranscriptMessages(channelID, page.Messages, prepend)
	if err != nil {
		return channelTranscriptResponse{}, err
	}
	location := "innerHTML"
	if prepend {
		location = "afterbegin"
	}
	bundle := renderRecyclrTemplateHTML(channelTranscriptMessagesTarget, markup, location) +
		renderRecyclrTemplateHTML(channelTranscriptPaginationTarget, renderChannelTranscriptPagination(page), "innerHTML")
	return channelTranscriptResponse{
		ChannelID: channelID, NextBeforeID: page.NextBeforeID, HasMore: page.HasMore,
		HTML: channelTranscriptHTML{Bundle: bundle},
	}, nil
}

func renderChannelTranscriptMessages(
	channelID model.ChannelID,
	messages []model.ChannelMessage,
	prepend bool,
) (string, error) {
	if len(messages) == 0 && !prepend {
		return `<p role="status" class="rounded-md border border-dashed border-white/10 px-4 py-8 text-center text-sm text-zinc-500">No messages in this channel yet.</p>`, nil
	}
	var output strings.Builder
	var previousID int64
	for _, message := range messages {
		if message.ID < 1 || message.ChannelID != channelID || message.ID <= previousID {
			return "", fmt.Errorf("channel transcript contains invalid or unordered message identity %d", message.ID)
		}
		if message.CreatedAt.IsZero() {
			return "", fmt.Errorf("channel message %d has no creation time", message.ID)
		}
		if err := model.ValidateChannelMessage(message.Role, message.Content); err != nil {
			return "", fmt.Errorf("channel message %d: %w", message.ID, err)
		}
		output.WriteString(renderChannelTranscriptMessage(message))
		previousID = message.ID
	}
	return output.String(), nil
}

func renderChannelTranscriptMessage(message model.ChannelMessage) string {
	role := string(message.Role)
	label := "User"
	if message.Role == model.ChannelMessageRoleAssistant {
		label = "Assistant"
	}
	timestamp := message.CreatedAt.UTC()
	return fmt.Sprintf(`
<article class="message-grid message-%s" data-channel-message-id="%d" data-channel-message-role="%s" aria-label="%s message">
  <div class="message-shell">
    <div class="message-meta">
      <span>%s</span>
      <time datetime="%s">%s</time>
    </div>
    <div class="message-body whitespace-pre-wrap break-words text-zinc-100">%s</div>
  </div>
</article>`,
		html.EscapeString(role), message.ID, html.EscapeString(role), label,
		strings.ToLower(label), html.EscapeString(timestamp.Format(time.RFC3339Nano)),
		html.EscapeString(timestamp.Format("15:04 UTC")), html.EscapeString(message.Content),
	)
}

func renderChannelTranscriptPagination(page model.ChannelMessagePage) string {
	if !page.HasMore || page.NextBeforeID == nil {
		return ""
	}
	cursor := strconv.FormatInt(*page.NextBeforeID, 10)
	return `<button type="button" data-action="chat#loadOlderChannelMessages" data-before-id="` + cursor +
		`" aria-controls="channel-transcript-messages" class="rounded-md border border-white/10 px-3 py-1.5 text-xs font-semibold text-zinc-300 transition hover:border-cyan-300/40 hover:bg-cyan-300/10 disabled:cursor-wait disabled:opacity-60">Load older messages</button>`
}
