import { escapeHTML, formatTime } from "./dom";
import type { ScrumChatMessage } from "./scrum_types";

export type ChatRenderMessage = {
  role: string;
  content: string;
  at?: string;
  created_at?: string;
};

export function chatMessageTimestamp(message: ChatRenderMessage): string {
  return message.at || message.created_at || new Date().toISOString();
}

export function renderChatMessage(message: ChatRenderMessage): string {
  const role = (message.role || "system").toLowerCase();
  const at = chatMessageTimestamp(message);
  return `
    <article class="message-grid message-${escapeHTML(role)}">
      <div class="message-shell">
        <div class="message-meta">
          <span>${escapeHTML(role)}</span>
          <time>${formatTime(at)}</time>
        </div>
        <div class="message-body text-zinc-100">${escapeHTML(message.content)}</div>
      </div>
    </article>
  `;
}

export function renderChatMessages(messages: ChatRenderMessage[], options?: { pending?: boolean; pendingLabel?: string }): string {
  const html = messages.map((message) => renderChatMessage(message)).join("");
  if (!options?.pending) return html;
  const label = options.pendingLabel?.trim() || "Working…";
  return (
    html +
    `
    <article class="message-grid message-assistant message-pending" aria-live="polite">
      <div class="message-shell border border-cyan-300/20 bg-cyan-300/5">
        <div class="message-meta">
          <span>assistant</span>
          <time>${formatTime(new Date().toISOString())}</time>
        </div>
        <div class="message-body flex items-center gap-2 text-sm text-cyan-100">
          <span class="inline-block h-2 w-2 animate-pulse rounded-full bg-cyan-300"></span>
          <span>${escapeHTML(label)}</span>
        </div>
      </div>
    </article>`
  );
}

export function renderChatComposer(options: {
  formAction: string;
  cardId?: string;
  placeholder?: string;
  inputTarget?: string;
  submitLabel?: string;
}): string {
  const cardAttr = options.cardId ? ` data-card-id="${escapeHTML(options.cardId)}"` : "";
  const inputTarget = options.inputTarget ? ` data-scrum-field="${escapeHTML(options.inputTarget)}"` : ' data-scrum-field="chatMessage"';
  return `
    <form data-action="${escapeHTML(options.formAction)}"${cardAttr} class="border-t border-white/10 bg-zinc-950/70 p-3 backdrop-blur-xl md:px-4">
      <div class="rounded-md border border-white/10 bg-zinc-900/90 p-2">
        <textarea${inputTarget} rows="2" placeholder="${escapeHTML(options.placeholder || "Ask Omni to inspect, build, research, or explain…")}" class="scrollbar max-h-32 min-h-[3.25rem] w-full resize-none bg-transparent text-sm leading-5 text-zinc-100 outline-none placeholder:text-zinc-500"></textarea>
        <div class="mt-2 flex flex-wrap items-center justify-between gap-2 border-t border-white/10 pt-2">
          <div class="flex items-center gap-1.5 text-[10px] text-zinc-500">
            <span class="rounded border border-white/10 px-1.5 py-0.5">Enter newline</span>
            <span class="rounded border border-white/10 px-1.5 py-0.5">⌘/Ctrl+Enter send</span>
          </div>
          <button type="submit" class="rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 transition hover:bg-cyan-200 disabled:cursor-not-allowed disabled:bg-zinc-700 disabled:text-zinc-400">
            ${escapeHTML(options.submitLabel || "Send")}
          </button>
        </div>
      </div>
    </form>
  `;
}

export function scrumMessagesToChat(messages: ScrumChatMessage[] = []): ChatRenderMessage[] {
  return messages.map((message) => ({
    role: message.role,
    content: message.content,
    created_at: message.created_at,
  }));
}
