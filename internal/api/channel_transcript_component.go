package api

import (
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
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

func neutralChannelTranscriptComponent() chatComponentPage {
	message := `<div role="status" class="mx-auto max-w-xl rounded-2xl border border-dashed border-white/10 bg-white/[.02] px-6 py-10 text-center">` +
		`<p class="text-base font-semibold text-zinc-200">Start a new conversation</p>` +
		`<p class="mt-2 text-sm leading-6 text-zinc-400">Start typing below. Sending your first message creates and selects a new conversation automatically.</p>` +
		`</div>`
	bundle := renderRecyclrTemplateHTML(channelTranscriptMessagesTarget, message, "innerHTML") +
		renderRecyclrTemplateHTML(channelTranscriptPaginationTarget, "", "innerHTML")
	return chatComponentPage{HTML: chatComponentHTML{Bundle: bundle}}
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
		if err := model.ValidateChannelMessageSpeaker(message.Role, message.SpeakerName); err != nil {
			return "", fmt.Errorf("channel message %d: %w", message.ID, err)
		}
		if err := validateChannelTranscriptPresentation(message); err != nil {
			return "", fmt.Errorf("channel message %d: %w", message.ID, err)
		}
		markup, err := renderChannelTranscriptMessage(message)
		if err != nil {
			return "", fmt.Errorf("channel message %d: %w", message.ID, err)
		}
		output.WriteString(markup)
		previousID = message.ID
	}
	return output.String(), nil
}

func renderChannelTranscriptMessage(message model.ChannelMessage) (string, error) {
	role := string(message.Role)
	label := "User"
	if message.Role == model.ChannelMessageRoleAssistant {
		label = "Assistant"
	}
	if message.SpeakerName != "" {
		label = message.SpeakerName
	}
	escapedLabel := html.EscapeString(label)
	speakerAttribute := ""
	if message.SpeakerName != "" {
		speakerAttribute = ` data-channel-message-speaker="` + escapedLabel + `"`
	}
	timestamp := message.CreatedAt.UTC()
	failure, err := renderChannelTurnFailure(message)
	if err != nil {
		return "", err
	}
	body := renderChannelMessageBody(message)
	return fmt.Sprintf(`
<article class="message-grid message-%s" data-channel-message-id="%d" data-channel-message-role="%s"%s aria-label="%s message">
  <div class="message-shell">
    <div class="message-meta">
      <span>%s</span>
      <time datetime="%s">%s</time>
    </div>
    %s
    %s
  </div>
</article>`,
		html.EscapeString(role), message.ID, html.EscapeString(role), speakerAttribute, escapedLabel,
		escapedLabel, html.EscapeString(timestamp.Format(time.RFC3339Nano)),
		html.EscapeString(timestamp.Format("15:04 UTC")), body, failure,
	), nil
}

func renderChannelMessageBody(message model.ChannelMessage) string {
	if message.Roleplay == nil || len(message.Roleplay.Parts) == 0 {
		return `<div class="message-body whitespace-pre-wrap break-words text-zinc-100">` +
			html.EscapeString(message.Content) + `</div>`
	}
	var body strings.Builder
	body.WriteString(`<div class="message-body grid gap-2 text-zinc-100">`)
	for _, part := range message.Roleplay.Parts {
		label := strings.ToUpper(part.Kind)
		body.WriteString(`<div class="grid grid-cols-[3.5rem_minmax(0,1fr)] gap-2">` +
			`<span class="pt-0.5 text-[10px] font-semibold tracking-[.12em] text-cyan-300/70">` +
			html.EscapeString(label) + `</span><span class="whitespace-pre-wrap break-words">` +
			html.EscapeString(part.Text) + `</span></div>`)
	}
	body.WriteString(`</div>`)
	return body.String()
}

func validateChannelTranscriptPresentation(message model.ChannelMessage) error {
	if message.Role == model.ChannelMessageRoleAssistant {
		if message.Roleplay != nil || message.Turn != nil {
			return fmt.Errorf("assistant message carries user-turn presentation authority")
		}
		return nil
	}
	if message.Roleplay == nil {
		if message.SpeakerName != "" {
			return fmt.Errorf("user speaker requires roleplay persona authority")
		}
	} else {
		authority := message.Roleplay
		if message.SpeakerName == "" {
			return fmt.Errorf("roleplay user turn requires a displayed persona")
		}
		switch roleplay.UserPersonaKind(authority.PersonaKind) {
		case roleplay.UserPersonaCharacter:
			if err := authority.CharacterID.Validate(); err != nil {
				return err
			}
			if authority.ContributionKind != string(roleplay.UserContributionDialogue) &&
				authority.ContributionKind != string(roleplay.UserContributionAction) &&
				authority.ContributionKind != string(roleplay.UserContributionActionDialogue) &&
				authority.ContributionKind != string(roleplay.UserContributionStructured) {
				return fmt.Errorf("character user turn has incompatible contribution kind")
			}
		case roleplay.UserPersonaNarrator:
			if authority.CharacterID != "" ||
				(authority.ContributionKind != string(roleplay.UserContributionNarration) &&
					authority.ContributionKind != string(roleplay.UserContributionDirection) &&
					authority.ContributionKind != string(roleplay.UserContributionNarrationDirection) &&
					authority.ContributionKind != string(roleplay.UserContributionCommand)) {
				return fmt.Errorf("narrator user turn has incompatible contribution authority")
			}
		case roleplay.UserPersonaLegacy:
			if authority.CharacterID != "" || authority.ContributionKind != string(roleplay.UserContributionLegacy) {
				return fmt.Errorf("historical user turn has contradictory presentation authority")
			}
		default:
			return fmt.Errorf("user turn has unsupported persona kind %q", authority.PersonaKind)
		}
	}
	if message.Turn == nil {
		return nil
	}
	if message.Turn.JobID < 1 {
		return fmt.Errorf("user turn has invalid job identity")
	}
	switch message.Turn.Status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
		model.JobStatusCompleted:
		if message.Turn.Error != "" {
			return fmt.Errorf("user turn has error text outside failed state")
		}
	case model.JobStatusFailed, model.JobStatusCanceled:
		if strings.TrimSpace(message.Turn.Error) == "" {
			return fmt.Errorf("terminal user turn has no exact error")
		}
	default:
		return fmt.Errorf("user turn has unsupported status %q", message.Turn.Status)
	}
	return nil
}

func roleplayContributionLabel(kind string) string {
	switch roleplay.UserContributionKind(kind) {
	case roleplay.UserContributionDialogue:
		return "Speech"
	case roleplay.UserContributionAction:
		return "Action"
	case roleplay.UserContributionActionDialogue:
		return "Action + speech"
	case roleplay.UserContributionStructured:
		return "Structured turn"
	case roleplay.UserContributionNarration:
		return "Narration"
	case roleplay.UserContributionDirection:
		return "Scene direction"
	case roleplay.UserContributionNarrationDirection:
		return "Narration + direction"
	case roleplay.UserContributionCommand:
		return "Command"
	case roleplay.UserContributionLegacy:
		return "Historical turn"
	default:
		return "Unknown"
	}
}

func renderChannelTurnFailure(message model.ChannelMessage) (string, error) {
	if message.Turn == nil ||
		(message.Turn.Status != model.JobStatusFailed && message.Turn.Status != model.JobStatusCanceled) {
		return "", nil
	}
	statusLabel := "This turn failed."
	if message.Turn.Status == model.JobStatusCanceled {
		statusLabel = "This turn was canceled."
	}
	roleplayAttributes := ""
	if message.Roleplay != nil {
		parts := message.Roleplay.Parts
		if parts == nil {
			parts = []model.ChannelMessageRoleplayPart{}
		}
		encodedParts, err := json.Marshal(parts)
		if err != nil {
			return "", fmt.Errorf("encode failed roleplay turn parts: %w", err)
		}
		roleplayAttributes = ` data-roleplay-persona-kind="` + html.EscapeString(message.Roleplay.PersonaKind) +
			`" data-roleplay-character-id="` + html.EscapeString(string(message.Roleplay.CharacterID)) +
			`" data-roleplay-contribution-kind="` + html.EscapeString(message.Roleplay.ContributionKind) +
			`" data-roleplay-turn-parts="` + html.EscapeString(string(encodedParts)) + `"`
	}
	return fmt.Sprintf(`<div role="alert" data-channel-turn-failure data-job-id="%d" class="mt-3 rounded-xl border border-rose-400/30 bg-rose-400/10 p-3 text-sm text-rose-100">
      <p class="font-semibold">%s</p>
      <p class="mt-1 whitespace-pre-wrap break-words text-rose-100/80">%s</p>
      <button type="button" data-action="chat#restoreFailedTurn" data-turn-content="%s"%s class="mt-3 rounded-md border border-rose-200/30 px-3 py-1.5 text-xs font-semibold transition hover:bg-rose-100/10">Restore and retry</button>
		</div>`, message.Turn.JobID, statusLabel, html.EscapeString(message.Turn.Error),
		html.EscapeString(message.Content), roleplayAttributes), nil
}

func renderChannelTranscriptPagination(page model.ChannelMessagePage) string {
	if !page.HasMore || page.NextBeforeID == nil {
		return ""
	}
	cursor := strconv.FormatInt(*page.NextBeforeID, 10)
	return `<button type="button" data-action="chat#loadOlderChannelMessages" data-before-id="` + cursor +
		`" aria-controls="channel-transcript-messages" class="rounded-md border border-white/10 px-3 py-1.5 text-xs font-semibold text-zinc-300 transition hover:border-cyan-300/40 hover:bg-cyan-300/10 disabled:cursor-wait disabled:opacity-60">Load older messages</button>`
}
