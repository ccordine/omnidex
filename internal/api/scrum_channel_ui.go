package api

import (
	"fmt"
	"html"
	"strings"
)

type scrumChannelSurfaceOptions struct {
	Eyebrow       string
	Title         string
	Subtitle      string
	StatusHTML    string
	ActionsHTML   string
	BadgeHTML     string
	MessagesHTML  string
	MessagesAttrs string
	MessagesClass string
	ComposerHTML  string
	ShellClass    string
	HeaderClass   string
}

type scrumChannelComposerOptions struct {
	FormAction      string
	KeydownAction   string
	CardID          string
	ComponentID     string
	Endpoint        string
	Placeholder     string
	InputTarget     string
	InputTargetAttr string
	InputName       string
	InputType       string
	SubmitLabel     string
	Disabled        bool
}

func renderChannelSurfaceHTML(options scrumChannelSurfaceOptions) string {
	shellClass := options.ShellClass
	if shellClass == "" {
		shellClass = "flex min-h-[min(70vh,720px)] flex-col overflow-hidden rounded-lg border border-white/10 bg-zinc-950/50"
	}
	headerClass := options.HeaderClass
	if headerClass == "" {
		headerClass = "flex flex-wrap items-center justify-between gap-2 border-b border-white/10 bg-zinc-950/45 px-3 py-2 backdrop-blur-xl md:px-4"
	}
	messagesClass := options.MessagesClass
	if messagesClass == "" {
		messagesClass = "scrollbar min-h-0 flex-1 overflow-y-auto overflow-x-hidden px-3 py-3 md:px-4"
	}
	attrs := ""
	if strings.TrimSpace(options.MessagesAttrs) != "" {
		attrs = " " + options.MessagesAttrs
	}
	eyebrow := ""
	if strings.TrimSpace(options.Eyebrow) != "" {
		eyebrow = fmt.Sprintf(`<p class="text-[10px] uppercase tracking-[.18em] text-cyan-200/80">%s</p>`, html.EscapeString(options.Eyebrow))
	}
	subtitle := ""
	if strings.TrimSpace(options.Subtitle) != "" {
		subtitle = fmt.Sprintf(`<p class="text-xs text-zinc-500">%s</p>`, html.EscapeString(options.Subtitle))
	}
	return fmt.Sprintf(`
    <div class="%s">
      <header class="%s">
        <div class="min-w-0">
          %s
          <h3 class="truncate text-lg font-semibold tracking-tight text-zinc-100">%s</h3>
          %s
        </div>
        <div class="flex flex-wrap items-center gap-2">
          %s
          %s
          %s
        </div>
      </header>
      <div%s class="%s">%s</div>
      %s
    </div>
  `, shellClass, headerClass, eyebrow, html.EscapeString(options.Title), subtitle, options.StatusHTML, options.ActionsHTML, options.BadgeHTML, attrs, messagesClass, options.MessagesHTML, options.ComposerHTML)
}

func renderChatComposerHTML(options scrumChannelComposerOptions) string {
	cardAttr := ""
	if strings.TrimSpace(options.CardID) != "" {
		cardAttr = fmt.Sprintf(` data-card-id="%s"`, html.EscapeString(options.CardID))
	}
	componentAttr := ""
	if strings.TrimSpace(options.ComponentID) != "" {
		componentAttr = fmt.Sprintf(` data-chat-component-id="%s"`, html.EscapeString(options.ComponentID))
	}
	endpointAttr := ""
	if strings.TrimSpace(options.Endpoint) != "" {
		endpointAttr = fmt.Sprintf(` data-chat-endpoint="%s"`, html.EscapeString(options.Endpoint))
	}
	inputTarget := ""
	switch {
	case strings.TrimSpace(options.InputTargetAttr) != "":
		inputTarget = fmt.Sprintf(` %s="%s"`, options.InputTargetAttr, html.EscapeString(options.InputTarget))
	case strings.TrimSpace(options.InputTarget) != "":
		inputTarget = fmt.Sprintf(` data-scrum-field="%s"`, html.EscapeString(options.InputTarget))
	default:
		inputTarget = ` data-scrum-field="chatMessage"`
	}
	inputName := ""
	if strings.TrimSpace(options.InputName) != "" {
		inputName = fmt.Sprintf(` name="%s"`, html.EscapeString(options.InputName))
	}
	disabled := ""
	if options.Disabled {
		disabled = " disabled"
	}
	keydownAction := ""
	if strings.TrimSpace(options.KeydownAction) != "" {
		keydownAction = fmt.Sprintf(` keydown->%s`, html.EscapeString(options.KeydownAction))
	}
	placeholder := html.EscapeString(firstNonEmpty(options.Placeholder, "Ask Omni to inspect, build, research, or explain..."))
	var control string
	if options.InputType == "input" {
		control = fmt.Sprintf(`<input%s%s%s placeholder="%s" class="min-w-[220px] flex-1 bg-transparent text-sm text-zinc-100 outline-none placeholder:text-zinc-500 disabled:opacity-60" />`, inputTarget, inputName, disabled, placeholder)
	} else {
		control = fmt.Sprintf(`<textarea%s%s%s rows="2" placeholder="%s" class="scrollbar max-h-32 min-h-[3.25rem] w-full resize-none bg-transparent text-sm leading-5 text-zinc-100 outline-none placeholder:text-zinc-500 disabled:opacity-60"></textarea>`, inputTarget, inputName, disabled, placeholder)
	}
	button := renderScrumComposerButtonHTML(options.SubmitLabel, disabled)
	if options.InputType == "input" {
		return fmt.Sprintf(`
    <form data-action="%s%s"%s%s%s class="border-t border-white/10 bg-zinc-950/70 p-3 backdrop-blur-xl md:px-4">
      <div class="rounded-md border border-white/10 bg-zinc-900/90 p-2">
        <div class="flex flex-wrap items-center gap-2">
          %s
          %s
        </div>
      </div>
    </form>
  `, html.EscapeString(options.FormAction), keydownAction, cardAttr, componentAttr, endpointAttr, control, button)
	}
	return fmt.Sprintf(`
    <form data-action="%s%s"%s%s%s class="border-t border-white/10 bg-zinc-950/70 p-3 backdrop-blur-xl md:px-4">
      <div class="rounded-md border border-white/10 bg-zinc-900/90 p-2">
        %s
        <div class="mt-2 flex flex-wrap items-center justify-between gap-2 border-t border-white/10 pt-2">
          <div class="flex items-center gap-1.5 text-[10px] text-zinc-500">
            <span class="rounded border border-white/10 px-1.5 py-0.5">Enter newline</span>
            <span class="rounded border border-white/10 px-1.5 py-0.5">Cmd/Ctrl+Enter send</span>
          </div>
          %s
        </div>
      </div>
    </form>
  `, html.EscapeString(options.FormAction), keydownAction, cardAttr, componentAttr, endpointAttr, control, button)
}

func renderScrumComposerButtonHTML(submitLabel, disabled string) string {
	label := strings.TrimSpace(submitLabel)
	if label == "" {
		label = "Send"
	}
	return fmt.Sprintf(`
    <button type="submit"%s class="rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 transition hover:bg-cyan-200 disabled:cursor-not-allowed disabled:bg-zinc-700 disabled:text-zinc-400">
      %s
    </button>
  `, disabled, html.EscapeString(label))
}

func renderScrumModalChannelMessagesHTML(card ScrumCard, pilotPending bool) string {
	messages := displayScrumChannelMessages(card)
	if len(messages) > scrumCardChatDefaultLimit {
		messages = messages[len(messages)-scrumCardChatDefaultLimit:]
	}
	isLive := card.PlayState == "running" || card.PlayState == "queued" || card.PlayState == "reviewing"
	showPending := pilotPending || isLive
	pendingLabel := "Agent working..."
	if pilotPending {
		pendingLabel = "Sending..."
	}
	if len(messages) == 0 && !showPending {
		return `<div class="flex h-full min-h-[12rem] items-center justify-center px-4 py-8 text-center text-sm text-zinc-500">Play this card to watch the agent work — commands, file edits, diffs, thinking, and replies stream here in real time.</div>`
	}
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(renderScrumCardChatMessageHTML(msg))
	}
	if showPending {
		b.WriteString(renderScrumPendingChannelMessageHTML(pendingLabel))
	}
	b.WriteString(`<div data-scrum-channel-anchor class="h-px w-full shrink-0" aria-hidden="true"></div>`)
	return b.String()
}

func renderScrumPendingChannelMessageHTML(label string) string {
	return fmt.Sprintf(`
    <article class="message-grid message-assistant message-pending" data-chat-component-working-message aria-live="polite">
      <div class="message-shell border border-cyan-300/20 bg-cyan-300/5">
        <div class="message-meta">
          <span>assistant</span>
          <time>now</time>
        </div>
        <div class="message-body flex items-center gap-2 text-sm text-cyan-100">
          <span class="inline-block h-2 w-2 animate-pulse rounded-full bg-cyan-300"></span>
          <span>%s</span>
        </div>
      </div>
    </article>`, html.EscapeString(label))
}
